//go:build integration

package postgres

import (
	"bytes"
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityRetentionPurge proves the effective maintenance-only credential
// cannot alter active proof state, and cleanup preserves nonsecret evidence.
// Every fixture targets disposable, loopback-only PostgreSQL.
func TestIdentityRetentionPurge(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	adminDSN := fixtureDSN(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := fixtureDSN(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	verifyDSN := fixtureDSN(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	maintenanceDSN := fixtureDSN(t, "TEST_IDENTITY_MAINTENANCE_URL", "mamony_identity_maintenance_runtime")
	root, _ := url.Parse(adminDSN)
	for _, dsn := range []string{regDSN, verifyDSN, maintenanceDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != root.Host || u.Path != root.Path {
			t.Fatal("retention fixture must use the same isolated database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	// Register close before test-fixture cleanup. t.Cleanup runs callbacks
	// in LIFO order, so the synthetic pending jobs are marked DEAD while
	// this admin connection is still open.
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	open := func(dsn, role string) *platformpg.Pool {
		p, err := platformpg.Open(ctx, platformpg.Config{
			DSN: dsn, RuntimeRole: role, MaxConns: 4,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	reg, err := NewRegistrationStore(open(regDSN, "mamony_identity_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	verify, err := NewProofStore(open(verifyDSN, "mamony_identity_verifier_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	maintenance, err := NewRetentionStore(open(maintenanceDSN, "mamony_identity_maintenance_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	create := func(t *testing.T, seed byte) (string, string) {
		t.Helper()
		p := syntheticPending(t, syntheticEmail(t))
		if err := reg.CreatePendingWithObligation(ctx, p); err != nil {
			t.Fatal(err)
		}
		challenge := randomID(t, "vch_")
		_, err := verify.Issue(ctx, usecase.EmailProofRequest{
			SubjectID: p.SubjectID, ChallengeID: challenge,
			EventID: randomID(t, "evt_"), TokenDigest: bytes.Repeat([]byte{seed}, 32),
			EnvelopeKeyID: "synthetic", SealedToken: bytes.Repeat([]byte{seed}, 71),
		})
		if err != nil {
			t.Fatal(err)
		}
		return p.SubjectID, challenge
	}
	oldSubject, old := create(t, 0xe1)
	_, live := create(t, 0xe2)
	_, leased := create(t, 0xe3)
	// The next suite's local Mailgun worker must never claim synthetic
	// retention fixtures that were intentionally left PENDING here.
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		for _, challenge := range []string{old, live, leased} {
			if _, err := admin.Exec(cleanupCtx, `UPDATE mamony_identity.email_proof_deliveries
				SET status='DEAD' WHERE challenge_id=$1 AND status='PENDING'`, challenge); err != nil {
				t.Errorf("cleanup of isolated synthetic proof failed: %v", err)
			}
		}
	})
	// Simulate expired proofs using administrator-only test fixture writes.
	// Proof/challenge integrity and timestamp constraints still apply.
	for _, challenge := range []string{old, leased} {
		if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_verification_challenges
			SET requested_at=clock_timestamp()-interval '49 hours',
			    expires_at=clock_timestamp()-interval '48 hours'
			WHERE challenge_id=$1`, challenge); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_proof_deliveries
		SET lease_id=$2, leased_until=clock_timestamp()+interval '10 minutes'
		WHERE challenge_id=$1`, leased, randomID(t, "lse_")); err != nil {
		t.Fatal(err)
	}
	// Synthetic fixed-window counters: 110 stale, 1 recent. Avoid consuming
	// admission quota and preserve other suites' historical accounting.
	for i := 0; i < 111; i++ {
		key := bytes.Repeat([]byte{byte(i + 1)}, 32)
		when := "50 hours"
		if i == 110 {
			when = "0 hours"
		}
		_, err := admin.Exec(ctx, `INSERT INTO mamony_identity.anonymous_admission_buckets
			(action,scope,key_digest,window_start,hits)
			VALUES ('REGISTER','CLIENT_MINUTE',$1,
				clock_timestamp() - make_interval(hours => $2::int),1)`, key, map[string]int{"50 hours": 50, "0 hours": 0}[when])
		if err != nil {
			t.Fatal(err)
		}
	}
	n, c, err := maintenance.PurgeOneBatch(ctx)
	if err != nil || n != 1 || c != 100 {
		t.Fatalf("first bounded retention run erased wrong rows: envelopes=%d counters=%d err=%v", n, c, err)
	}
	var ciphertextMissing, purged, liveCiphertext bool
	var preserved int
	err = admin.QueryRow(ctx, `SELECT d.sealed_token IS NULL, d.purged_at IS NOT NULL,
		(SELECT count(*) FROM mamony_identity.security_outbox
		WHERE subject_id=$2 AND event_type='EMAIL_PROOF_ISSUED')
		FROM mamony_identity.email_proof_deliveries d
		WHERE d.challenge_id=$1`, old, oldSubject).Scan(&ciphertextMissing, &purged, &preserved)
	if err != nil || !ciphertextMissing || !purged || preserved != 1 {
		t.Fatalf("old encrypted proof not erased or event evidence lost: %v %v %d %v",
			ciphertextMissing, purged, preserved, err)
	}
	if err := admin.QueryRow(ctx, `SELECT sealed_token IS NOT NULL
		FROM mamony_identity.email_proof_deliveries WHERE challenge_id=$1`, live).
		Scan(&liveCiphertext); err != nil || !liveCiphertext {
		t.Fatalf("live proof was erased: %v %v", liveCiphertext, err)
	}
	if err := admin.QueryRow(ctx, `SELECT sealed_token IS NOT NULL
		FROM mamony_identity.email_proof_deliveries WHERE challenge_id=$1`, leased).
		Scan(&liveCiphertext); err != nil || !liveCiphertext {
		t.Fatalf("actively leased envelope was erased: %v %v", liveCiphertext, err)
	}
	n, c, err = maintenance.PurgeOneBatch(ctx)
	if err != nil || n != 0 || c != 10 {
		t.Fatalf("second batch should finish stale counters: %d %d %v", n, c, err)
	}
	n, c, err = maintenance.PurgeOneBatch(ctx)
	if err != nil || n != 0 || c != 0 {
		t.Fatalf("purge should be idempotent: %d %d %v", n, c, err)
	}
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_proof_deliveries
		SET leased_until=clock_timestamp()-interval '1 second' WHERE challenge_id=$1`, leased); err != nil {
		t.Fatal(err)
	}
	n, c, err = maintenance.PurgeOneBatch(ctx)
	if err != nil || n != 1 || c != 0 {
		t.Fatalf("expired lease did not permit later purge: %d %d %v", n, c, err)
	}
	var kept int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.anonymous_admission_buckets
		WHERE action='REGISTER' AND scope='CLIENT_MINUTE' AND
		key_digest=$1`, bytes.Repeat([]byte{111}, 32)).Scan(&kept); err != nil || kept != 1 {
		t.Fatalf("recent admission window erased: count=%d err=%v", kept, err)
	}
}
