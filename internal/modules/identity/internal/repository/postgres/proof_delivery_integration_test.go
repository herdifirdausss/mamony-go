//go:build integration

package postgres

import (
	"bytes"
	"context"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

func TestIdentityProofDeliveryOutbox(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	adminDSN := fixtureDSN(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := fixtureDSN(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	verifyDSN := fixtureDSN(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	deliveryDSN := fixtureDSN(t, "TEST_IDENTITY_DELIVERY_URL", "mamony_identity_delivery_runtime")
	a, _ := url.Parse(adminDSN)
	for _, dsn := range []string{regDSN, verifyDSN, deliveryDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != a.Host || u.Path != a.Path {
			t.Fatal("unsafe fixture database mismatch")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	open := func(dsn, role string) *platformpg.Pool {
		p, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: 4})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	reg, err := NewStore(open(regDSN, "mamony_identity_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewProofStore(open(verifyDSN, "mamony_identity_verifier_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := NewDeliveryStore(open(deliveryDSN, "mamony_identity_delivery_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	newPending := func(t *testing.T) usecase.PendingRegistration {
		t.Helper()
		p := syntheticPending(t, syntheticEmail(t))
		if err := reg.CreatePendingWithObligation(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	newIssue := func(t *testing.T, subject string, n byte) usecase.EmailProofRequest {
		t.Helper()
		p := usecase.EmailProofRequest{
			SubjectID: subject, ChallengeID: randomID(t, "vch_"),
			EventID: randomID(t, "evt_"), TokenDigest: bytes.Repeat([]byte{n}, 32),
			EnvelopeKeyID: "test-key", SealedToken: bytes.Repeat([]byte{n}, 71),
		}
		if _, err := issuer.Issue(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("transaction commits encrypted queue with digest and audit", func(t *testing.T) {
		p := newPending(t)
		issue := newIssue(t, p.SubjectID, 0xd1)
		var count int
		err := admin.QueryRow(ctx, `SELECT count(*) FROM
			mamony_identity.email_verification_challenges c
			JOIN mamony_identity.email_proof_deliveries d USING(challenge_id)
			JOIN mamony_identity.security_outbox o ON o.subject_id=c.subject_id
			WHERE c.challenge_id=$1 AND c.purpose='EMAIL_VERIFY'
			AND d.status='PENDING' AND d.attempts=0
			AND octet_length(c.token_digest)=32
			AND octet_length(d.sealed_token)=71 AND d.key_id='test-key'
			AND o.event_type='EMAIL_PROOF_ISSUED'`, issue.ChallengeID).Scan(&count)
		if err != nil || count != 1 {
			t.Fatalf("durable envelope not linked: count=%d err=%v", count, err)
		}
		// Simulate failure of the LAST audit insert; the envelope and challenge
		// must not survive the same failed issuance transaction.
		conflict := usecase.EmailProofRequest{
			SubjectID: p.SubjectID, ChallengeID: randomID(t, "vch_"),
			EventID: issue.EventID, TokenDigest: bytes.Repeat([]byte{0xd2}, 32),
			EnvelopeKeyID: "test-key", SealedToken: bytes.Repeat([]byte{0xd2}, 71),
		}
		if _, err := issuer.Issue(ctx, conflict); err == nil {
			t.Fatal("outbox collision accepted")
		}
		err = admin.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM mamony_identity.email_proof_deliveries WHERE challenge_id=$1) +
			(SELECT count(*) FROM mamony_identity.email_verification_challenges WHERE challenge_id=$1)`, conflict.ChallengeID).Scan(&count)
		if err != nil || count != 0 {
			t.Fatalf("envelope or challenge escaped rollback: %d %v", count, err)
		}
		// Keep later subtests isolated: a pending queue can be globally claimed.
		if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_proof_deliveries
			SET status='DEAD' WHERE challenge_id=$1`, issue.ChallengeID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("lease fences concurrent workers and retries", func(t *testing.T) {
		p := newPending(t)
		issue := newIssue(t, p.SubjectID, 0xd3)
		first, ok, err := delivery.Claim(ctx, randomID(t, "lse_"))
		if err != nil || !ok || first.ChallengeID != issue.ChallengeID ||
			first.SubjectID != p.SubjectID || first.Recipient != p.CanonicalEmail ||
			!bytes.Equal(first.SealedToken, issue.SealedToken) {
			t.Fatal("claim did not bind encrypted pending delivery")
		}
		if _, ok, err := delivery.Claim(ctx, randomID(t, "lse_")); err != nil || ok {
			t.Fatalf("already leased job was reclaimed: ok=%v err=%v", ok, err)
		}
		if err := delivery.Finalize(ctx, first.ChallengeID, randomID(t, "lse_"), usecase.DeliveryAccepted); err == nil {
			t.Fatal("stale lease completion allowed")
		}
		if err := delivery.Finalize(ctx, first.ChallengeID, first.LeaseID, usecase.DeliveryTemporary); err != nil {
			t.Fatal(err)
		}
		var status string
		var attempts int
		err = admin.QueryRow(ctx, `SELECT status,attempts FROM mamony_identity.email_proof_deliveries
			WHERE challenge_id=$1`, issue.ChallengeID).Scan(&status, &attempts)
		if err != nil || status != "PENDING" || attempts != 1 {
			t.Fatal("transient retry state invalid")
		}
		if _, ok, err := delivery.Claim(ctx, randomID(t, "lse_")); err != nil || ok {
			t.Fatal("backoff was bypassed")
		}
		if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_proof_deliveries
			SET next_attempt_at=clock_timestamp()-interval '1 second'
			WHERE challenge_id=$1`, issue.ChallengeID); err != nil {
			t.Fatal(err)
		}
		second, ok, err := delivery.Claim(ctx, randomID(t, "lse_"))
		if err != nil || !ok || second.LeaseID == first.LeaseID {
			t.Fatal("retry not reclaimed with new lease")
		}
		if err := delivery.Finalize(ctx, second.ChallengeID, second.LeaseID, usecase.DeliveryAccepted); err != nil {
			t.Fatal(err)
		}
		err = admin.QueryRow(ctx, `SELECT status,attempts FROM mamony_identity.email_proof_deliveries
			WHERE challenge_id=$1`, issue.ChallengeID).Scan(&status, &attempts)
		if err != nil || status != "ACCEPTED" || attempts != 2 {
			t.Fatal("provider acceptance state invalid")
		}
	})

	t.Run("fifth retry ends terminally without unbounded sends", func(t *testing.T) {
		p := newPending(t)
		issue := newIssue(t, p.SubjectID, 0xd4)
		for i := 0; i < 5; i++ {
			if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_proof_deliveries
			    SET next_attempt_at=clock_timestamp()-interval '1 second'
			    WHERE challenge_id=$1`, issue.ChallengeID); err != nil {
				t.Fatal(err)
			}
			j, ok, err := delivery.Claim(ctx, randomID(t, "lse_"))
			if err != nil || !ok {
				t.Fatalf("attempt %d unavailable: %v", i, err)
			}
			if err := delivery.Finalize(ctx, j.ChallengeID, j.LeaseID, usecase.DeliveryTemporary); err != nil {
				t.Fatal(err)
			}
		}
		var status string
		var attempts int
		if err := admin.QueryRow(ctx, `SELECT status,attempts
			FROM mamony_identity.email_proof_deliveries WHERE challenge_id=$1`,
			issue.ChallengeID).Scan(&status, &attempts); err != nil || status != "DEAD" || attempts != 5 {
			t.Fatalf("dead letter budget failed: %s %d %v", status, attempts, err)
		}
		if _, ok, err := delivery.Claim(ctx, randomID(t, "lse_")); err != nil || ok {
			t.Fatal("dead letter was redelivered")
		}
	})

	t.Run("revoked or consumed proof never enters claim", func(t *testing.T) {
		p := newPending(t)
		old := newIssue(t, p.SubjectID, 0xd5)
		_, err := issuer.Issue(ctx, usecase.EmailProofRequest{
			SubjectID: p.SubjectID, ChallengeID: randomID(t, "vch_"),
			EventID: randomID(t, "evt_"), TokenDigest: bytes.Repeat([]byte{0xd6}, 32),
			EnvelopeKeyID: "test-key", SealedToken: bytes.Repeat([]byte{0xd6}, 71),
		})
		if err != nil {
			t.Fatal(err)
		}
		j, ok, err := delivery.Claim(ctx, randomID(t, "lse_"))
		if err != nil || !ok || j.ChallengeID == old.ChallengeID {
			t.Fatal("revoked proof was leased")
		}
		if err := delivery.Finalize(ctx, j.ChallengeID, j.LeaseID, usecase.DeliveryPermanent); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("parallel claims are exclusive", func(t *testing.T) {
		p := newPending(t)
		issue := newIssue(t, p.SubjectID, 0xd7)
		const n = 8
		var wg sync.WaitGroup
		jobs := make(chan usecase.ProofDeliveryJob, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				job, ok, err := delivery.Claim(ctx, randomID(t, "lse_"))
				if err == nil && ok {
					jobs <- job
				}
			}()
		}
		wg.Wait()
		close(jobs)
		matches := 0
		for job := range jobs {
			if job.ChallengeID == issue.ChallengeID {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("expected one leased challenge, got %d", matches)
		}
	})
}

// Delivery unit tests assert encryption and provider error redaction separately.
// This test intentionally uses synthetic sealed blobs to validate SQL state,
// not as evidence of a production AEAD key or provider integration.
