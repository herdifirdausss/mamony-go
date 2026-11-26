//go:build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/integration/internal/fixture"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailcrypto"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

func TestIdentityMailgunMockRecoveryDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 105*time.Second)
	defer cancel()
	adminDSN := fixture.GuardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	issuerDSN := fixture.GuardedDB(t, "TEST_IDENTITY_RECOVERY_ISSUER_URL", "mamony_identity_recovery_issuer_runtime")
	workerDSN := fixture.GuardedDB(t, "TEST_IDENTITY_RECOVERY_DELIVERY_URL", "mamony_identity_recovery_delivery_runtime")
	admitDSN := fixture.GuardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	original, _ := url.Parse(adminDSN)
	for _, raw := range []string{issuerDSN, workerDSN, admitDSN} {
		u, _ := url.Parse(raw)
		if u.Host != original.Host || u.Path != original.Path {
			t.Fatal("recovery provider E2E must use one disposable DB")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	open := func(raw, role string) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{DSN: raw, RuntimeRole: role, MaxConns: 2})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	issuerStore, err := identitypg.NewRecoveryIssuerStore(open(issuerDSN, "mamony_identity_recovery_issuer_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	admissionStore, err := identitypg.NewAdmissionStore(open(admitDSN, "mamony_identity_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	gate, err := usecase.NewRecoveryAdmission(admissionStore, bytes.Repeat([]byte{0x91}, 32))
	if err != nil {
		t.Fatal(err)
	}
	secret := bytes.Repeat([]byte{0x79}, 32)
	ring, err := mailcrypto.NewRecoveryKeyring("recovery-fixture", map[string][]byte{
		"recovery-fixture": secret,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := usecase.NewRecoveryRequest(gate, issuerStore, ring)
	if err != nil {
		t.Fatal(err)
	}
	subject := fixture.RandomID(t, "sub_")
	email := strings.ToLower(fixture.RandomID(t, "recovery_")) + "@example.test"
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES($1,clock_timestamp())",
		subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.email_aliases
		(canonical_email,subject_id,verified_at,created_at)
		VALUES($1,$2,clock_timestamp(),clock_timestamp())`, email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	if err := request.Request(ctx, email, netip.MustParseAddr("192.0.2.213")); err != nil {
		t.Fatal("genuine AES-GCM recovery issuance failed")
	}
	var challenge string
	var digest, encrypted []byte
	if err := admin.QueryRow(ctx, `SELECT c.challenge_id,c.token_digest,d.sealed_token
		FROM mamony_identity.password_recovery_challenges c
		JOIN mamony_identity.password_recovery_deliveries d USING(challenge_id)
		WHERE c.subject_id=$1 AND d.status='PENDING'`, subject).
		Scan(&challenge, &digest, &encrypted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		if _, err := admin.Exec(cleanupCtx, `UPDATE mamony_identity.password_recovery_deliveries
			SET status='DEAD',lease_id=NULL,leased_until=NULL
			WHERE challenge_id=$1 AND status='PENDING'`, challenge); err != nil {
			t.Errorf("could not quarantine recovery mail fixture: %v", err)
		}
	}()
	if len(encrypted) != 71 || len(digest) != 32 {
		t.Fatal("recovery queue did not store AEAD ciphertext plus only digest")
	}
	// A request to a non-existent email must run the same internal
	// cryptographic pipeline without creating a database challenge.
	unknown := strings.ToLower(fixture.RandomID(t, "unknown_")) + "@example.test"
	if err := request.Request(ctx, unknown, netip.MustParseAddr("192.0.2.214")); err != nil {
		t.Fatal("unknown recovery alias leaked existence through usecase")
	}
	var unknownCount int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.password_recovery_challenges
		WHERE canonical_email=$1`, unknown).Scan(&unknownCount); err != nil || unknownCount != 0 {
		t.Fatal("unknown account unexpectedly received a durable token")
	}
	var calls atomic.Int32
	var providerCode atomic.Int32
	providerCode.Store(429)
	var mu sync.Mutex
	var submitted []string
	fragment := regexp.MustCompile("#token=([A-Za-z0-9_-]{43})")
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		user, key, ok := r.BasicAuth()
		if !ok || user != "api" || key != "synthetic-recovery-sending-key" ||
			r.Method != http.MethodPost || r.URL.Path != "/v3/mg.example.test/messages" {
			t.Error("recovery Mailgun request used an untrusted destination or authorization")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if err := r.ParseMultipartForm(8192); err != nil {
			t.Error(err)
			return
		}
		body := r.FormValue("text")
		match := fragment.FindStringSubmatch(body)
		if r.FormValue("to") != email ||
			r.FormValue("subject") != "Mamony account password recovery" ||
			r.FormValue("o:tracking") != "no" ||
			r.FormValue("o:tracking-opens") != "no" ||
			r.FormValue("o:tracking-clicks") != "no" ||
			r.FormValue("o:require-tls") != "yes" ||
			r.FormValue("o:deliver-within") != "5m" ||
			!strings.Contains(body, "https://app.example.test/account/recover-password#token=") ||
			len(match) != 2 {
			t.Error("recovery Mailgun mock received unexpected token-bearing message")
		}
		if len(match) == 2 {
			mu.Lock()
			submitted = append(submitted, match[1])
			mu.Unlock()
		}
		status := int(providerCode.Load())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = fmt.Fprint(w, `{"id":"<fixture-recovery@mailgun.test>","message":"Queued"}`)
		} else {
			_, _ = fmt.Fprint(w, `{"message":"synthetic recovery throttle"}`)
		}
	}))
	defer mock.Close()
	mockOrigin, err := url.Parse(mock.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := identity.RecoveryDeliveryConfig{
		DSN: workerDSN, Region: "US", Domain: "mg.example.test",
		From:         "Mamony Security <security@mg.example.test>",
		MailgunKey:   "synthetic-recovery-sending-key",
		RecoveryURL:  "https://app.example.test/account/recover-password",
		ActiveKeyID:  "recovery-fixture",
		EnvelopeKeys: map[string][]byte{"recovery-fixture": secret},
		PollInterval: 100 * time.Millisecond,
		HTTPClient: &http.Client{Timeout: 2 * time.Second,
			Transport: fixture.MailgunTransport{Origin: mockOrigin}},
	}
	state := func() (string, int) {
		t.Helper()
		var s string
		var attempts int
		if err := admin.QueryRow(ctx, `SELECT status,attempts
			FROM mamony_identity.password_recovery_deliveries WHERE challenge_id=$1`,
			challenge).Scan(&s, &attempts); err != nil {
			t.Fatal(err)
		}
		return s, attempts
	}
	runUntil := func(want string, attempts int) {
		t.Helper()
		runCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- identity.RunRecoveryDelivery(runCtx, cfg) }()
		deadline := time.Now().Add(8 * time.Second)
		for {
			got, n := state()
			if got == want && n >= attempts {
				break
			}
			if time.Now().After(deadline) {
				stop()
				t.Fatalf("recovery provider pipeline stuck at %s/%d; wanted %s/%d",
					got, n, want, attempts)
			}
			time.Sleep(25 * time.Millisecond)
		}
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("recovery delivery facade failed cancellation: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("recovery delivery facade ignored cancellation")
		}
	}
	runUntil("PENDING", 1)
	if calls.Load() != 1 {
		t.Fatalf("429 should produce exactly one provider attempt, got %d", calls.Load())
	}
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_recovery_deliveries
		SET next_attempt_at=clock_timestamp()-interval '1 second'
		WHERE challenge_id=$1`, challenge); err != nil {
		t.Fatal(err)
	}
	providerCode.Store(200)
	runUntil("ACCEPTED", 2)
	if calls.Load() != 2 {
		t.Fatal("429/200 simulated provider flow did not send exactly twice")
	}
	mu.Lock()
	seen := append([]string(nil), submitted...)
	mu.Unlock()
	if len(seen) != 2 || seen[0] != seen[1] {
		t.Fatal("retry sent different recovery bearer or did not send twice")
	}
	derived, err := usecase.PasswordRecoveryProofDigest(seen[0])
	if err != nil || !bytes.Equal(derived[:], digest) ||
		bytes.Contains(encrypted, []byte(seen[0])) {
		t.Fatal("provider received wrong bearer, or database stored plaintext")
	}
	var epoch int64
	if err := admin.QueryRow(ctx,
		"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
		subject).Scan(&epoch); err != nil || epoch < 1 {
		t.Fatal("recovery delivery unexpectedly changed an account session")
	}
	// No redemption operation is introduced: ACCEPTED is provider submission
	// evidence, not password reset, account login or inbox delivery.
}
