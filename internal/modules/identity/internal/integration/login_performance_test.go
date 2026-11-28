//go:build integration && identity_perf

package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/integration/internal/fixture"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/credential"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailcrypto"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

type loginPerfSampleKey struct{}

type loginPerfSample struct {
	total, admissionDB, credentialDB, begin, sqlCallbacks, kdf time.Duration
}

type loginPerfTx struct {
	pool      *platformpg.Pool
	admission bool
}

func (db loginPerfTx) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	sample := ctx.Value(loginPerfSampleKey{}).(*loginPerfSample)
	start := time.Now()
	err := db.pool.WithTx(ctx, func(tx pgx.Tx) error {
		callbackStarted := time.Now()
		sample.begin += callbackStarted.Sub(start)
		err := fn(tx)
		sample.sqlCallbacks += time.Since(callbackStarted)
		return err
	})
	if db.admission {
		sample.admissionDB += time.Since(start)
	} else {
		sample.credentialDB += time.Since(start)
	}
	return err
}

type loginPerfVerifier struct {
	hasher *credential.PasswordHasher
}

func (v loginPerfVerifier) Verify(ctx context.Context, password string, verifier domain.PasswordVerifier) (bool, error) {
	sample := ctx.Value(loginPerfSampleKey{}).(*loginPerfSample)
	start := time.Now()
	ok, err := v.hasher.Verify(ctx, password, verifier)
	sample.kdf += time.Since(start)
	return ok, err
}

type loginPerfActor struct {
	email   string
	subject string
	peer    netip.Addr
}

