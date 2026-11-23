//go:build integration

package postgres

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

func TestIdentityEmailVerification(t *testing.T) {
	adminDSN := fixtureDSN(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	registrationDSN := fixtureDSN(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	verifierDSN := fixtureDSN(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, raw := range []string{registrationDSN, verifierDSN} {
		u, _ := url.Parse(raw)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("every Identity test role must target the same disposable DB")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	regPool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: registrationDSN, RuntimeRole: "mamony_identity_runtime", MaxConns: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer regPool.Close()
	verifierPool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: verifierDSN, RuntimeRole: "mamony_identity_verifier_runtime", MaxConns: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer verifierPool.Close()
	reg, err := NewStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	proofs, err := NewProofStore(verifierPool)
	if err != nil {
		t.Fatal(err)
	}
	service, err := usecase.NewEmailVerification(proofs, fakeProofSealer{})
	if err != nil {
		t.Fatal(err)
	}
	newPending := func(t *testing.T) usecase.PendingRegistration {
		t.Helper()
		pending := syntheticPending(t, syntheticEmail(t))
		if err := reg.CreatePendingWithObligation(ctx, pending); err != nil {
			t.Fatalf("register pending before proof: %v", err)
		}
		return pending
	}
	verifyPending := func(t *testing.T, subject string, want string) {
		t.Helper()
		var status string
		var verified bool
		err := admin.QueryRow(ctx, `SELECT s.status, a.verified_at IS NOT NULL
			FROM mamony_identity.subjects s JOIN mamony_identity.email_aliases a
			ON a.subject_id = s.subject_id WHERE s.subject_id=$1`, subject).Scan(&status, &verified)
		if err != nil || status != want || verified != (want == "ACTIVE") {
			t.Fatalf("subject status or contact evidence differs: status=%q verified=%v err=%v", status, verified, err)
		}
	}

	t.Run("issuer binds purpose subject email version and stores only digest", func(t *testing.T) {
		pending := newPending(t)
		out, err := service.Prepare(ctx, pending.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		email, secret := out.ForTrustedDelivery()
		if email != pending.CanonicalEmail || len(secret) != 43 {
			t.Fatal("prepared proof target or random secret invalid")
		}
		if fmt.Sprint(out) == secret || fmt.Sprintf("%#v", out) == secret {
			t.Fatal("proof leaked through formatted output")
		}
		if _, err := out.MarshalJSON(); err == nil {
			t.Fatal("proof must not marshal into a public response")
		}
		var count int
		err = admin.QueryRow(ctx, `SELECT count(*)
			FROM mamony_identity.email_verification_challenges
			WHERE subject_id=$1 AND canonical_email=$2 AND contact_version=1
			  AND purpose='EMAIL_VERIFY' AND length(token_digest)=32
			  AND consumed_at IS NULL AND revoked_at IS NULL`,
			pending.SubjectID, email).Scan(&count)
		if err != nil || count != 1 {
			t.Fatalf("proof digest not persistently scoped: %d %v", count, err)
		}
		verifyPending(t, pending.SubjectID, "UNVERIFIED")
		if err := service.Confirm(ctx, secret+"x"); !errors.Is(err, usecase.ErrEmailProofInvalid) {
			t.Fatalf("malformed bearer accepted: %v", err)
		}
		if err := service.Confirm(ctx, secret); err != nil {
			t.Fatalf("valid email proof rejected: %v", err)
		}
		verifyPending(t, pending.SubjectID, "ACTIVE")
		if err := service.Confirm(ctx, secret); !errors.Is(err, usecase.ErrEmailProofInvalid) {
			t.Fatalf("single-use proof replay accepted: %v", err)
		}
		var events int
		err = admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.security_outbox
			WHERE subject_id=$1 AND event_type='CONTACT_VERIFIED'`, pending.SubjectID).Scan(&events)
		if err != nil || events != 1 {
			t.Fatalf("contact activation must commit one durable event: %d %v", events, err)
		}
		if _, err := service.Prepare(ctx, pending.SubjectID); !errors.Is(err, usecase.ErrEmailProofUnavailable) {
			t.Fatalf("already active user reissued enrollment proof: %v", err)
		}
	})

	t.Run("reissue atomically revokes previous token", func(t *testing.T) {
		pending := newPending(t)
		first, err := service.Prepare(ctx, pending.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		// Existing reissue semantics are tested outside the new 60-second
		// budget window, without sleeping or bypassing the production guard.
		if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_verification_challenges
			SET requested_at=clock_timestamp()-interval '65 seconds'
			WHERE subject_id=$1`, pending.SubjectID); err != nil {
			t.Fatal(err)
		}
		second, err := service.Prepare(ctx, pending.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		_, token1 := first.ForTrustedDelivery()
		_, token2 := second.ForTrustedDelivery()
		if token1 == token2 {
			t.Fatal("new proof reused token")
		}
		if err := service.Confirm(ctx, token1); err == nil {
			t.Fatal("old proof accepted after rotate")
		}
		verifyPending(t, pending.SubjectID, "UNVERIFIED")
		if err := service.Confirm(ctx, token2); err != nil {
			t.Fatal(err)
		}
		verifyPending(t, pending.SubjectID, "ACTIVE")
	})

	t.Run("expired proof refuses activation", func(t *testing.T) {
		pending := newPending(t)
		out, err := service.Prepare(ctx, pending.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		_, secret := out.ForTrustedDelivery()
		_, err = admin.Exec(ctx, `UPDATE mamony_identity.email_verification_challenges
			SET requested_at = clock_timestamp()-interval '1 hour',
			    expires_at = clock_timestamp()-interval '1 minute'
			WHERE subject_id=$1`, pending.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.Confirm(ctx, secret); err == nil {
			t.Fatal("expired token accepted")
		}
		verifyPending(t, pending.SubjectID, "UNVERIFIED")
	})

	t.Run("current contact binding prevents wrong-account activation", func(t *testing.T) {
		victim := newPending(t)
		other := newPending(t)
		out, err := service.Prepare(ctx, victim.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		_, secret := out.ForTrustedDelivery()
		// Admin-only tampering simulates a stale/mismatched contact snapshot,
		// not a claim that Profile A resists arbitrary DB administrator writes.
		_, err = admin.Exec(ctx, `UPDATE mamony_identity.email_verification_challenges
			SET canonical_email=$1 WHERE subject_id=$2`,
			other.CanonicalEmail, victim.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.Confirm(ctx, secret); err == nil {
			t.Fatal("mismatched current alias accepted")
		}
		verifyPending(t, victim.SubjectID, "UNVERIFIED")
		verifyPending(t, other.SubjectID, "UNVERIFIED")
	})

	t.Run("outbox failure rolls back consume and activation", func(t *testing.T) {
		pending := newPending(t)
		req := usecase.EmailProofRequest{
			SubjectID:     pending.SubjectID,
			ChallengeID:   randomID(t, "vch_"),
			EventID:       randomID(t, "evt_"),
			TokenDigest:   bytes.Repeat([]byte{0x63}, 32),
			EnvelopeKeyID: "testkey",
			SealedToken:   bytes.Repeat([]byte{0xA5}, 71),
		}
		if _, err := proofs.Issue(ctx, req); err != nil {
			t.Fatal(err)
		}
		// Reusing the issuance event ID intentionally fails the final outbox
		// INSERT, after token consumption and status change were attempted.
		if err := proofs.Consume(ctx, req.TokenDigest, req.EventID); !errors.Is(err, usecase.ErrEmailProofUnavailable) {
			t.Fatalf("failed outbox must roll back activation and report sanitized authority outage: %v", err)
		}
		verifyPending(t, pending.SubjectID, "UNVERIFIED")
		var consumed bool
		if err := admin.QueryRow(ctx, `SELECT consumed_at IS NOT NULL
			FROM mamony_identity.email_verification_challenges
			WHERE challenge_id=$1`, req.ChallengeID).Scan(&consumed); err != nil || consumed {
			t.Fatalf("failed event left consumed token: %v %v", consumed, err)
		}
		if err := proofs.Consume(ctx, req.TokenDigest, randomID(t, "evt_")); err != nil {
			t.Fatal(err)
		}
		verifyPending(t, pending.SubjectID, "ACTIVE")
	})

	t.Run("failed reissue event cannot revoke existing proof", func(t *testing.T) {
		pending := newPending(t)
		first := usecase.EmailProofRequest{
			SubjectID: pending.SubjectID, ChallengeID: randomID(t, "vch_"),
			EventID: randomID(t, "evt_"), TokenDigest: bytes.Repeat([]byte{0x71}, 32),
			EnvelopeKeyID: "testkey", SealedToken: bytes.Repeat([]byte{0xA5}, 71),
		}
		if _, err := proofs.Issue(ctx, first); err != nil {
			t.Fatal(err)
		}
		// Existing reissue semantics are tested outside the new 60-second
		// budget window, without sleeping or bypassing the production guard.
		if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_verification_challenges
			SET requested_at=clock_timestamp()-interval '65 seconds'
			WHERE subject_id=$1`, pending.SubjectID); err != nil {
			t.Fatal(err)
		}
		conflict := usecase.EmailProofRequest{
			SubjectID: pending.SubjectID, ChallengeID: randomID(t, "vch_"),
			EventID: first.EventID, TokenDigest: bytes.Repeat([]byte{0x72}, 32),
			EnvelopeKeyID: "testkey", SealedToken: bytes.Repeat([]byte{0xA5}, 71),
		}
		if _, err := proofs.Issue(ctx, conflict); err == nil {
			t.Fatal("duplicate issuance outbox event must fail")
		}
		if err := proofs.Consume(ctx, first.TokenDigest, randomID(t, "evt_")); err != nil {
			t.Fatalf("failed reissue must not revoke original proof: %v", err)
		}
		verifyPending(t, pending.SubjectID, "ACTIVE")
	})

	t.Run("concurrent consume permits exactly one winner", func(t *testing.T) {
		pending := newPending(t)
		out, err := service.Prepare(ctx, pending.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		_, secret := out.ForTrustedDelivery()
		const workers = 8
		errs := make(chan error, workers)
		for i := 0; i < workers; i++ {
			go func() { errs <- service.Confirm(ctx, secret) }()
		}
		success := 0
		for i := 0; i < workers; i++ {
			switch err := <-errs; {
			case err == nil:
				success++
			case errors.Is(err, usecase.ErrEmailProofInvalid):
			default:
				t.Errorf("unexpected race outcome: %v", err)
			}
		}
		if success != 1 {
			t.Fatalf("exactly one proof must activate account: wins=%d", success)
		}
		verifyPending(t, pending.SubjectID, "ACTIVE")
	})
}

// fakeProofSealer is test-only. Production requires an authenticated
// encryption envelope and externally provisioned key from the mailcrypto adapter.
type fakeProofSealer struct{}

func (fakeProofSealer) Seal(_, _, _ string) (string, []byte, error) {
	return "testkey", bytes.Repeat([]byte{0xA5}, 71), nil
}
