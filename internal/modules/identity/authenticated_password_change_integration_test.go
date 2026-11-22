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

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/credential"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityZAuthenticatedPasswordChange exercises the real, unmounted Go
// usecase: durable pre-KDF admission, Argon2id, two separate SQL grant
// purposes, valid session, atomic verifier change/epoch/audit and re-login.
func TestIdentityZAuthenticatedPasswordChange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 140*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := guardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	authDSN := guardedDB(t, "TEST_IDENTITY_AUTHENTICATOR_URL", "mamony_identity_authenticator_runtime")
	grantDSN := guardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	changeDSN := guardedDB(t, "TEST_IDENTITY_PASSWORD_CHANGE_URL", "mamony_identity_password_change_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, raw := range []string{regDSN, authDSN, grantDSN, sessionDSN, changeDSN} {
		u, _ := url.Parse(raw)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("password change roles must share one isolated loopback database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	grantConn, err := pgx.Connect(ctx, grantDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = grantConn.Close(context.Background()) })
	changeConn, err := pgx.Connect(ctx, changeDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = changeConn.Close(context.Background()) })
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
	authPool := open(authDSN, "mamony_identity_authenticator_runtime")
	grantPool := open(grantDSN, "mamony_identity_auth_grant_runtime")
	sessionPool := open(sessionDSN, "mamony_identity_session_runtime")
	changePool := open(changeDSN, "mamony_identity_password_change_runtime")
	kdf, err := credential.NewPasswordHasher(1)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := identitypg.NewAuthenticationStore(authPool)
	if err != nil {
		t.Fatal(err)
	}
	password, err := usecase.NewPasswordAuthentication(reader, kdf)
	if err != nil {
		t.Fatal(err)
	}
	admissionStore, err := identitypg.NewAdmissionStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := usecase.NewLoginAdmission(admissionStore, bytes.Repeat([]byte{0xe3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	loginFlow, err := usecase.NewPasswordLoginFlow(gate, password)
	if err != nil {
		t.Fatal(err)
	}
	grantStore, err := identitypg.NewAuthenticationGrantStore(grantPool)
	if err != nil {
		t.Fatal(err)
	}
	grants, err := usecase.NewAuthenticationGrantAuthority(grantStore)
	if err != nil {
		t.Fatal(err)
	}
	sessionStore, err := identitypg.NewOpaqueSessionStore(sessionPool)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := usecase.NewOpaqueSessionAuthority(sessionStore)
	if err != nil {
		t.Fatal(err)
	}
	sessionLogin, err := usecase.NewSessionLogin(loginFlow, grants, sessions)
	if err != nil {
		t.Fatal(err)
	}
	rotationStore, err := identitypg.NewPasswordChangeStore(changePool)
	if err != nil {
		t.Fatal(err)
	}
	changer, err := usecase.NewAuthenticatedPasswordChange(sessions, loginFlow, grants,
		kdf, rotationStore, domain.PasswordPolicy{
			IsBlocked: func(normalized, _ string) bool {
				return normalized == "an obviously compromised password"
			},
		})
	if err != nil {
		t.Fatal(err)
	}
	// Only this disposable fixture's admin creates a verified synthetic
	// account. No production admin path or bypass is introduced.
	subject := randomSyntheticID(t, "sub_")
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if _, err := admin.Exec(cleanupCtx,
			"UPDATE mamony_identity.password_change_notices SET status='DEAD',lease_id=NULL,leased_until=NULL WHERE subject_id=$1 AND status='PENDING'",
			subject); err != nil {
			t.Errorf("failed to quarantine synthetic password notice: %v", err)
		}
	})
	email := strings.ToLower(randomSyntheticID(t, "password_")) + "@example.test"
	oldPassword := "synthetic old password for rotated credential"
	nextPassword := "synthetic brand new password for rotated credential"
	original, err := kdf.Hash(ctx, oldPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.subjects (subject_id,created_at) VALUES ($1,clock_timestamp())",
		subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.email_aliases
		(canonical_email,subject_id,verified_at,created_at)
		VALUES ($1,$2,clock_timestamp(),clock_timestamp())`, email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_verifiers
		(subject_id,kdf_version,memory_kib,iterations,parallelism,salt,digest,created_at)
		VALUES ($1,1,65536,3,1,$2,$3,clock_timestamp())`,
		subject, original.Salt, original.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1",
		subject); err != nil {
		t.Fatal(err)
	}
	epoch := func() int64 {
		t.Helper()
		var v int64
		if err := admin.QueryRow(ctx,
			"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
			subject).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	eventCount := func() int {
		t.Helper()
		var n int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.security_outbox
			WHERE subject_id=$1 AND event_type='PASSWORD_CHANGED'`,
			subject).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	peer := netip.MustParseAddr("198.51.100.192")
	issued, err := sessionLogin.Authenticate(ctx, email, oldPassword, peer)
	if err != nil {
		t.Fatal(err)
	}
	bearer := issued.ForTrustedTransport()
	if who, err := sessions.Resolve(ctx, bearer); err != nil || who != subject {
		t.Fatal("old password and valid session could not authenticate")
	}
	before := epoch()
	// A purpose-bound change proof MUST be unusable as login proof.
	changeProof, err := grants.IssueChange(ctx, subject, before)
	if err != nil {
		t.Fatal(err)
	}
	if newBearer, err := sessions.Issue(ctx, changeProof); newBearer.ForTrustedTransport() != "" ||
		!errors.Is(err, usecase.ErrSessionDenied) {
		t.Fatal("PASSWORD_CHANGE grant was accepted as a login grant")
	}
	// The wrong password can never create a rotation grant or change DB
	// verifier, while still charging the durable pre-KDF admission budget.
	if err := changer.Change(ctx, bearer, email, oldPassword+"wrong", nextPassword, peer); !errors.Is(err, usecase.ErrPasswordChangeDenied) {
		t.Fatalf("wrong old password accepted for change: %v", err)
	}
	if epoch() != before || eventCount() != 0 {
		t.Fatal("denied password change mutated credential state")
	}
	// A valid session without a change-purpose grant cannot invoke Rotate;
	// similarly, a LOGIN-purpose grant cannot authorize a PASSWORD_CHANGE.
	raw, _ := base64.RawURLEncoding.DecodeString(bearer)
	sessionDigest := sha256.Sum256(append([]byte("mamony:identity:session-token:v1\x00"), raw...))
	normalGrant := sha256.Sum256([]byte("synthetic:ordinary-login-grant:" + subject))
	var stageAllowed bool
	if err := grantConn.QueryRow(ctx,
		"SELECT mamony_identity.stage_password_auth_grant($1,$2,$3)",
		subject, before, normalGrant[:]).Scan(&stageAllowed); err != nil || !stageAllowed {
		t.Fatalf("could not stage ordinary login-purpose grant: %v %v", stageAllowed, err)
	}
	newVerifier, err := kdf.Hash(ctx, nextPassword)
	if err != nil {
		t.Fatal(err)
	}
	var rotateAllowed bool
	if err := changeConn.QueryRow(ctx,
		"SELECT mamony_identity.rotate_password_verifier($1,$2,$3,$4,$5)",
		sessionDigest[:], normalGrant[:], newVerifier.Salt, newVerifier.Digest,
		randomSyntheticID(t, "evt_")).Scan(&rotateAllowed); err != nil || rotateAllowed {
		t.Fatalf("ordinary login grant rotated password: allowed=%t err=%v", rotateAllowed, err)
	}
	// A failed audit insert must roll back grant consumption AND verifier
	// mutation, so an unrelated duplicate event ID cannot revoke sessions.
	changeDigest := sha256.Sum256([]byte("synthetic:rollback-password-change:" + subject))
	if err := grantConn.QueryRow(ctx,
		"SELECT mamony_identity.stage_password_change_grant($1,$2,$3)",
		subject, before, changeDigest[:]).Scan(&stageAllowed); err != nil || !stageAllowed {
		t.Fatalf("cannot stage rollback test proof: %v %v", stageAllowed, err)
	}
	collisionID := randomSyntheticID(t, "evt_")
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.security_outbox
		(event_id,subject_id,event_type,occurred_at)
		VALUES ($1,$2,'SESSION_CREATED',clock_timestamp())`, collisionID, subject); err != nil {
		t.Fatal(err)
	}
	if err := changeConn.QueryRow(ctx,
		"SELECT mamony_identity.rotate_password_verifier($1,$2,$3,$4,$5)",
		sessionDigest[:], changeDigest[:], newVerifier.Salt, newVerifier.Digest,
		collisionID).Scan(&rotateAllowed); err == nil {
		t.Fatal("outbox identifier collision was not atomic failure")
	}
	if epoch() != before || eventCount() != 0 {
		t.Fatal("failed outbox insert committed verifier or epoch changes")
	}
	if who, err := sessions.Resolve(ctx, bearer); err != nil || who != subject {
		t.Fatal("rolled-back rotation invalidated prior session")
	}
	var consumed bool
	if err := admin.QueryRow(ctx, `SELECT consumed_at IS NOT NULL
		FROM mamony_identity.password_login_grants WHERE grant_digest=$1`,
		changeDigest[:]).Scan(&consumed); err != nil || consumed {
		t.Fatalf("failed password rotation consumed its grant: %v %v", consumed, err)
	}
	// Exactly one authorized internal change invalidates all old sessions
	// without issuing a replacement session. The caller must log in again.
	if err := changer.Change(ctx, bearer, email, oldPassword, nextPassword, peer); err != nil {
		t.Fatalf("authorized password change did not commit: %v", err)
	}
	if epoch() != before+1 || eventCount() != 1 {
		t.Fatal("password change did not atomically advance epoch and record audit")
	}
	var pending int
	if err := admin.QueryRow(ctx, `SELECT count(*)
		FROM mamony_identity.password_change_notices n
		JOIN mamony_identity.security_outbox o USING(event_id)
		WHERE n.subject_id=$1 AND n.recipient=$2
			AND o.event_type='PASSWORD_CHANGED' AND n.status='PENDING'`,
		subject, email).Scan(&pending); err != nil || pending != 1 {
		t.Fatal("successful password change did not transactionally enqueue one security notice")
	}
	if who, err := sessions.Resolve(ctx, bearer); who != "" ||
		!errors.Is(err, usecase.ErrSessionInvalid) {
		t.Fatal("old session remains usable after password change")
	}
	var different bool
	if err := admin.QueryRow(ctx, `SELECT salt<>$2 AND digest<>$3
		FROM mamony_identity.password_verifiers WHERE subject_id=$1`,
		subject, original.Salt, original.Digest).Scan(&different); err != nil || !different {
		t.Fatal("password verifier was not rotated to new salted Argon2id")
	}
	// An old password cannot log in after the change. A new credential
	// consumes the next authorized pre-KDF admission slot and gets new epoch.
	if secret, err := sessionLogin.Authenticate(ctx, email, oldPassword, peer); secret.ForTrustedTransport() != "" || !errors.Is(err, usecase.ErrPasswordAuthenticationDenied) {
		t.Fatal("old password still authenticates after rotation")
	}
	relogin, err := sessionLogin.Authenticate(ctx, email, nextPassword, peer)
	if err != nil {
		t.Fatalf("new password cannot authenticate: %v", err)
	}
	if who, err := sessions.Resolve(ctx, relogin.ForTrustedTransport()); err != nil || who != subject {
		t.Fatal("new login did not receive a usable fresh-epoch session")
	}
	if eventCount() != 1 {
		t.Fatal("relogin duplicated password-change audit")
	}
}
