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
	var approvalConsumed bool
	if err := admin.QueryRow(ctx, `SELECT consumed_at IS NOT NULL
		FROM mamony_identity.password_recovery_code_enrollment_approvals
		WHERE subject_id=$1`, subject).Scan(&approvalConsumed); err != nil || !approvalConsumed {
		t.Fatal("successful enrollment failed to consume independent approval")
	}
	// The encrypted recovery request and any associated grant must be
	// revoked when an authenticated user rotates the offline set.
	challenge := randomSyntheticID(t, "rcv_")
	recoveryDigest := sha256.Sum256([]byte("synthetic pending email recovery before rotation " + subject))
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_recovery_challenges
		(challenge_id,subject_id,canonical_email,contact_version,subject_epoch,
		 token_digest,requested_at,expires_at)
		VALUES($1,$2,$3,1,$4,$5,clock_timestamp(),clock_timestamp()+interval '15 minutes')`,
		challenge, subject, email, epoch, recoveryDigest[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_recovery_deliveries
		(challenge_id,key_id,sealed_token) VALUES($1,'testcode', $2)`,
		challenge, bytes.Repeat([]byte{0x51}, 71)); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_recovery_assurance_grants
		(subject_id,subject_epoch,challenge_id,approved_at,expires_at)
		VALUES($1,$2,$3,clock_timestamp(),clock_timestamp()+interval '4 minutes')`,
		subject, epoch, challenge); err != nil {
		t.Fatal(err)
	}
	// Wrong code must NOT rotate the set, consume a real older code, or
	// revoke pending recovery challenges/approvals.
	other, err := usecase.NewOfflineRecoveryCode()
	if err != nil {
		t.Fatal(err)
	}
	if denied, err := manager.Rotate(ctx, bearer, email, currentPassword, other.TrustedOfflineDisplay(), peer); !errors.Is(err, usecase.ErrRecoveryCodeManagerDenied) || denied.ForTrustedDisplay()[0] != "" {
		t.Fatalf("wrong saved recovery code was not generically denied: err=%v rawReturned=%t",
			err, denied.ForTrustedDisplay()[0] != "")
	}
	var challengeRevoked bool
	if err := admin.QueryRow(ctx, `SELECT revoked_at IS NOT NULL
		FROM mamony_identity.password_recovery_challenges WHERE challenge_id=$1`,
		challenge).Scan(&challengeRevoked); err != nil || challengeRevoked {
		t.Fatal("failed rotation revoked a valid recovery challenge")
	}
	rotated, err := manager.Rotate(ctx, bearer, email, currentPassword, issued[0], peer)
	if err != nil {
		t.Fatalf("valid password+session+saved-code rotation denied: %v", err)
	}
	fresh := rotated.ForTrustedDisplay()
	if len(fresh) != 5 || fresh[0] == "" {
		t.Fatal("committed rotation withheld new saved codes")
	}
	d0, err := usecase.OfflineRecoveryCodeDigest(issued[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.password_recovery_offline_codes
		WHERE subject_id=$1 AND code_digest=$2`, subject, d0[:]).Scan(&count); err != nil || count != 0 {
		t.Fatal("rotated-out code remains usable or stored")
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.password_recovery_offline_codes
		WHERE subject_id=$1 AND consumed_at IS NULL`, subject).Scan(&count); err != nil || count != 5 {
		t.Fatal("rotation did not replace full set atomically")
	}
	if err := admin.QueryRow(ctx, `SELECT revoked_at IS NOT NULL
		FROM mamony_identity.password_recovery_challenges WHERE challenge_id=$1`,
		challenge).Scan(&challengeRevoked); err != nil || !challengeRevoked {
		t.Fatal("rotated recovery codes did not revoke outstanding emailed proof")
	}
	var deliveryState string
	if err := admin.QueryRow(ctx, "SELECT status FROM mamony_identity.password_recovery_deliveries WHERE challenge_id=$1",
		challenge).Scan(&deliveryState); err != nil || deliveryState != "DEAD" {
		t.Fatal("old recovery-link mail not quarantined after code rotation")
	}
	var priorApprovalConsumed bool
	if err := admin.QueryRow(ctx, `SELECT consumed_at IS NOT NULL
		FROM mamony_identity.password_recovery_assurance_grants WHERE subject_id=$1`,
		subject).Scan(&priorApprovalConsumed); err != nil || !priorApprovalConsumed {
		t.Fatal("old password-recovery approval survived new offline code set")
	}
	var enrolled, rotatedEvents, notices int
	if err := admin.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM mamony_identity.security_outbox o WHERE o.subject_id=$1
				AND o.event_type='RECOVERY_CODES_ENROLLED'),
			(SELECT count(*) FROM mamony_identity.security_outbox o WHERE o.subject_id=$1
				AND o.event_type='RECOVERY_CODES_ROTATED'),
			(SELECT count(*) FROM mamony_identity.password_recovery_code_notices n WHERE n.subject_id=$1
				AND n.status='PENDING')`, subject).Scan(&enrolled, &rotatedEvents, &notices); err != nil || enrolled != 1 || rotatedEvents != 1 || notices != 2 {
		t.Fatal("offline code set changes not atomically audited and queued for token-free notification")
	}
	// Replay of a prior code is denied even with the correct password;
	// reused enrollment approval is also denied once a set exists.
	if _, err := manager.Rotate(ctx, bearer, email, currentPassword, issued[0], peer); !errors.Is(err, usecase.ErrRecoveryCodeManagerDenied) {
		t.Fatal("burned recovery code authorized a later rotation")
	}
	if _, err := manager.Enroll(ctx, bearer, email, currentPassword, approval, peer); !errors.Is(err, usecase.ErrRecoveryCodeManagerDenied) {
		t.Fatal("used bootstrap approval allowed re-enrollment over existing code set")
	}

	// Use restricted auth-grant staging for a deterministic SQL rollback
	// test. This does not bypass the Go reauthentication tested above.
	grantConn, err := pgx.Connect(ctx, grantDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = grantConn.Close(context.Background()) }()
	rawSession, err := base64.RawURLEncoding.DecodeString(bearer)
	if err != nil {
		t.Fatal(err)
	}
	sessionDigest := sha256.Sum256(append([]byte("mamony:identity:session-token:v1\x00"), rawSession...))
	d1, err := usecase.OfflineRecoveryCodeDigest(fresh[0])
	if err != nil {
		t.Fatal(err)
	}
	newCodeDigests := func(seed string) [5][32]byte {
		var result [5][32]byte
		for i := 0; i < 5; i++ {
			result[i] = sha256.Sum256([]byte(seed + string(rune('a'+i)) + subject))
		}
		return result
	}
	stage := func(label string) [32]byte {
		t.Helper()
		digest := sha256.Sum256([]byte("synthetic stage recovery code change " + label + subject))
		var allowed bool
		if err := grantConn.QueryRow(ctx,
			"SELECT mamony_identity.stage_recovery_code_rotation_grant($1,$2,$3)",
			subject, epoch, digest[:]).Scan(&allowed); err != nil || !allowed {
			t.Fatalf("trusted fixture could not stage recovery-specific reauth grant: %v", err)
		}
		return digest
	}
	rollbackGrant := stage("rollback")
	collision := randomSyntheticID(t, "evt_")
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.security_outbox
		(event_id,subject_id,event_type,occurred_at)
		VALUES($1,$2,'ENROLLMENT_PENDING',clock_timestamp())`,
		collision, subject); err != nil {
		t.Fatal(err)
	}
	if ok, err := managerStore.Replace(ctx, sessionDigest, rollbackGrant, d1,
		newCodeDigests("rollback"), collision); ok || err == nil {
		t.Fatal("duplicate audit event committed offline code replacement")
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.password_recovery_offline_codes
		WHERE subject_id=$1 AND code_digest=$2 AND consumed_at IS NULL`,
		subject, d1[:]).Scan(&count); err != nil || count != 1 {
		t.Fatal("failed audit write removed previously saved offline code")
	}
	var rollbackConsumed bool
	if err := admin.QueryRow(ctx, `SELECT consumed_at IS NOT NULL
		FROM mamony_identity.password_login_grants WHERE grant_digest=$1`,
		rollbackGrant[:]).Scan(&rollbackConsumed); err != nil || rollbackConsumed {
		t.Fatal("failed enrollment audit committed a consumed reauthentication grant")
	}

	// Two concurrent rotations with identical grant and old code have one
	// transaction winner. Durable session/reauth proof never succeeds twice.
	racingGrant := stage("race")
	results := make(chan bool, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			ok, err := managerStore.Replace(ctx, sessionDigest, racingGrant, d1,
				newCodeDigests("concurrent "+string(rune('a'+index))), randomSyntheticID(t, "evt_"))
			results <- ok && err == nil
		}(i)
	}
	wg.Wait()
	close(results)
	winners := 0
	for ok := range results {
		if ok {
			winners++
		}
	}
	if winners != 1 {
		t.Fatal("concurrent recovery code replacement succeeded more than once")
	}
	if who, err := sessions.Resolve(ctx, bearer); err != nil || who != subject {
		t.Fatal("offline code rotation unexpectedly invalidated healthy session epoch")
	}
}
