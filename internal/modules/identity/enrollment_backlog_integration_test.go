//go:build integration

package identity

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailcrypto"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityZEnrollmentBacklogDB verifies the restricted-role COUNT-only
// inspection facade against real disposable PostgreSQL. It must not send mail
// or mutate an account just by looking at it.
func TestIdentityZEnrollmentBacklogDB(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := guardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	verifyDSN := guardedDB(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	root, _ := url.Parse(adminDSN)
	for _, raw := range []string{regDSN, verifyDSN} {
		u, _ := url.Parse(raw)
		if root.Host != u.Host || root.Path != u.Path {
			t.Fatal("fixture roles are not using the same disposable PostgreSQL database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		pool, err := platformpg.Open(ctx, platformpg.Config{
			DSN: dsn, RuntimeRole: role, MaxConns: 2,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		return pool
	}
	reg, err := identitypg.NewStore(open(regDSN, "mamony_identity_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	proof, err := identitypg.NewProofStore(open(verifyDSN, "mamony_identity_verifier_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := InspectEnrollmentBacklog(ctx, verifyDSN)
	if err != nil {
		t.Fatal(err)
	}
	newPending := func(created time.Time) string {
		t.Helper()
		subject := randomSyntheticID(t, "sub_")
		email := strings.ToLower(randomSyntheticID(t, "backlog_")) + "@example.test"
		if err := reg.CreatePendingWithObligation(ctx, usecase.PendingRegistration{
			SubjectID: subject, EventID: randomSyntheticID(t, "evt_"),
			CanonicalEmail: email, Status: "UNVERIFIED", CreatedAt: created,
			PasswordVerifier: domain.PasswordVerifier{
				Version: 1, MemoryKiB: 65536, Iterations: 3, Parallelism: 1,
				Salt: bytes.Repeat([]byte{0x19}, 16), Digest: bytes.Repeat([]byte{0x32}, 32),
			},
		}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			// Quarantine synthetic queue rows before any later integration
			// test may start a Mailgun mock worker in the same test database.
			cleanCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			if _, err := admin.Exec(cleanCtx, `UPDATE mamony_identity.email_proof_deliveries d
				SET status='DEAD' FROM mamony_identity.email_verification_challenges c
				WHERE d.challenge_id=c.challenge_id AND c.subject_id=$1
				  AND d.status='PENDING'`, subject); err != nil {
				t.Errorf("failed to quarantine synthetic backlog envelope: %v", err)
			}
		})
		return subject
	}
	eligible := newPending(time.Now().UTC().Add(-5 * time.Minute))
	_ = newPending(time.Now().UTC()) // not yet old enough for reconciliation
	_ = newPending(time.Now().UTC().Add(-26 * time.Hour)) // stale: manual intervention
	observed, err := InspectEnrollmentBacklog(ctx, verifyDSN)
	if err != nil {
		t.Fatal(err)
	}
	if observed.EligibleSampled != baseline.EligibleSampled+1 || !observed.StalePresent {
		t.Fatalf("backlog boundaries or aggregate counts invalid: baseline=%+v observed=%+v",
			baseline, observed)
	}
	// Read-only inspection is not permitted to issue a challenge.
	var challengeCount int
	if err := admin.QueryRow(ctx, `SELECT count(*)
		FROM mamony_identity.email_verification_challenges WHERE subject_id=$1`,
		eligible).Scan(&challengeCount); err != nil || challengeCount != 0 {
		t.Fatalf("inspector unexpectedly issued a proof: %d %v", challengeCount, err)
	}
	keys, err := mailcrypto.NewKeyring("synthetic-watch", map[string][]byte{
		"synthetic-watch": bytes.Repeat([]byte{0xf1}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := usecase.NewEmailVerification(proof, keys)
	if err != nil {
		t.Fatal(err)
	}
	if err := issuer.FirstIfMissing(ctx, eligible); err != nil {
		t.Fatal(err)
	}
	after, err := InspectEnrollmentBacklog(ctx, verifyDSN)
	if err != nil {
		t.Fatal(err)
	}
	if after.EligibleSampled != baseline.EligibleSampled || !after.StalePresent {
		t.Fatalf("issued first proof not reflected in capped backlog: %+v", after)
	}
	if _, err := InspectEnrollmentBacklog(ctx, regDSN); err == nil {
		t.Fatal("registration role unexpectedly became a monitoring authority")
	}
}
