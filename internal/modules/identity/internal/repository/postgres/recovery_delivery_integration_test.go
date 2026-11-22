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

func TestIdentityRecoveryDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	adminDSN := fixtureDSN(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	issuerDSN := fixtureDSN(t, "TEST_IDENTITY_RECOVERY_ISSUER_URL", "mamony_identity_recovery_issuer_runtime")
	workerDSN := fixtureDSN(t, "TEST_IDENTITY_RECOVERY_DELIVERY_URL", "mamony_identity_recovery_delivery_runtime")
	base, _ := url.Parse(adminDSN)
	for _, dsn := range []string{issuerDSN, workerDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != base.Host || u.Path != base.Path {
			t.Fatal("recovery sender roles require same disposable database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		pool, err := platformpg.Open(ctx, platformpg.Config{
			DSN: dsn, RuntimeRole: role, MaxConns: 4,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		return pool
	}
	issuer, err := NewRecoveryIssuerStore(open(issuerDSN, "mamony_identity_recovery_issuer_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := NewRecoveryDeliveryStore(open(workerDSN, "mamony_identity_recovery_delivery_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	newJob := func(n byte) (string, string, string) {
		t.Helper()
		subject, email := randomID(t, "sub_"), syntheticEmail(t)
		if _, err := admin.Exec(ctx,
			"INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES ($1,clock_timestamp())",
			subject); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.email_aliases
			(canonical_email,subject_id,verified_at,created_at)
			VALUES($1,$2,clock_timestamp(),clock_timestamp())`, email, subject); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx,
			"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1",
			subject); err != nil {
			t.Fatal(err)
		}
		id := randomID(t, "rcv_")
		var digest [32]byte
		digest[0] = n
		issued, err := issuer.Issue(ctx, usecase.RecoveryIssue{
			CanonicalEmail: email, ChallengeID: id, TokenDigest: digest,
			KeyID: "mock-recovery", SealedToken: bytes.Repeat([]byte{n}, 71),
		})
		if err != nil || !issued {
			t.Fatalf("fixture could not issue recovery challenge: %v %v", issued, err)
		}
		t.Cleanup(func() {
			cleanupCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
			defer done()
			if _, err := admin.Exec(cleanupCtx,
				"UPDATE mamony_identity.password_recovery_deliveries SET status='DEAD',lease_id=NULL,leased_until=NULL WHERE challenge_id=$1 AND status='PENDING'",
				id); err != nil {
				t.Errorf("quarantine recovery fixture: %v", err)
			}
		})
		return subject, email, id
	}

	t.Run("fenced lease, bounded retry and accepted finalization", func(t *testing.T) {
		_, email, challenge := newJob(0x31)
		lease := randomID(t, "lse_")
		job, ok, err := delivery.Claim(ctx, lease)
		if err != nil || !ok || job.ChallengeID != challenge ||
			job.Recipient != email || job.LeaseID != lease ||
			job.KeyID != "mock-recovery" || len(job.SealedToken) != 71 ||
			!job.ExpiresAt.After(time.Now().Add(10*time.Minute)) {
			t.Fatalf("recovery claim lost verified contact or ciphertext: ok=%v err=%v", ok, err)
		}
		if _, ok, err := delivery.Claim(ctx, randomID(t, "lse_")); err != nil || ok {
			t.Fatal("leased recovery job was claimed twice")
		}
		if err := delivery.Finalize(ctx, challenge, randomID(t, "lse_"), usecase.DeliveryAccepted); err == nil {
			t.Fatal("wrong lease finalized recovery job")
		}
		if err := delivery.Finalize(ctx, challenge, lease, usecase.DeliveryTemporary); err != nil {
			t.Fatal(err)
		}
		var status string
		var attempts int
		if err := admin.QueryRow(ctx,
			"SELECT status,attempts FROM mamony_identity.password_recovery_deliveries WHERE challenge_id=$1",
			challenge).Scan(&status, &attempts); err != nil ||
			status != "PENDING" || attempts != 1 {
			t.Fatalf("temporary provider failure not durably retried: %s %d %v", status, attempts, err)
		}
		if _, ok, err := delivery.Claim(ctx, randomID(t, "lse_")); err != nil || ok {
			t.Fatal("recovery retry did not honor backoff")
		}
		if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_recovery_deliveries
			SET next_attempt_at=clock_timestamp()-interval '1 second'
			WHERE challenge_id=$1`, challenge); err != nil {
			t.Fatal(err)
		}
		secondLease := randomID(t, "lse_")
		job, ok, err = delivery.Claim(ctx, secondLease)
		if err != nil || !ok || job.ChallengeID != challenge || job.LeaseID == lease {
			t.Fatal("recovery retry did not fence the prior worker")
		}
		if err := delivery.Finalize(ctx, challenge, lease, usecase.DeliveryAccepted); err == nil {
			t.Fatal("expired lease accepted an obsolete finalization")
		}
		if err := delivery.Finalize(ctx, challenge, secondLease, usecase.DeliveryAccepted); err != nil {
			t.Fatal(err)
		}
		if err := admin.QueryRow(ctx, `SELECT status,attempts FROM mamony_identity.password_recovery_deliveries
			WHERE challenge_id=$1`, challenge).Scan(&status, &attempts); err != nil ||
			status != "ACCEPTED" || attempts != 2 {
			t.Fatalf("recovery provider acknowledgement not committed: %s %d %v", status, attempts, err)
		}
		if _, ok, err := delivery.Claim(ctx, randomID(t, "lse_")); err != nil || ok {
			t.Fatal("accepted recovery job was redelivered")
		}
	})

	t.Run("stale subject and expired challenge are quarantined", func(t *testing.T) {
		subject, _, challenge := newJob(0x32)
		if _, err := admin.Exec(ctx,
			"UPDATE mamony_identity.subjects SET status='SUSPENDED' WHERE subject_id=$1",
			subject); err != nil {
			t.Fatal(err)
		}
		job, ok, err := delivery.Claim(ctx, randomID(t, "lse_"))
		if err != nil || ok || job.Recipient != "" || len(job.SealedToken) != 0 {
			t.Fatalf("revoked recovery contact was exposed to sender: ok=%v err=%v", ok, err)
		}
		var state string
		if err := admin.QueryRow(ctx,
			"SELECT status FROM mamony_identity.password_recovery_deliveries WHERE challenge_id=$1",
			challenge).Scan(&state); err != nil || state != "DEAD" {
			t.Fatalf("stale challenge not quarantined: %s %v", state, err)
		}
		_, _, expiring := newJob(0x33)
		if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_recovery_challenges
			SET expires_at=clock_timestamp()-interval '1 second'
			WHERE challenge_id=$1`, expiring); err != nil {
			t.Fatal(err)
		}
		job, ok, err = delivery.Claim(ctx, randomID(t, "lse_"))
		if err != nil || ok || job.Recipient != "" || len(job.SealedToken) != 0 {
			t.Fatal("expired recovery challenge was delivered")
		}
		if err := admin.QueryRow(ctx,
			"SELECT status FROM mamony_identity.password_recovery_deliveries WHERE challenge_id=$1",
			expiring).Scan(&state); err != nil || state != "DEAD" {
			t.Fatal("expired ciphertext was not quarantined")
		}
	})
}
