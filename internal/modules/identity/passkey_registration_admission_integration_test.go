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
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityZPasskeyRegistrationAdmission requires real Argon2id,
// durable pre-KDF login admission and five distinct SQL roles. It cannot
// register a credential or mint an independent recovery factor.
func TestIdentityZPasskeyRegistrationAdmission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := guardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	authDSN := guardedDB(t, "TEST_IDENTITY_AUTHENTICATOR_URL", "mamony_identity_authenticator_runtime")
	grantDSN := guardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	issuerDSN := guardedDB(t, "TEST_IDENTITY_PASSKEY_REGISTRATION_ISSUER_URL", "mamony_identity_passkey_registration_issuer_runtime")
	admissionDSN := guardedDB(t, "TEST_IDENTITY_PASSKEY_REGISTRATION_ADMISSION_URL", "mamony_identity_passkey_registration_admission_runtime")
	base, _ := url.Parse(adminDSN)
	for _, raw := range []string{regDSN, authDSN, grantDSN, sessionDSN, issuerDSN, admissionDSN} {
		u, _ := url.Parse(raw)
		if u.Host != base.Host || u.Path != base.Path {
			t.Fatal("passkey reauth requires one disposable PostgreSQL database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	grantConn, err := pgx.Connect(ctx, grantDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = grantConn.Close(context.Background()) }()
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
	authStore, err := identitypg.NewAuthenticationStore(open(authDSN, "mamony_identity_authenticator_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	password, err := usecase.NewPasswordAuthentication(authStore, kdf)
	if err != nil {
		t.Fatal(err)
	}
	budgetStore, err := identitypg.NewAdmissionStore(open(regDSN, "mamony_identity_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	gate, err := usecase.NewLoginAdmission(budgetStore, bytes.Repeat([]byte{0xb9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	passwordFlow, err := usecase.NewPasswordLoginFlow(gate, password)
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
	reauthStore, err := identitypg.NewPasskeyRegistrationReauthGrantStore(grantPool)
	if err != nil {
		t.Fatal(err)
	}
	reauthGrants, err := usecase.NewPasskeyRegistrationReauthGrantAuthority(reauthStore)
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
	login, err := usecase.NewSessionLogin(passwordFlow, loginGrants, sessions)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := usecase.NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	if err != nil {
		t.Fatal(err)
	}
	intentStore, err := identitypg.NewPasskeyRegistrationIntentStore(open(issuerDSN, "mamony_identity_passkey_registration_issuer_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := usecase.NewPasskeyRegistrationIntentIssuer(sessions, intentStore, policy)
	if err != nil {
		t.Fatal(err)
	}
	admStore, err := identitypg.NewPasskeyRegistrationAdmissionStore(open(admissionDSN, "mamony_identity_passkey_registration_admission_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	reauth, err := usecase.NewAuthenticatedPasskeyRegistrationAdmission(sessions, passwordFlow, reauthGrants, admStore)
	if err != nil {
		t.Fatal(err)
	}

	// Only a disposable privileged fixture may create this synthetic account.
	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "passkey_adm_")) + "@example.test"
	secret := "synthetic password for passkey reauthentication"
	verifier, err := kdf.Hash(ctx, secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES($1,clock_timestamp())",
		subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.email_aliases(canonical_email,subject_id,verified_at,created_at) VALUES($1,$2,clock_timestamp(),clock_timestamp())",
		email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.password_verifiers(subject_id,kdf_version,memory_kib,iterations,parallelism,salt,digest,created_at) VALUES($1,1,65536,3,1,$2,$3,clock_timestamp())",
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

	peer := netip.MustParseAddr("198.51.100.154")
	first, err := login.Authenticate(ctx, email, secret, peer)
	if err != nil {
		t.Fatal(err)
	}
	bearer := first.ForTrustedTransport()
	intent, err := issuer.Begin(ctx, bearer)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	countAdmissions := func() int {
		t.Helper()
		if err := admin.QueryRow(ctx,
			"SELECT count(*) FROM mamony_identity.passkey_registration_admissions WHERE subject_id=$1",
			subject).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	if countAdmissions() != 0 {
		t.Fatal("nonce alone created reauth admission")
	}

	// A second valid session for the SAME account cannot admit another
	// session's challenge, even after a successful password verification.
	second, err := login.Authenticate(ctx, email, secret, peer)
	if err != nil {
		t.Fatal(err)
	}
	if err := reauth.Admit(ctx, second.ForTrustedTransport(), intent.IntentID(),
		email, secret, peer); !errors.Is(err, usecase.ErrPasskeyAdmissionDenied) {
		t.Fatalf("different live session admitted original intent: %v", err)
	}
	if err := reauth.Admit(ctx, bearer, intent.IntentID(),
		email, secret+"wrong", peer); !errors.Is(err, usecase.ErrPasskeyAdmissionDenied) {
		t.Fatalf("wrong password admitted intent: %v", err)
	}
	if countAdmissions() != 0 {
		t.Fatal("denied reauth persisted admission")
	}

	// A PASSWORD_LOGIN_SESSION grant cannot authorize passkey enrollment.
	raw, err := base64.RawURLEncoding.DecodeString(bearer)
	if err != nil {
		t.Fatal(err)
	}
	sessionDigest := sha256.Sum256(append([]byte("mamony:identity:session-token:v1\x00"), raw...))
	wrongPurpose := sha256.Sum256([]byte("passkey-wrong-purpose-" + subject))
	var staged bool
	if err := grantConn.QueryRow(ctx, "SELECT mamony_identity.stage_password_auth_grant($1,$2,$3)",
		subject, epoch, wrongPurpose[:]).Scan(&staged); err != nil || !staged {
		t.Fatalf("stage wrong-purpose fixture failed: %v", err)
	}
	if accepted, err := admStore.Admit(ctx, sessionDigest, intent.IntentID(),
		wrongPurpose); err != nil || accepted {
		t.Fatalf("login grant admitted passkey: accepted=%t err=%v", accepted, err)
	}

	if err := reauth.Admit(ctx, bearer, intent.IntentID(), email, secret, peer); err != nil {
		t.Fatalf("real password reauth was not admitted: %v", err)
	}
	var storedID, provenance string
	var storedSession []byte
	var storedEpoch int64
	var usedProofs int
	if err := admin.QueryRow(ctx, "SELECT intent_id,provenance,session_digest,subject_epoch, "+
		"(SELECT count(*) FROM mamony_identity.password_login_grants WHERE subject_id=$1 "+
		"AND purpose='PASSKEY_REGISTRATION_REAUTH' AND consumed_at IS NOT NULL) "+
		"FROM mamony_identity.passkey_registration_admissions WHERE subject_id=$1",
		subject).Scan(&storedID, &provenance, &storedSession, &storedEpoch, &usedProofs); err != nil ||
		storedID != intent.IntentID() || provenance != "PASSWORD_REAUTH_ONLY" ||
		!bytes.Equal(storedSession, sessionDigest[:]) || storedEpoch != epoch || usedProofs != 1 {
		t.Fatalf("trusted v32 admission not one-session/password-bound: %v", err)
	}
	if err := reauth.Admit(ctx, bearer, intent.IntentID(),
		email, secret, peer); !errors.Is(err, usecase.ErrPasskeyAdmissionDenied) {
		t.Fatalf("same live challenge admission replayed: %v", err)
	}
	var previousRaw []byte
	if err := admin.QueryRow(ctx, "SELECT grant_digest FROM mamony_identity.password_login_grants "+
		"WHERE subject_id=$1 AND purpose='PASSKEY_REGISTRATION_REAUTH' AND consumed_at IS NOT NULL "+
		"ORDER BY issued_at DESC LIMIT 1", subject).Scan(&previousRaw); err != nil {
		t.Fatal(err)
	}
	var previous [32]byte
	if len(previousRaw) != 32 {
		t.Fatal("invalid proof digest")
	}
	copy(previous[:], previousRaw)
	if accepted, err := admStore.Admit(ctx, sessionDigest, intent.IntentID(), previous); err != nil || accepted {
		t.Fatalf("consumed proof replayed: accepted=%t err=%v", accepted, err)
	}
	// A new v30 nonce must never inherit the prior admission, even though
	// the subject and issuing session are still identical. The old admission
	// row remains but its intent ID no longer matches the current challenge.
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.passkey_registration_intents "+
		"SET expires_at=issued_at+interval '1 millisecond' WHERE subject_id=$1",
		subject); err != nil {
		t.Fatal(err)
	}
	replacement, err := issuer.Begin(ctx, bearer)
	if err != nil || replacement.IntentID() == intent.IntentID() {
		t.Fatalf("expired v30 intent did not rotate: %v", err)
	}
	var admissionIntent string
	if err := admin.QueryRow(ctx,
		"SELECT intent_id FROM mamony_identity.passkey_registration_admissions WHERE subject_id=$1",
		subject).Scan(&admissionIntent); err != nil || admissionIntent != intent.IntentID() {
		t.Fatal("new nonce silently inherited the previous password reauth")
	}
	// Only a NEW purpose-specific grant can approve a superseding nonce;
	// in this disposable SQL fixture, the trusted grant role simulates a
	// separate already-verified password event.
	replacementProof := sha256.Sum256([]byte("reauth-for-replacement-" + subject))
	if err := grantConn.QueryRow(ctx,
		"SELECT mamony_identity.stage_passkey_registration_reauth_grant($1,$2,$3)",
		subject, epoch, replacementProof[:]).Scan(&staged); err != nil || !staged {
		t.Fatalf("could not stage independent replacement reauth assertion: %v", err)
	}
	if accepted, err := admStore.Admit(ctx, sessionDigest, replacement.IntentID(),
		replacementProof); err != nil || !accepted {
		t.Fatalf("separately proven new intent not admitted: %t %v", accepted, err)
	}
	if countAdmissions() != 1 {
		t.Fatal("passkey admission cardinality exceeded one per subject")
	}
	if err := admin.QueryRow(ctx,
		"SELECT intent_id FROM mamony_identity.passkey_registration_admissions WHERE subject_id=$1",
		subject).Scan(&admissionIntent); err != nil || admissionIntent != replacement.IntentID() {
		t.Fatal("new purpose-bound grant failed to replace old intent binding")
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.subjects SET session_epoch=session_epoch+1 WHERE subject_id=$1",
		subject); err != nil {
		t.Fatal(err)
	}
	if err := reauth.Admit(ctx, bearer, intent.IntentID(),
		email, secret, peer); !errors.Is(err, usecase.ErrPasskeyAdmissionDenied) {
		t.Fatal("stale session epoch admitted passkey")
	}
	var independent int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM mamony_identity.password_recovery_code_enrollment_approvals WHERE subject_id=$1",
		subject).Scan(&independent); err != nil || independent != 0 {
		t.Fatal("password-backed admission minted independent recovery approval")
	}
}
