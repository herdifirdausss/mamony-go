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
	"sync"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/credential"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityZRecoveryCodeEnrollmentRotation uses current-password Argon2id,
// durable login admission and the existing short-lived session/reauth grants,
// with disposable PostgreSQL and no real independent bootstrap issuer.
func TestIdentityZRecoveryCodeEnrollmentRotation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 160*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := guardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	authDSN := guardedDB(t, "TEST_IDENTITY_AUTHENTICATOR_URL", "mamony_identity_authenticator_runtime")
	grantDSN := guardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	managerDSN := guardedDB(t, "TEST_IDENTITY_RECOVERY_CODE_MANAGER_URL", "mamony_identity_recovery_code_manager_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, raw := range []string{regDSN, authDSN, grantDSN, sessionDSN, managerDSN} {
		u, _ := url.Parse(raw)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("recovery code test requires same disposable PostgreSQL database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: 4})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	kdf, err := credential.NewPasswordHasher(2)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := identitypg.NewAuthenticationStore(open(authDSN, "mamony_identity_authenticator_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	password, err := usecase.NewPasswordAuthentication(reader, kdf)
	if err != nil {
		t.Fatal(err)
	}
	admissionStore, err := identitypg.NewAdmissionStore(open(regDSN, "mamony_identity_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	gate, err := usecase.NewLoginAdmission(admissionStore, bytes.Repeat([]byte{0xe8}, 32))
	if err != nil {
		t.Fatal(err)
	}
	loginFlow, err := usecase.NewPasswordLoginFlow(gate, password)
	if err != nil {
		t.Fatal(err)
	}
	grantPool := open(grantDSN, "mamony_identity_auth_grant_runtime")
	loginGrantStore, err := identitypg.NewAuthenticationGrantStore(grantPool)
	if err != nil {
		t.Fatal(err)
	}
	loginGrants, err := usecase.NewAuthenticationGrantAuthority(loginGrantStore)
	if err != nil {
		t.Fatal(err)
	}
	recoveryGrantStore, err := identitypg.NewRecoveryCodeGrantStore(grantPool)
	if err != nil {
		t.Fatal(err)
	}
	codeGrants, err := usecase.NewRecoveryCodeGrantAuthority(recoveryGrantStore)
	if err != nil {
		t.Fatal(err)
	}
	sessionStore, err := identitypg.NewOpaqueSessionStore(open(sessionDSN, "mamony_identity_session_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := usecase.NewOpaqueSessionAuthority(sessionStore)
	if err != nil {
		t.Fatal(err)
	}
	sessionLogin, err := usecase.NewSessionLogin(loginFlow, loginGrants, sessions)
	if err != nil {
		t.Fatal(err)
	}
	managerStore, err := identitypg.NewRecoveryCodeManagerStore(open(managerDSN, "mamony_identity_recovery_code_manager_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := usecase.NewAuthenticatedRecoveryCodeManager(sessions, loginFlow, codeGrants, managerStore)
	if err != nil {
		t.Fatal(err)
	}
	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "code_")) + "@example.test"
	currentPassword := "synthetic credential for recovery code enrollment"
	verifier, err := kdf.Hash(ctx, currentPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES($1,clock_timestamp())", subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.email_aliases
		(canonical_email,subject_id,verified_at,created_at)
		VALUES($1,$2,clock_timestamp(),clock_timestamp())`, email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_verifiers
		(subject_id,kdf_version,memory_kib,iterations,parallelism,salt,digest,created_at)
		VALUES($1,1,65536,3,1,$2,$3,clock_timestamp())`,
		subject, verifier.Salt, verifier.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	var epoch int64
	if err := admin.QueryRow(ctx, "SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", subject).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		conn, err := pgx.Connect(cleanupCtx, adminDSN)
		if err != nil {
			t.Errorf("notice cleanup connection: %v", err)
			return
		}
		defer func() { _ = conn.Close(context.Background()) }()
		if _, err := conn.Exec(cleanupCtx, `UPDATE mamony_identity.password_recovery_code_notices
			SET status='DEAD' WHERE status='PENDING' AND subject_id=$1`, subject); err != nil {
			t.Errorf("unlaunched recovery code notice cleanup: %v", err)
		}
		if _, err := conn.Exec(cleanupCtx, `UPDATE mamony_identity.password_recovery_deliveries d
			SET status='DEAD',lease_id=NULL,leased_until=NULL
			WHERE status='PENDING' AND EXISTS (
				SELECT 1 FROM mamony_identity.password_recovery_challenges c
				WHERE c.challenge_id=d.challenge_id AND c.subject_id=$1)`, subject); err != nil {
			t.Errorf("recovery challenge delivery cleanup: %v", err)
		}
	})
	peer := netip.MustParseAddr("198.51.100.224")
	session, err := sessionLogin.Authenticate(ctx, email, currentPassword, peer)
	if err != nil {
		t.Fatal(err)
	}
	bearer := session.ForTrustedTransport()
	if who, err := sessions.Resolve(ctx, bearer); err != nil || who != subject {
		t.Fatal("valid session did not match authenticated subject")
	}
	// The account has no code and no independently authorized bootstrap
	// approval: session + correct password must NOT be sufficient.
	var raw [32]byte
	for i := range raw {
		raw[i] = byte(i + 31)
	}
	approval := base64.RawURLEncoding.EncodeToString(raw[:])
	batch, err := manager.Enroll(ctx, bearer, email, currentPassword, approval, peer)
	if !errors.Is(err, usecase.ErrRecoveryCodeManagerDenied) || batch.ForTrustedDisplay()[0] != "" {
		t.Fatal("default-deny bootstrap issued a saved recovery code")
	}
	var count int
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM mamony_identity.password_recovery_offline_codes WHERE subject_id=$1",
		subject).Scan(&count); err != nil || count != 0 {
		t.Fatal("denied enrollment wrote offline code digests")
	}
	// Admin-only fixture simulates a separate independent factor authority;
	// there is NO production role or API capable of inserting this row.
	approvalDigest, err := usecase.RecoveryEnrollmentApprovalDigest(approval)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_recovery_code_enrollment_approvals
		(subject_id,subject_epoch,proof_digest,approved_at,expires_at)
		VALUES($1,$2,$3,clock_timestamp(),clock_timestamp()+interval '4 minutes')`,
		subject, epoch, approvalDigest[:]); err != nil {
		t.Fatal(err)
	}
	batch, err = manager.Enroll(ctx, bearer, email, currentPassword, approval, peer)
	if err != nil {
		t.Fatalf("approved enrollment denied: %v", err)
	}
	issued := batch.ForTrustedDisplay()
	if len(issued) != 5 {
		t.Fatal("enrollment returned wrong number of raw offline codes")
	}
	for _, rawCode := range issued {
		if len(rawCode) != 43 {
			t.Fatal("committed offline code did not have 256-bit encoding")
		}
		digest, err := usecase.OfflineRecoveryCodeDigest(rawCode)
		if err != nil {
			t.Fatal(err)
		}
		var owned bool
		if err := admin.QueryRow(ctx, `SELECT count(*)=1
			FROM mamony_identity.password_recovery_offline_codes
			WHERE subject_id=$1 AND code_digest=$2 AND consumed_at IS NULL`,
			subject, digest[:]).Scan(&owned); err != nil || !owned {
			t.Fatal("committed saved code missing its digest-only row")
		}
	}
