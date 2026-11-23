//go:build integration

package identity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/credential"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailcrypto"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityOpaqueSessions joins actual password admission, Argon2id,
// atomic verified-contact proof and the isolated session authority. The
// disposable database proves a status change cannot resurrect old tokens.
func TestIdentityOpaqueSessions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 130*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := guardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	proofDSN := guardedDB(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	authDSN := guardedDB(t, "TEST_IDENTITY_AUTHENTICATOR_URL", "mamony_identity_authenticator_runtime")
	grantDSN := guardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	maintenanceDSN := guardedDB(t, "TEST_IDENTITY_MAINTENANCE_URL", "mamony_identity_maintenance_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, dsn := range []string{regDSN, proofDSN, authDSN, grantDSN, sessionDSN, maintenanceDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("session E2E roles must target the same disposable database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: 3})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	regPool := open(regDSN, "mamony_identity_runtime")
	proofPool := open(proofDSN, "mamony_identity_verifier_runtime")
	authPool := open(authDSN, "mamony_identity_authenticator_runtime")
	grantPool := open(grantDSN, "mamony_identity_auth_grant_runtime")
	sessionPool := open(sessionDSN, "mamony_identity_session_runtime")
	maintPool := open(maintenanceDSN, "mamony_identity_maintenance_runtime")
	reg, err := identitypg.NewStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	proofs, err := identitypg.NewProofStore(proofPool)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := identitypg.NewAuthenticationStore(authPool)
	if err != nil {
		t.Fatal(err)
	}
	sessionStore, err := identitypg.NewOpaqueSessionStore(sessionPool)
	if err != nil {
		t.Fatal(err)
	}
	grantStore, err := identitypg.NewAuthenticationGrantStore(grantPool)
	if err != nil {
		t.Fatal(err)
	}
	grantAuthority, err := usecase.NewAuthenticationGrantAuthority(grantStore)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := usecase.NewOpaqueSessionAuthority(sessionStore)
	if err != nil {
		t.Fatal(err)
	}
	maintenance, err := identitypg.NewRetentionStore(maintPool)
	if err != nil {
		t.Fatal(err)
	}
	kdf, err := credential.NewPasswordHasher(1)
	if err != nil {
		t.Fatal(err)
	}
	passwordAuth, err := usecase.NewPasswordAuthentication(reader, kdf)
	if err != nil {
		t.Fatal(err)
	}
	admissionStore, err := identitypg.NewAdmissionStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := usecase.NewLoginAdmission(admissionStore, bytes.Repeat([]byte{0xdb}, 32))
	if err != nil {
		t.Fatal(err)
	}
	composed, err := usecase.NewPasswordLoginFlow(gate, passwordAuth)
	if err != nil {
		t.Fatal(err)
	}
	sessionLogin, err := usecase.NewSessionLogin(composed, grantAuthority, authority)
	if err != nil {
		t.Fatal(err)
	}

	email := strings.ToLower(randomSyntheticID(t, "session_")) + "@example.test"
	password := "synthetic strong session issue test password"
	subject := randomSyntheticID(t, "sub_")
	verifier, err := kdf.Hash(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.CreatePendingWithObligation(ctx, usecase.PendingRegistration{
		SubjectID: subject, EventID: randomSyntheticID(t, "evt_"),
		CanonicalEmail: email, Status: "UNVERIFIED", CreatedAt: time.Now().UTC(),
		PasswordVerifier: verifier,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanupCtx, `UPDATE mamony_identity.email_proof_deliveries d
			SET status='DEAD' FROM mamony_identity.email_verification_challenges c
			WHERE c.challenge_id=d.challenge_id AND c.subject_id=$1
			AND d.status='PENDING'`, subject); err != nil {
			t.Errorf("synthetic session proof cleanup: %v", err)
		}
	})
	currentEpoch := func() int64 {
		t.Helper()
		var epoch int64
		if err := admin.QueryRow(ctx, "SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", subject).Scan(&epoch); err != nil {
			t.Fatal(err)
		}
		return epoch
	}
	network := netip.MustParseAddr("198.51.100.233")
	// No account with an unverified contact may mint through the composed
	// login, even with the correct password and an available admission slot.
	if session, err := sessionLogin.Authenticate(ctx, email, password, network); session.ForTrustedTransport() != "" || !errors.Is(err, usecase.ErrPasswordAuthenticationDenied) {
		t.Fatal("unverified contact was issued a session")
	}
	if _, err := grantAuthority.Issue(ctx, subject, currentEpoch()); !errors.Is(err, usecase.ErrSessionDenied) {
		t.Fatal("session DB function accepted unverified contact")
	}
	sealer, err := mailcrypto.NewKeyring("session-test", map[string][]byte{
		"session-test": bytes.Repeat([]byte{0xa7}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := usecase.NewEmailVerification(proofs, sealer)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := proof.Prepare(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	_, token := challenge.ForTrustedDelivery()
	if err := proof.Confirm(ctx, token); err != nil {
		t.Fatal(err)
	}

	// Actual admission -> Argon2id -> restricted grant -> session issuance, with a
	// DB-side ACTIVE/verified recheck after the KDF has finished.
	issued, err := sessionLogin.Authenticate(ctx, email, password, network)
	if err != nil {
		t.Fatal(err)
	}
	secret := issued.ForTrustedTransport()
	if len(secret) != 43 {
		t.Fatal("CSPRNG session token must encode 32 bytes")
	}
	realSubject, err := authority.Resolve(ctx, secret)
	if err != nil || realSubject != subject {
		t.Fatal("issued session did not resolve")
	}
	decoded, _ := base64.RawURLEncoding.DecodeString(secret)
	domainDigest := sha256.Sum256(append([]byte("mamony:identity:session-token:v1\x00"), decoded...))
	var rows, matching, events int
	var ageHours float64
	err = admin.QueryRow(ctx, `SELECT count(*)::int,
		count(*) FILTER (WHERE token_digest=$2 AND subject_epoch=s.session_epoch)::int,
		(SELECT count(*) FROM mamony_identity.security_outbox WHERE subject_id=$1 AND event_type='SESSION_CREATED')::int,
		max(extract(epoch FROM (ss.expires_at-ss.issued_at))/3600)::float8
		FROM mamony_identity.opaque_sessions ss
		JOIN mamony_identity.subjects s ON s.subject_id=ss.subject_id
		WHERE ss.subject_id=$1`, subject, domainDigest[:]).
		Scan(&rows, &matching, &events, &ageHours)
	if err != nil || rows != 1 || matching != 1 || events != 1 || ageHours != 12 {
		t.Fatalf("session persisted incorrect hash/epoch/ttl/audit: rows=%d digest=%d audit=%d ttl=%f err=%v",
			rows, matching, events, ageHours, err)
	}
	if bytes.Contains(domainDigest[:], decoded) {
		t.Fatal("digest matched raw bearer bytes")
	}

	// The test admin freezes the account between a correct prior password
	// decision and a fresh issue. The owning SQL function MUST deny after
	// revalidating ACTIVE and verified state in the locked transaction.
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='FROZEN' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	if _, err := grantAuthority.Issue(ctx, subject, currentEpoch()); !errors.Is(err, usecase.ErrSessionDenied) {
		t.Fatal("stale password candidate minted after account freeze")
	}
	if resolved, err := authority.Resolve(ctx, secret); resolved != "" ||
		!errors.Is(err, usecase.ErrSessionInvalid) {
		t.Fatal("frozen account retained session access")
	}
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	if resolved, err := authority.Resolve(ctx, secret); resolved != "" ||
		!errors.Is(err, usecase.ErrSessionInvalid) {
		t.Fatal("session epoch failed to prevent token resurrection after thaw")
	}
	var epoch int64
	if err := admin.QueryRow(ctx, "SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", subject).Scan(&epoch); err != nil ||
		epoch < 4 {
		t.Fatal("status changes did not transactionally advance session epoch")
	}

	// A fresh internal session may issue after ACTIVE is re-established;
	// logout revocation is idempotent and audited exactly once.
	freshGrant, err := grantAuthority.Issue(ctx, subject, currentEpoch())
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := authority.Issue(ctx, freshGrant)
	if err != nil {
		t.Fatal(err)
	}
	freshSecret := fresh.ForTrustedTransport()
	if resolved, err := authority.Resolve(ctx, freshSecret); err != nil || resolved != subject {
		t.Fatal("new epoch session was not valid")
	}
	if err := authority.Revoke(ctx, freshSecret); err != nil {
		t.Fatal(err)
	}
	if err := authority.Revoke(ctx, freshSecret); err != nil {
		t.Fatal(err)
	}
	if err := authority.Revoke(ctx, base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xcc}, 32))); err != nil {
		t.Fatal("unknown syntactically valid token logout must remain idempotent")
	}
	if resolved, err := authority.Resolve(ctx, freshSecret); resolved != "" || !errors.Is(err, usecase.ErrSessionInvalid) {
		t.Fatal("revoked bearer was still accepted")
	}
	var revocations int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.security_outbox
		WHERE subject_id=$1 AND event_type='SESSION_REVOKED'`, subject).Scan(&revocations); err != nil || revocations != 1 {
		t.Fatal("repeated logout created duplicate revocation events")
	}

	// Expiry is authoritative DB clock, never the caller's machine clock.
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.opaque_sessions SET
		issued_at=clock_timestamp()-interval '40 hours',
		last_seen_at=clock_timestamp()-interval '40 hours',
		expires_at=clock_timestamp()-interval '27 hours'
		WHERE subject_id=$1`, subject); err != nil {
		t.Fatal(err)
	}
	if resolved, err := authority.Resolve(ctx, secret); resolved != "" || !errors.Is(err, usecase.ErrSessionInvalid) {
		t.Fatal("expired opaque token remained valid")
	}
	if resolved, err := authority.Resolve(ctx, freshSecret); resolved != "" || !errors.Is(err, usecase.ErrSessionInvalid) {
		t.Fatal("expired revoked token remained valid")
	}
	env, counter, sessions, err := maintenance.PurgeOneBatchWithSessions(ctx)
	if err != nil || sessions != 2 || env < 0 || counter < 0 {
		t.Fatalf("session retention must purge two old digests: sessions=%d err=%v", sessions, err)
	}
	_, _, sessions, err = maintenance.PurgeOneBatchWithSessions(ctx)
	if err != nil || sessions != 0 {
		t.Fatal("bounded session purge was not idempotent")
	}
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM mamony_identity.security_outbox WHERE subject_id=$1 AND event_type='SESSION_CREATED'",
		subject).Scan(&events); err != nil || events != 2 {
		t.Fatal("retention must preserve both identifier-only creation audit events")
	}
}