func TestIdentityLoginPerformance(t *testing.T) {
	const (
		attempts = 64
		actors   = 16
		cpuCount = 2
		goMemory = 256 << 20
	)
	workers, err := strconv.Atoi(os.Getenv("MAMONY_IDENTITY_PERF_WORKERS"))
	if err != nil || (workers != 1 && workers != 4) {
		t.Fatal("set MAMONY_IDENTITY_PERF_WORKERS to 1 or 4 and use a fresh performance fixture")
	}
	if runtime.GOMAXPROCS(0) != cpuCount || debug.SetMemoryLimit(-1) != goMemory {
		t.Fatal("set GOMAXPROCS=2 and GOMEMLIMIT=256MiB for comparable runs")
	}

	ctx := t.Context()
	var endpoint string
	open := func(key, role string) *platformpg.Pool {
		t.Helper()
		dsn := fixture.GuardedDB(t, key, role)
		u, _ := url.Parse(dsn)
		if u.Path != "/mamony_test_identity_perf" {
			t.Fatal("the performance test requires the dedicated disposable database")
		}
		if endpoint == "" {
			endpoint = u.Host
		}
		if u.Host != endpoint {
			t.Fatal("all performance roles must use the same disposable database")
		}
		pool, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: 2})
		requireLoginPerf(t, err)
		t.Cleanup(pool.Close)
		return pool
	}
	regPool := open("TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	verifyPool := open("TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	authPool := open("TEST_IDENTITY_AUTHENTICATOR_URL", "mamony_identity_authenticator_runtime")

	kdf, err := credential.NewPasswordHasher(1)
	requireLoginPerf(t, err)
	registration, err := identitypg.NewRegistrationStore(regPool)
	requireLoginPerf(t, err)
	proofs, err := identitypg.NewProofStore(verifyPool)
	requireLoginPerf(t, err)
	sealer, err := mailcrypto.NewKeyring("perf-fixture", map[string][]byte{
		"perf-fixture": bytes.Repeat([]byte{0x79}, 32),
	})
	requireLoginPerf(t, err)
	emailVerification, err := usecase.NewEmailVerification(proofs, sealer)
	requireLoginPerf(t, err)

	const password = "synthetic benchmark Identity passphrase"
	users := make([]loginPerfActor, actors)
	for i := range users {
		user := loginPerfActor{
			email:   strings.ToLower(fixture.RandomID(t, "perf_")) + "@example.test",
			subject: fixture.RandomID(t, "sub_"),
			peer:    netip.AddrFrom4([4]byte{192, 0, 2, byte(i + 1)}),
		}
		verifier, err := kdf.Hash(ctx, password)
		requireLoginPerf(t, err)
		requireLoginPerf(t, registration.CreatePendingWithObligation(ctx, usecase.PendingRegistration{
			SubjectID: user.subject, EventID: fixture.RandomID(t, "evt_"),
			CanonicalEmail: user.email, Status: "UNVERIFIED", CreatedAt: time.Now().UTC(),
			PasswordVerifier: verifier,
		}))
		proof, err := emailVerification.Prepare(ctx, user.subject)
		requireLoginPerf(t, err)
		_, secret := proof.ForTrustedDelivery()
		requireLoginPerf(t, emailVerification.Confirm(ctx, secret))
		users[i] = user
	}

	admissionStore, err := identitypg.NewAdmissionStore(loginPerfTx{pool: regPool, admission: true})
	requireLoginPerf(t, err)
	admission, err := usecase.NewLoginAdmission(admissionStore, bytes.Repeat([]byte{0x27}, 32))
	requireLoginPerf(t, err)
	reader, err := identitypg.NewAuthenticationStore(loginPerfTx{pool: authPool})
	requireLoginPerf(t, err)
	auth, err := usecase.NewPasswordAuthentication(reader, loginPerfVerifier{hasher: kdf})
	requireLoginPerf(t, err)
	flow, err := usecase.NewPasswordLoginFlow(admission, auth)
	requireLoginPerf(t, err)

	login := func(user loginPerfActor, secret string, sample *loginPerfSample) (string, error) {
		start := time.Now()
		subject, err := flow.Authenticate(context.WithValue(ctx, loginPerfSampleKey{}, sample), user.email, secret, user.peer)
		sample.total = time.Since(start)
		return subject, err
	}
	for _, user := range users[:workers] {
		subject, err := login(user, password, &loginPerfSample{})
		requireLoginPerf(t, err)
		if subject != user.subject {
			t.Fatal("warm-up returned a different subject")
		}
	}

	samples := make([]loginPerfSample, attempts)
	failures := make([]error, attempts)
	start := make(chan struct{})
	var running sync.WaitGroup
	for worker := range workers {
		running.Go(func() {
			<-start
			for i := worker; i < attempts; i += workers {
				user := users[i%len(users)]
				subject, err := login(user, password, &samples[i])
				switch {
				case err != nil:
					failures[i] = err
				case subject != user.subject:
					failures[i] = errors.New("login returned a different subject")
				case samples[i].admissionDB <= 0 || samples[i].credentialDB <= 0 || samples[i].kdf <= 0:
					failures[i] = errors.New("login omitted a measured stage")
				}
			}
		})
	}
	started := time.Now()
	close(start)
	running.Wait()
	elapsed := time.Since(started)
	for i, err := range failures {
		if err != nil {
			t.Fatalf("measured login %d: %v", i, err)
		}
	}
	denied, err := login(users[workers], password+" incorrect", &loginPerfSample{})
	if denied != "" || !errors.Is(err, usecase.ErrPasswordAuthenticationDenied) {
		t.Fatal("wrong-password control did not return a generic denial")
	}

	t.Logf("identity_login_perf workers=%d successes=%d elapsed=%s successes_per_second=%.2f", workers, attempts, elapsed, attempts/elapsed.Seconds())
	for _, metric := range []struct {
		name string
		read func(loginPerfSample) time.Duration
	}{
		{"login", func(s loginPerfSample) time.Duration { return s.total }},
		{"admission_db_tx", func(s loginPerfSample) time.Duration { return s.admissionDB }},
		{"credential_db_tx", func(s loginPerfSample) time.Duration { return s.credentialDB }},
		{"db_checkout_and_begin", func(s loginPerfSample) time.Duration { return s.begin }},
		{"db_callbacks", func(s loginPerfSample) time.Duration { return s.sqlCallbacks }},
		{"kdf_including_slot_wait", func(s loginPerfSample) time.Duration { return s.kdf }},
	} {
		t.Logf("identity_login_perf %s %s", metric.name, loginPerfQuantiles(samples, metric.read))
	}
}

func requireLoginPerf(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func loginPerfQuantiles(samples []loginPerfSample, read func(loginPerfSample) time.Duration) string {
	values := make([]time.Duration, len(samples))
	for i, sample := range samples {
		values[i] = read(sample)
	}
	slices.Sort(values)
	at := func(p int) time.Duration {
		return values[(p*len(values)+99)/100-1].Round(time.Microsecond)
	}
	return fmt.Sprintf("p50=%s p95=%s p99=%s", at(50), at(95), at(99))
}
