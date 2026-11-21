//go:build integration

package postgres

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/credential"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailcrypto"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityPasswordAuthentication enforces real password comparison
// through the separate SELECT-only database role and real Argon2id.
// It creates no session or public login endpoint and never calls Mailgun.
func TestIdentityPasswordAuthentication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	adminDSN := fixtureDSN(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := fixtureDSN(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	verifyDSN := fixtureDSN(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	authDSN := fixtureDSN(t, "TEST_IDENTITY_AUTHENTICATOR_URL", "mamony_identity_authenticator_runtime")
	base, _ := url.Parse(adminDSN)
	for _, dsn := range []string{regDSN, verifyDSN, authDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != base.Host || u.Path != base.Path {
			t.Fatal("authentication credentials do not target the same disposable DB")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{
			DSN: dsn, RuntimeRole: role, MaxConns: 3,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	regPool := open(regDSN, "mamony_identity_runtime")
	verifyPool := open(verifyDSN, "mamony_identity_verifier_runtime")
	authPool := open(authDSN, "mamony_identity_authenticator_runtime")
	reg, err := NewStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	proofStore, err := NewProofStore(verifyPool)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := NewAuthenticationStore(authPool)
	if err != nil {
		t.Fatal(err)
	}
	kdf, err := credential.NewPasswordHasher(1)
	if err != nil {
		t.Fatal(err)
	}
	login, err := usecase.NewPasswordAuthentication(reader, kdf)
	if err != nil {
		t.Fatal(err)
	}
	email := syntheticEmail(t)
	password := "a unique synthetic phrase for Argon2id E2E"
	digest, err := kdf.Hash(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	subject := randomID(t, "sub_")
	if err := reg.CreatePendingWithObligation(ctx, usecase.PendingRegistration{
		SubjectID: subject, EventID: randomID(t, "evt_"),
		CanonicalEmail: email, Status: "UNVERIFIED", CreatedAt: time.Now().UTC(),
		PasswordVerifier: digest,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if _, err := admin.Exec(cleanupCtx, `UPDATE mamony_identity.email_proof_deliveries d
			SET status='DEAD' FROM mamony_identity.email_verification_challenges c
			WHERE c.challenge_id=d.challenge_id AND c.subject_id=$1
			AND d.status='PENDING'`, subject); err != nil {
			t.Errorf("failed to quarantine synthetic authentication proof delivery: %v", err)
		}
	})
	denied := func(label, alias, supplied string) {
		t.Helper()
		candidate, err := login.Authenticate(ctx, alias, supplied)
		if candidate != "" || !errors.Is(err, usecase.ErrPasswordAuthenticationDenied) ||
			err.Error() != usecase.ErrPasswordAuthenticationDenied.Error() {
			t.Fatalf("%s: login revealed subject or distinguishable denial: %v", label, err)
		}
	}
	denied("email not verified", email, password)
	denied("unknown alias", syntheticEmail(t), password)
	denied("incorrect password", email, password+"wrong")
	denied("canonical case-insensitive email before verification", strings.ToUpper(email), password)

	sealer, err := mailcrypto.NewKeyring("auth-test", map[string][]byte{
		"auth-test": bytes.Repeat([]byte{0x7a}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	verify, err := usecase.NewEmailVerification(proofStore, sealer)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := verify.Prepare(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	_, secret := prepared.ForTrustedDelivery()
	if err := verify.Confirm(ctx, secret); err != nil {
		t.Fatal(err)
	}
	var status string
	var verified bool
	var events int
	if err := admin.QueryRow(ctx, `SELECT s.status, a.verified_at IS NOT NULL,
		(SELECT count(*) FROM mamony_identity.security_outbox o
		 WHERE o.subject_id=s.subject_id AND o.event_type='CONTACT_VERIFIED')
		FROM mamony_identity.subjects s JOIN mamony_identity.email_aliases a
			ON a.subject_id=s.subject_id
		WHERE s.subject_id=$1`, subject).Scan(&status, &verified, &events); err != nil ||
		status != "ACTIVE" || !verified || events != 1 {
		t.Fatalf("real proof confirmation did not activate before login: %s %v %d %v",
			status, verified, events, err)
	}
	candidate, err := login.Authenticate(ctx, strings.ToUpper(email), password)
	if err != nil || candidate != subject {
		t.Fatal("real Argon2id verified ACTIVE subject was not an internal candidate")
	}
	denied("wrong password after activation", email, password+"wrong")
	denied("unknown after activation", syntheticEmail(t), password)

	// The authenticator role may ONLY read columns, never disable statuses.
	if _, err := admin.Exec(ctx,
		`UPDATE mamony_identity.subjects SET status='FROZEN' WHERE subject_id=$1`, subject); err != nil {
		t.Fatal(err)
	}
	denied("frozen subject with correct password", email, password)

	// Wiring a registration credential to the same adapter must fail closed:
	// forbidden SELECT cannot quietly become an unknown/wrong password.
	wrongRole, err := NewAuthenticationStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = wrongRole.LookupCredential(ctx, email)
	if !errors.Is(err, usecase.ErrPasswordAuthenticationUnavailable) ||
		err.Error() != usecase.ErrPasswordAuthenticationUnavailable.Error() {
		t.Fatal("wrong PostgreSQL role was treated as an unknown account")
	}
}
