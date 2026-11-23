//go:build integration

package identity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityZPasskeyActivationChallenge tests v34 issuer/reader against
// a genuine v33 SQL-staged, NOT ACTIVATED credential on disposable PostgreSQL.
// The candidate is synthetic TEST-ONLY staging data (not a verified signed
// browser assertion). v33 WebAuthn protocol correctness is tested separately.
func TestIdentityZPasskeyActivationChallenge(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 125*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	regIssuerDSN := guardedDB(t, "TEST_IDENTITY_PASSKEY_REGISTRATION_ISSUER_URL", "mamony_identity_passkey_registration_issuer_runtime")
	stageDSN := guardedDB(t, "TEST_IDENTITY_PASSKEY_REGISTRATION_STAGE_URL", "mamony_identity_passkey_registration_stage_runtime")
	activationIssuerDSN := guardedDB(t, "TEST_IDENTITY_PASSKEY_ACTIVATION_ISSUER_URL", "mamony_identity_passkey_activation_issuer_runtime")
	activationReaderDSN := guardedDB(t, "TEST_IDENTITY_PASSKEY_ACTIVATION_READER_URL", "mamony_identity_passkey_activation_reader_runtime")
	base, _ := url.Parse(adminDSN)
	for _, raw := range []string{sessionDSN, regIssuerDSN, stageDSN, activationIssuerDSN, activationReaderDSN} {
		u, _ := url.Parse(raw)
		if u.Host != base.Host || u.Path != base.Path {
			t.Fatal("activation contract requires one disposable database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: 5})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	sessStore, err := identitypg.NewOpaqueSessionStore(open(sessionDSN, "mamony_identity_session_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := usecase.NewOpaqueSessionAuthority(sessStore)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := usecase.NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	if err != nil {
		t.Fatal(err)
	}
	regStore, err := identitypg.NewPasskeyRegistrationIntentStore(
		open(regIssuerDSN, "mamony_identity_passkey_registration_issuer_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	regIssuer, err := usecase.NewPasskeyRegistrationIntentIssuer(sessions, regStore, policy)
	if err != nil {
		t.Fatal(err)
	}
	stageStore, err := identitypg.NewPasskeyRegistrationStageStore(
		open(stageDSN, "mamony_identity_passkey_registration_stage_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	activationStore, err := identitypg.NewPasskeyActivationChallengeStore(
		open(activationIssuerDSN, "mamony_identity_passkey_activation_issuer_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := usecase.NewPasskeyActivationChallengeIssuer(sessions, activationStore, policy)
	if err != nil {
		t.Fatal(err)
	}
	readerStore, err := identitypg.NewPasskeyActivationEvidenceStore(
		open(activationReaderDSN, "mamony_identity_passkey_activation_reader_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := usecase.NewPasskeyActivationEvidenceReader(sessions, readerStore, policy)
	if err != nil {
		t.Fatal(err)
	}

	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "activation_")) + "@example.test"
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES($1,clock_timestamp())",
		subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.email_aliases(canonical_email,subject_id,verified_at,created_at) "+
			"VALUES($1,$2,clock_timestamp(),clock_timestamp())", email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1",
		subject); err != nil {
		t.Fatal(err)
	}
	var epoch int64
	if err := admin.QueryRow(ctx, "SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
		subject).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	newBearer := func() (string, [32]byte) {
		t.Helper()
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			t.Fatal(err)
		}
		bearer := base64.RawURLEncoding.EncodeToString(raw)
		return bearer, sha256.Sum256(append([]byte("mamony:identity:session-token:v1\x00"), raw...))
	}
	bearer, digest := newBearer()
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.opaque_sessions(token_digest,subject_id,subject_epoch,issued_at,expires_at) "+
			"VALUES($1,$2,$3,clock_timestamp(),clock_timestamp()+interval '12 hours')",
		digest[:], subject, epoch); err != nil {
		t.Fatal(err)
	}
	registration, err := regIssuer.Begin(ctx, bearer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.passkey_registration_admissions "+
			"(subject_id,intent_id,session_digest,subject_epoch,contact_version,provenance,approved_at,expires_at) "+
			"SELECT subject_id,intent_id,session_digest,subject_epoch,contact_version,"+
			"'PASSWORD_REAUTH_ONLY',clock_timestamp(),expires_at "+
			"FROM mamony_identity.passkey_registration_intents WHERE subject_id=$1 AND intent_id=$2",
		subject, registration.IntentID()); err != nil {
		t.Fatal(err)
	}
	// Privileged TEST-only stage. The maintained WebAuthn verifier's real
	// positive/negative cases are tested in v33, not bypassed in production.
	credentialID := bytes.Repeat([]byte{0x81}, 32)
	signer, err := identitywa.NewSyntheticActivationSigner()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := signer.PublicCOSE()
	if err != nil {
		t.Fatal(err)
	}
	candidate := usecase.PasskeyRegistrationCandidate{
		CredentialID:      credentialID,
		PublicKeyCOSE:     publicKey,
		AAGUID:            make([]byte, 16),
		AttestationFormat: "none",
		AttestationType:   "none",
		UserPresent:       true,
		UserVerified:      true,
	}
	copyOfNonce := [32]byte{}
	copy(copyOfNonce[:], registration.Challenge())
	ok, err := stageStore.Stage(ctx, digest, registration.IntentID(),
		usecase.PasskeyRegistrationEvidence{
			Subject: subject, Epoch: epoch, Nonce: copyOfNonce,
			RPID: policy.RPID(), Origin: policy.Origin(), PolicyGeneration: 1,
		}, candidate, randomSyntheticID(t, "evt_"))
	if err != nil || !ok {
		t.Fatalf("v33 pending credential synthetic fixture did not persist: allowed=%v err=%v", ok, err)
	}

	// Same account, DIFFERENT valid live session cannot issue activation
	// for this credential: original registration session is authoritative.
	otherBearer, otherDigest := newBearer()
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.opaque_sessions(token_digest,subject_id,subject_epoch,issued_at,expires_at) "+
			"VALUES($1,$2,$3,clock_timestamp(),clock_timestamp()+interval '12 hours')",
		otherDigest[:], subject, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.Begin(ctx, otherBearer, credentialID); !errors.Is(err, usecase.ErrPasskeyActivationDenied) {
		t.Fatalf("second valid session issued original credential activation: %v", err)
	}
	if _, err := issuer.Begin(ctx, bearer, bytes.Repeat([]byte{0x82}, 32)); !errors.Is(err, usecase.ErrPasskeyActivationDenied) {
		t.Fatalf("unknown credential ID issued activation: %v", err)
	}

	// Concurrent issuance has exactly one winner; second request cannot
	// silently replace or read an unexpired challenge.
	type result struct {
		value usecase.PasskeyActivationChallenge
		err   error
	}
	outcomes := make(chan result, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			v, e := issuer.Begin(ctx, bearer, credentialID)
			outcomes <- result{value: v, err: e}
		})
	}
	wg.Wait()
	close(outcomes)
	successes, denials := 0, 0
	var issued usecase.PasskeyActivationChallenge
	for x := range outcomes {
		if x.err == nil {
			successes++
			issued = x.value
		} else if errors.Is(x.err, usecase.ErrPasskeyActivationDenied) {
			denials++
		} else {
			t.Fatalf("activation issue concurrency unexpected error: %v", x.err)
		}
	}
	if successes != 1 || denials != 1 || issued.ChallengeID() == "" ||
		bytes.Equal(issued.Challenge(), registration.Challenge()) {
		t.Fatalf("activation challenge not independent and single outstanding: successes=%d denied=%d", successes, denials)
	}
	if _, err := issuer.Begin(ctx, bearer, credentialID); !errors.Is(err, usecase.ErrPasskeyActivationDenied) {
		t.Fatalf("unexpired activation challenge silently replaced: %v", err)
	}
	evidence, err := reader.Read(ctx, bearer, issued.ChallengeID())
	if err != nil || evidence.Subject != subject || evidence.Epoch != epoch ||
		!bytes.Equal(evidence.Nonce[:], issued.Challenge()) ||
		!bytes.Equal(evidence.CredentialID, credentialID) ||
		!bytes.Equal(evidence.PublicKeyCOSE, publicKey) ||
		evidence.RPID != policy.RPID() || evidence.Origin != policy.Origin() ||
		evidence.PolicyGeneration != 1 {
		t.Fatalf("v34 reader did not preserve session-bound pending credential context: %v", err)
	}
	// v35: this is a REAL ES256 signed assertion, not a mocked crypto
	// verifier. The public key was retrieved through the isolated v34
	// PostgreSQL evidence reader and the nonce is the exact v34 issue.
	assertionProtocol, err := identitywa.NewActivationAssertionVerifier(policy)
	if err != nil {
		t.Fatal(err)
	}
	assertionVerifier, err := usecase.NewPasskeyActivationAssertionVerifier(reader, assertionProtocol)
	if err != nil {
		t.Fatal(err)
	}
	signedResponse, err := signer.SignedResponse(evidence.Nonce, credentialID, subject, 1)
	if err != nil {
		t.Fatal(err)
	}
	if verified, err := assertionVerifier.VerifyOnly(ctx, bearer, issued.ChallengeID(), signedResponse); err != nil || !verified.UserPresent || !verified.UserVerified ||
		verified.Counter != 1 || !bytes.Equal(verified.CredentialID, credentialID) {
		t.Fatalf("v35 signed activation assertion with DB-owned key rejected: %v", err)
	}
	// VerifyOnly MUST NOT consume: replay remains cryptographically valid.
	// The future v36 atomic commit must fence the nonce against replay.
	if _, err := assertionVerifier.VerifyOnly(ctx, bearer, issued.ChallengeID(), signedResponse); err != nil {
		t.Fatalf("read-only v35 verifier unexpectedly consumed challenge: %v", err)
	}
	if _, err := assertionVerifier.VerifyOnly(ctx, otherBearer, issued.ChallengeID(), signedResponse); !errors.Is(err, usecase.ErrPasskeyAssertionDenied) {
		t.Fatal("other live session was able to verify credential assertion")
	}
	badNonce := evidence.Nonce
	badNonce[0] ^= 0x40
	wrongResponse, err := signer.SignedResponse(badNonce, credentialID, subject, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := assertionVerifier.VerifyOnly(ctx, bearer, issued.ChallengeID(), wrongResponse); !errors.Is(err, usecase.ErrPasskeyAssertionDenied) {
		t.Fatal("v35 verified signature over a wrong challenge")
	}

	if _, err := reader.Read(ctx, otherBearer, issued.ChallengeID()); !errors.Is(err, usecase.ErrPasskeyActivationDenied) {
		t.Fatalf("second live session read original activation evidence: %v", err)
	}
	if _, err := reader.Read(ctx, bearer, "pav_BBBBBBBBBBBBBBBBBBBBBB"); !errors.Is(err, usecase.ErrPasskeyActivationDenied) {
		t.Fatal("unissued challenge ID leaked credential context")
	}
	var state string
	if err := admin.QueryRow(ctx,
		"SELECT state FROM mamony_identity.passkey_credentials WHERE credential_id=$1",
		credentialID).Scan(&state); err != nil || state != "PENDING_ASSERTION" {
		t.Fatal("nonce issuance unexpectedly activated a passkey")
	}
	var count int
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM mamony_identity.passkey_activation_challenges WHERE subject_id=$1 AND consumed_at IS NULL",
		subject).Scan(&count); err != nil || count != 1 {
		t.Fatal("concurrent issue created multiple outstanding challenges")
	}

	// Expiry must deny reader and permit a new challenge; old challenge
	// cannot regain validity or inherit a replacement's nonce.
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.passkey_activation_challenges "+
			"SET issued_at=clock_timestamp()-interval '2 minutes',"+
			"expires_at=clock_timestamp()-interval '1 minute' WHERE subject_id=$1",
		subject); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Read(ctx, bearer, issued.ChallengeID()); !errors.Is(err, usecase.ErrPasskeyActivationDenied) {
		t.Fatal("expired activation challenge remained readable")
	}
	replaced, err := issuer.Begin(ctx, bearer, credentialID)
	if err != nil || replaced.ChallengeID() == issued.ChallengeID() {
		t.Fatalf("old activation nonce not rotated after expiry: %v", err)
	}
	if _, err := reader.Read(ctx, bearer, issued.ChallengeID()); !errors.Is(err, usecase.ErrPasskeyActivationDenied) {
		t.Fatal("superseded challenge ID was readable after replacement")
	}
	if _, err := reader.Read(ctx, bearer, replaced.ChallengeID()); err != nil {
		t.Fatalf("new activation nonce unexpectedly denied: %v", err)
	}
	// Epoch change invalidates session and all pending activation contexts.
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.subjects SET session_epoch=session_epoch+1 WHERE subject_id=$1",
		subject); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Read(ctx, bearer, replaced.ChallengeID()); !errors.Is(err, usecase.ErrPasskeyActivationDenied) {
		t.Fatal("stale subject epoch released assertion context")
	}
	if _, err := issuer.Begin(ctx, bearer, credentialID); !errors.Is(err, usecase.ErrPasskeyActivationDenied) {
		t.Fatal("stale subject epoch issued a fresh activation nonce")
	}
	if err := admin.QueryRow(ctx,
		"SELECT state FROM mamony_identity.passkey_credentials WHERE credential_id=$1",
		credentialID).Scan(&state); err != nil || state != "PENDING_ASSERTION" {
		t.Fatal("v34 made credential login capable")
	}
}
