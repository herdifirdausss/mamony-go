//go:build integration

package integration

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/integration/internal/fixture"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailcrypto"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

type staleRepairFinder struct{ subject string }

func (f staleRepairFinder) FindUnissuedPending(context.Context, int) ([]string, error) {
	return []string{f.subject}, nil
}

// TestIdentityZEnrollmentReconciliationDB exercises the actual verifier role,
// encrypted proof obligation and real PostgreSQL. No real provider is used.
func TestIdentityZEnrollmentReconciliationDB(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminDSN := fixture.GuardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := fixture.GuardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	verifyDSN := fixture.GuardedDB(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	root, _ := url.Parse(adminDSN)
	for _, raw := range []string{regDSN, verifyDSN} {
		u, _ := url.Parse(raw)
		if root.Host != u.Host || root.Path != u.Path {
			t.Fatal("test roles must target identical disposable PostgreSQL")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		pool, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: 2})
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
	keyBytes := bytes.Repeat([]byte{0xd1}, 32)
	keys, err := mailcrypto.NewKeyring("synthetic-repair", map[string][]byte{"synthetic-repair": keyBytes})
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := usecase.NewEmailVerification(proof, keys)
	if err != nil {
		t.Fatal(err)
	}
	newPending := func(created time.Time) string {
		t.Helper()
		subject := fixture.RandomID(t, "sub_")
		email := strings.ToLower(fixture.RandomID(t, "repair_")) + "@example.test"
		err := reg.CreatePendingWithObligation(ctx, usecase.PendingRegistration{
			SubjectID: subject, EventID: fixture.RandomID(t, "evt_"),
			CanonicalEmail: email, Status: "UNVERIFIED", CreatedAt: created,
			PasswordVerifier: domain.PasswordVerifier{
				Version: 1, MemoryKiB: 65536, Iterations: 3, Parallelism: 1,
				Salt: bytes.Repeat([]byte{0x28}, 16), Digest: bytes.Repeat([]byte{0x59}, 32),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			if _, err := admin.Exec(cleanCtx, `UPDATE mamony_identity.email_proof_deliveries d
				SET status='DEAD'
				FROM mamony_identity.email_verification_challenges c
				WHERE d.challenge_id=c.challenge_id AND c.subject_id=$1
				  AND d.status='PENDING'`, subject); err != nil {
				t.Errorf("cannot quarantine synthetic repair mail: %v", err)
			}
		})
		return subject
	}
	old := time.Now().UTC().Add(-4 * time.Minute)
	gap := newPending(old) // simulates a crash after registration, before proof issuance
	withProof := newPending(old)
	if err := issuer.Resend(ctx, withProof); err != nil {
		t.Fatal(err)
	}
	tooFresh := newPending(time.Now().UTC())
	tooOld := newPending(time.Now().UTC().Add(-25 * time.Hour))
	// Use the same verifier role and real AES-GCM sealer that the existing
	// registration/worker pipeline uses; no Mailgun API or browser routes.
	result, err := identity.ReconcileEnrollmentOnce(ctx, identity.EnrollmentReconciliationConfig{
		DSN: verifyDSN, ActiveKeyID: "synthetic-repair",
		EnvelopeKeys: map[string][]byte{"synthetic-repair": keyBytes},
	})
	if err != nil || result.Issued < 1 {
		t.Fatalf("orphaned first-proof reconciliation failed: %+v err=%v", result, err)
	}
	proofsFor := func(subject string) int {
		t.Helper()
		var count int
		if err := admin.QueryRow(ctx, `SELECT count(*)
			FROM mamony_identity.email_verification_challenges c
			JOIN mamony_identity.email_proof_deliveries d USING(challenge_id)
			WHERE c.subject_id=$1 AND octet_length(d.sealed_token)>=60`, subject).
			Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	if proofsFor(gap) != 1 || proofsFor(withProof) != 1 || proofsFor(tooFresh) != 0 || proofsFor(tooOld) != 0 {
		t.Fatal("repair reissued existing proof or selected too-recent registration")
	}
	// Simulate a stale finder that observed a missing first proof just before
	// another legitimate issuer committed. The conditional check must repeat
	// under the same subject lock and refuse all proof rotation.
	stale, err := usecase.NewEnrollmentReconciler(staleRepairFinder{subject: withProof}, issuer)
	if err != nil {
		t.Fatal(err)
	}
	staleResult, err := stale.ReconcileOnce(ctx)
	if err != nil || staleResult.Skipped != 1 || staleResult.Issued != 0 ||
		proofsFor(withProof) != 1 {
		t.Fatalf("stale candidate rotated a previously committed challenge: %+v %v", staleResult, err)
	}
	// The repaired subject has exactly one durable issuance event and the
	// encrypted delivery obligation, not an authenticated session.
	var events int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.security_outbox
		WHERE subject_id=$1 AND event_type='EMAIL_PROOF_ISSUED'`, gap).
		Scan(&events); err != nil || events != 1 {
		t.Fatalf("repair did not durably append proof event: %d %v", events, err)
	}
	if _, err := identity.ReconcileEnrollmentOnce(ctx, identity.EnrollmentReconciliationConfig{
		DSN: verifyDSN, ActiveKeyID: "synthetic-repair",
		EnvelopeKeys: map[string][]byte{"synthetic-repair": keyBytes},
	}); err != nil {
		t.Fatal("repeating a safe scan must be allowed")
	}
	if proofsFor(gap) != 1 {
		t.Fatal("repeat reconciliation minted an extra proof")
	}
	if _, err := identity.ReconcileEnrollmentOnce(ctx, identity.EnrollmentReconciliationConfig{
		DSN: verifyDSN, ActiveKeyID: "synthetic-repair",
		EnvelopeKeys: map[string][]byte{"synthetic-repair": []byte("short")},
	}); err == nil {
		t.Fatal("invalid envelope key should fail closed")
	}
}
