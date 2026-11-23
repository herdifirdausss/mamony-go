//go:build integration

package identity

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/credential"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityZRecoveryFactorSignedChallenge uses live Argon2id+session
// admission with an Ed25519 key provisioned ONLY by disposable admin fixture.
func TestIdentityZRecoveryFactorSignedChallenge(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
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

	issuerDSN := guardedDB(t, "TEST_IDENTITY_RECOVERY_FACTOR_ISSUER_URL", "mamony_identity_recovery_factor_issuer_runtime")
	verifierDSN := guardedDB(t, "TEST_IDENTITY_RECOVERY_FACTOR_VERIFIER_URL", "mamony_identity_recovery_factor_verifier_runtime")
	root, _ := url.Parse(adminDSN)
	for _, raw := range []string{issuerDSN, verifierDSN} {
		u, _ := url.Parse(raw)
		if u.Host != root.Host || u.Path != root.Path {
			t.Fatal("independent factor authorities must share disposable database")
		}
	}
	issueStore, err := identitypg.NewRecoveryFactorIssuerStore(open(issuerDSN, "mamony_identity_recovery_factor_issuer_runtime"))
	if err != nil { t.Fatal(err) }
	verificationStore, err := identitypg.NewRecoveryFactorVerifierStore(open(verifierDSN, "mamony_identity_recovery_factor_verifier_runtime"))
	if err != nil { t.Fatal(err) }
	issuer, err := usecase.NewRecoveryFactorChallengeIssuer(sessions,loginFlow,codeGrants,issueStore)
	if err != nil { t.Fatal(err) }
	authority, err := usecase.NewRecoveryFactorApprovalVerifier(verificationStore)
	if err != nil { t.Fatal(err) }
	// A valid session/password is INSUFFICIENT without a separately,
	// previously provisioned independent signing key.
	if denied,err:=issuer.Issue(ctx,bearer,email,currentPassword,peer);
		!errors.Is(err,usecase.ErrRecoveryFactorDenied) || denied.ChallengeID()!="" {
		t.Fatal("unprovisioned factor issued a bootstrap challenge")
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { t.Fatal(err) }
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.recovery_code_factor_keys
		(subject_id,public_key,bound_epoch,bound_contact_version,bound_at)
		VALUES($1,$2,$3,1,clock_timestamp())`,subject,[]byte(publicKey),epoch); err != nil {
		t.Fatal(err)
	}
	// The factor cannot be provisioned through either production SQL role,
	// nor through the anonymous recovery flows (verified separately).
	challenge,err:=issuer.Issue(ctx,bearer,email,currentPassword,peer)
	if err!=nil || len(challenge.ChallengeID())!=26 ||
		len(challenge.ForTrustedAuthenticator())<100 {
		t.Fatalf("valid pre-provisioned factor challenge denied: %v",err)
	}
	_, attackerKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { t.Fatal(err) }
	badSignature:=ed25519.Sign(attackerKey,challenge.ForTrustedAuthenticator())
	if denied,err:=authority.Verify(ctx,challenge.ChallengeID(),badSignature);
		!errors.Is(err,usecase.ErrRecoveryFactorDenied) ||
		denied.ForTrustedEnrollment()!="" {
		t.Fatal("attacker's key issued independent factor approval")
	}
	var failures int
	if err:=admin.QueryRow(ctx,`SELECT failed_attempts FROM mamony_identity.recovery_code_factor_challenges
		WHERE challenge_id=$1`,challenge.ChallengeID()).Scan(&failures);
		err!=nil || failures!=1 {
		t.Fatal("bad signed proof was not charged against the challenge")
	}
	realSignature:=ed25519.Sign(privateKey,challenge.ForTrustedAuthenticator())
	approval,err:=authority.Verify(ctx,challenge.ChallengeID(),realSignature)
	if err!=nil || len(approval.ForTrustedEnrollment())!=43 {
		t.Fatalf("independent factor signature not accepted: %v",err)
	}
	if fmt.Sprintf("%v",approval)==approval.ForTrustedEnrollment() {
		t.Fatal("independent approval leaked in default formatting")
	}
	if out,err:=json.Marshal(approval);err==nil||len(out)!=0{
		t.Fatal("independent approval allowed JSON bearer serialization")
	}
	var approvedCount,auditCount int
	if err:=admin.QueryRow(ctx,`SELECT count(*) FROM mamony_identity.password_recovery_code_enrollment_approvals
		WHERE subject_id=$1 AND consumed_at IS NULL AND expires_at>clock_timestamp()`,
		subject).Scan(&approvedCount);err!=nil||approvedCount!=1 {
		t.Fatal("valid signature did not commit one-use v25 bootstrap approval")
	}
	if err:=admin.QueryRow(ctx,`SELECT count(*) FROM mamony_identity.security_outbox
		WHERE subject_id=$1 AND event_type='RECOVERY_FACTOR_APPROVED'`,
		subject).Scan(&auditCount);err!=nil||auditCount!=1{
		t.Fatal("factor proof approval was not identifier-only audited")
	}
	if _,err:=authority.Verify(ctx,challenge.ChallengeID(),realSignature);
		!errors.Is(err,usecase.ErrRecoveryFactorDenied) {
		t.Fatal("replay of a valid signature created another approval")
	}
	batch,err:=manager.Enroll(ctx,bearer,email,currentPassword,approval.ForTrustedEnrollment(),peer)
	if err!=nil || batch.ForTrustedDisplay()[0]=="" {
		t.Fatalf("genuine v25 enrollment refused newly verified cryptographic factor: %v",err)
	}
	var codes int
	if err:=admin.QueryRow(ctx,`SELECT count(*) FROM mamony_identity.password_recovery_offline_codes
		WHERE subject_id=$1 AND consumed_at IS NULL`,subject).Scan(&codes);err!=nil||codes!=5 {
		t.Fatal("signed approval did not atomically enroll five saved recovery codes")
	}
	var consumed bool
	if err:=admin.QueryRow(ctx,`SELECT consumed_at IS NOT NULL FROM mamony_identity.password_recovery_code_enrollment_approvals
		WHERE subject_id=$1`,subject).Scan(&consumed);err!=nil||!consumed{
		t.Fatal("enrollment did not consume one-use cryptographic approval")
	}
	// Even a still-valid session and pre-provisioned signer cannot bootstrap
	// again once recovery codes were enrolled; rotation needs an old code.
	if later,err:=issuer.Issue(ctx,bearer,email,currentPassword,peer);
		!errors.Is(err,usecase.ErrRecoveryFactorDenied) || later.ChallengeID()!=""{
		t.Fatal("existing code set permitted a fresh bootstrap ceremony")
	}
	// Pre-existing challenges go stale after credential/contact epoch moves.
	stale := randomSyntheticID(t,"fac_")
	nonce := make([]byte,32)
	if _,err:=rand.Read(nonce);err!=nil{t.Fatal(err)}
	if _,err:=admin.Exec(ctx,`INSERT INTO mamony_identity.recovery_code_factor_challenges
		(challenge_id,subject_id,subject_epoch,nonce,issued_at,expires_at)
		VALUES($1,$2,$3,$4,clock_timestamp(),clock_timestamp()+interval '2 minutes')`,
		stale,subject,epoch,nonce);err!=nil{
		t.Fatal(err)
	}
	if _,err:=admin.Exec(ctx,`UPDATE mamony_identity.subjects
		SET session_epoch=session_epoch+1 WHERE subject_id=$1`,subject);err!=nil{
		t.Fatal(err)
	}
	if evidence,found,err:=verificationStore.Read(ctx,stale);
		err!=nil||found||evidence.Subject!=""{
		t.Fatal("epoch-stale cryptographic challenge was readable")
	}
}
