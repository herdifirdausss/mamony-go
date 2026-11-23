//go:build integration

package postgres

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityResendAdmission is a mandatory real-PostgreSQL test. It proves
// budgets are consulted while holding the same subject/alias row locks used to
// serialize challenge rotation. All times are the authoritative DB clock.
func TestIdentityResendAdmission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	adminDSN := fixtureDSN(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := fixtureDSN(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	verifyDSN := fixtureDSN(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	root, _ := url.Parse(adminDSN)
	for _, dsn := range []string{regDSN, verifyDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != root.Host || u.Path != root.Path {
			t.Fatal("Identity fixture connections do not point to one disposable database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	regPool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: regDSN, RuntimeRole: "mamony_identity_runtime", MaxConns: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer regPool.Close()
	verifyPool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: verifyDSN, RuntimeRole: "mamony_identity_verifier_runtime", MaxConns: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer verifyPool.Close()
	reg, err := NewStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := NewProofStore(verifyPool)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := usecase.NewEmailVerification(proof, fakeProofSealer{})
	if err != nil {
		t.Fatal(err)
	}
	pending := func(t *testing.T) usecase.PendingRegistration {
		t.Helper()
		p := syntheticPending(t, syntheticEmail(t))
		if err := reg.CreatePendingWithObligation(ctx, p); err != nil {
			t.Fatal(err)
		}
		// The DB integration suites share one disposable database. Never
		// leave a synthetic pending envelope for a later Mailgun mock suite
		// to claim; production queue semantics are unchanged.
		t.Cleanup(func() {
			cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			_, err := admin.Exec(cleanupCtx, `UPDATE mamony_identity.email_proof_deliveries d
				SET status='DEAD'
				FROM mamony_identity.email_verification_challenges c
				WHERE c.challenge_id=d.challenge_id AND c.subject_id=$1
				  AND d.status='PENDING'`, p.SubjectID)
			if err != nil {
				t.Errorf("cleanup of isolated synthetic challenge fixture failed: %v", err)
			}
		})
		return p
	}
	markOlder := func(t *testing.T, subject string, age string) {
		t.Helper()
		var count int64
		// Static caller-owned age strings are used only for this integration
		// fixture; no production SQL accepts arbitrary interval input.
		switch age {
		case "65s":
			tag, err := admin.Exec(ctx, `UPDATE mamony_identity.email_verification_challenges
				SET requested_at=clock_timestamp()-interval '65 seconds'
				WHERE subject_id=$1 AND requested_at>clock_timestamp()-interval '60 seconds'`, subject)
			if err != nil {
				t.Fatal(err)
			}
			count = tag.RowsAffected()
		case "2h":
			tag, err := admin.Exec(ctx, `UPDATE mamony_identity.email_verification_challenges
				SET requested_at=clock_timestamp()-interval '2 hours'
				WHERE subject_id=$1`, subject)
			if err != nil {
				t.Fatal(err)
			}
			count = tag.RowsAffected()
		case "25h":
			tag, err := admin.Exec(ctx, `UPDATE mamony_identity.email_verification_challenges
				SET requested_at=clock_timestamp()-interval '25 hours'
				WHERE subject_id=$1`, subject)
			if err != nil {
				t.Fatal(err)
			}
			count = tag.RowsAffected()
		default:
			t.Fatal("unknown synthetic time shift")
		}
		if count < 1 {
			t.Fatal("fixture failed to move issuance clock")
		}
	}
	stats := func(t *testing.T, subject string) (challenges, events, envelopes, active int) {
		t.Helper()
		err := admin.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM mamony_identity.email_verification_challenges
			 WHERE subject_id=$1),
			(SELECT count(*) FROM mamony_identity.security_outbox
			 WHERE subject_id=$1 AND event_type='EMAIL_PROOF_ISSUED'),
			(SELECT count(*) FROM mamony_identity.email_proof_deliveries d
			 JOIN mamony_identity.email_verification_challenges c USING(challenge_id)
			 WHERE c.subject_id=$1),
			(SELECT count(*) FROM mamony_identity.email_verification_challenges
			 WHERE subject_id=$1 AND consumed_at IS NULL AND revoked_at IS NULL)`, subject).
			Scan(&challenges, &events, &envelopes, &active)
		if err != nil {
			t.Fatal(err)
		}
		return
	}

	t.Run("cooldown denial retains old valid token and creates no queue or audit event", func(t *testing.T) {
		p := pending(t)
		first, err := svc.Prepare(ctx, p.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		_, oldToken := first.ForTrustedDelivery()
		if err := svc.Resend(ctx, p.SubjectID); !errors.Is(err, usecase.ErrEmailProofThrottled) {
			t.Fatalf("immediate resend must be rejected without rotating old token: %v", err)
		}
		c, e, d, live := stats(t, p.SubjectID)
		if c != 1 || e != 1 || d != 1 || live != 1 {
			t.Fatalf("denied reissue mutated persisted state: challenges=%d events=%d outbox=%d live=%d", c, e, d, live)
		}
		if err := svc.Confirm(ctx, oldToken); err != nil {
			t.Fatalf("denied resend invalidated previously issued token: %v", err)
		}
		if err := svc.Resend(ctx, p.SubjectID); !errors.Is(err, usecase.ErrEmailProofUnavailable) {
			t.Fatalf("already verified subject cannot resend: %v", err)
		}
	})

	t.Run("rolling hour and day quota count revoked proofs and recover with DB-time window", func(t *testing.T) {
		p := pending(t)
		for i := 0; i < 3; i++ {
			if err := svc.Resend(ctx, p.SubjectID); err != nil {
				t.Fatalf("issue %d should be allowed: %v", i+1, err)
			}
			markOlder(t, p.SubjectID, "65s")
		}
		if err := svc.Resend(ctx, p.SubjectID); !errors.Is(err, usecase.ErrEmailProofThrottled) {
			t.Fatalf("4th issuance within 1 hour should be denied: %v", err)
		}
		if c, e, d, live := stats(t, p.SubjectID); c != 3 || e != 3 || d != 3 || live != 1 {
			t.Fatalf("hour rejection changed state: %d/%d/%d/%d", c, e, d, live)
		}
		markOlder(t, p.SubjectID, "2h")
		for i := 0; i < 2; i++ {
			if err := svc.Resend(ctx, p.SubjectID); err != nil {
				t.Fatalf("hour reset should permit remaining daily issuance: %v", err)
			}
			markOlder(t, p.SubjectID, "65s")
		}
		if err := svc.Resend(ctx, p.SubjectID); !errors.Is(err, usecase.ErrEmailProofThrottled) {
			t.Fatalf("6th issuance within 24 hours should be denied: %v", err)
		}
		if c, e, d, live := stats(t, p.SubjectID); c != 5 || e != 5 || d != 5 || live != 1 {
			t.Fatalf("day rejection changed state: %d/%d/%d/%d", c, e, d, live)
		}
		markOlder(t, p.SubjectID, "25h")
		if err := svc.Resend(ctx, p.SubjectID); err != nil {
			t.Fatalf("expired sliding quota did not reopen: %v", err)
		}
		if c, e, d, live := stats(t, p.SubjectID); c != 6 || e != 6 || d != 6 || live != 1 {
			t.Fatalf("new window did not produce exactly one new job: %d/%d/%d/%d", c, e, d, live)
		}
	})

	t.Run("ten concurrent resend requests get one winner", func(t *testing.T) {
		p := pending(t)
		const contenders = 10
		var wg sync.WaitGroup
		start := make(chan struct{})
		results := make(chan error, contenders)
		for i := 0; i < contenders; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results <- svc.Resend(ctx, p.SubjectID)
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		accepted, throttled := 0, 0
		for err := range results {
			switch {
			case err == nil:
				accepted++
			case errors.Is(err, usecase.ErrEmailProofThrottled):
				throttled++
			default:
				t.Fatalf("concurrent issue failure was not bounded: %v", err)
			}
		}
		if accepted != 1 || throttled != contenders-1 {
			t.Fatalf("subject row lock failed admission: admitted=%d limited=%d", accepted, throttled)
		}
		if c, e, d, live := stats(t, p.SubjectID); c != 1 || e != 1 || d != 1 || live != 1 {
			t.Fatalf("race produced duplicate obligation: %d/%d/%d/%d", c, e, d, live)
		}
	})
}
