//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/integration/internal/fixture"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	identitywa "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/webauthn"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// This fixture constructs a structurally complete ES256, fmt:none WebAuthn
// response. The library verifies the real challenge, RP hash, type, origin,
// UP and UV; no attestation-none response claims independent device trust.
func stagedPasskeyResponse(t *testing.T, nonce []byte, credentialID []byte) []byte {
	t.Helper()
	payload, err := identitywa.SyntheticRegistrationResponse(nonce, credentialID)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// TestIdentityZPasskeyRegistrationAtomicStage uses real PostgreSQL and
// maintained go-webauthn protocol verification through the unmounted Go
// stage usecase. The v32 password-only provenance is seeded by an isolated
// ADMIN fixture: its real Argon2id path is tested in the preceding v32 test.
// Never use this fixture to claim production enrollment authority.
func TestIdentityZPasskeyRegistrationAtomicStage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 135*time.Second)
	defer cancel()
	adminDSN := fixture.GuardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	sessionDSN := fixture.GuardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	issuerDSN := fixture.GuardedDB(t, "TEST_IDENTITY_PASSKEY_REGISTRATION_ISSUER_URL", "mamony_identity_passkey_registration_issuer_runtime")
	readerDSN := fixture.GuardedDB(t, "TEST_IDENTITY_PASSKEY_REGISTRATION_READER_URL", "mamony_identity_passkey_registration_reader_runtime")
	stageDSN := fixture.GuardedDB(t, "TEST_IDENTITY_PASSKEY_REGISTRATION_STAGE_URL", "mamony_identity_passkey_registration_stage_runtime")
	base, _ := url.Parse(adminDSN)
	for _, raw := range []string{sessionDSN, issuerDSN, readerDSN, stageDSN} {
		u, _ := url.Parse(raw)
		if u.Host != base.Host || u.Path != base.Path {
			t.Fatal("passkey atomic stage requires a single disposable database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		pool, err := platformpg.Open(ctx, platformpg.Config{
			DSN: dsn, RuntimeRole: role, MaxConns: 5,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		return pool
	}
	sessionStore, err := identitypg.NewOpaqueSessionStore(open(sessionDSN, "mamony_identity_session_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := usecase.NewOpaqueSessionAuthority(sessionStore)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := usecase.NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	if err != nil {
		t.Fatal(err)
	}
	intentStore, err := identitypg.NewPasskeyRegistrationIntentStore(
		open(issuerDSN, "mamony_identity_passkey_registration_issuer_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := usecase.NewPasskeyRegistrationIntentIssuer(sessions, intentStore, policy)
	if err != nil {
		t.Fatal(err)
	}
	evidenceStore, err := identitypg.NewPasskeyRegistrationEvidenceStore(
		open(readerDSN, "mamony_identity_passkey_registration_reader_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	evidenceReader, err := usecase.NewPasskeyRegistrationEvidenceReader(
		sessions, evidenceStore, policy)
	if err != nil {
		t.Fatal(err)
	}
	protocol, err := identitywa.NewRegistrationVerifier(policy)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := usecase.NewPasskeyRegistrationProofVerifier(evidenceReader, protocol)
	if err != nil {
		t.Fatal(err)
	}
	stageStore, err := identitypg.NewPasskeyRegistrationStageStore(
		open(stageDSN, "mamony_identity_passkey_registration_stage_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	stager, err := usecase.NewPasskeyRegistrationStager(proof, stageStore)
	if err != nil {
		t.Fatal(err)
	}
	subject := fixture.RandomID(t, "sub_")
	email := strings.ToLower(fixture.RandomID(t, "stage_")) + "@example.test"
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
		"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1",
		subject); err != nil {
		t.Fatal(err)
	}
	var epoch int64
	if err := admin.QueryRow(ctx,
		"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
		subject).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	raw := bytes.Repeat([]byte{0x69}, 32)
	bearer := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256(append([]byte("mamony:identity:session-token:v1\x00"), raw...))
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.opaque_sessions(token_digest,subject_id,subject_epoch,issued_at,expires_at) "+
			"VALUES($1,$2,$3,clock_timestamp(),clock_timestamp()+interval '12 hours')",
		digest[:], subject, epoch); err != nil {
		t.Fatal(err)
	}
	// Only privileged test code may create the already-proven v32 admission.
	// It is bound to this exact existing nonce; v32's real reauth is
	// exercised separately in TestIdentityZPasskeyRegistrationAdmission.
	admitTestOnly := func(id string) {
		t.Helper()
		_, err := admin.Exec(ctx,
			"INSERT INTO mamony_identity.passkey_registration_admissions "+
				"(subject_id,intent_id,session_digest,subject_epoch,contact_version,provenance,approved_at,expires_at) "+
				"SELECT subject_id,intent_id,session_digest,subject_epoch,contact_version,"+
				"'PASSWORD_REAUTH_ONLY',clock_timestamp(),expires_at "+
				"FROM mamony_identity.passkey_registration_intents WHERE subject_id=$1 AND intent_id=$2 "+
				"ON CONFLICT(subject_id) DO UPDATE SET "+
				"intent_id=EXCLUDED.intent_id,session_digest=EXCLUDED.session_digest,"+
				"subject_epoch=EXCLUDED.subject_epoch,contact_version=EXCLUDED.contact_version,"+
				"provenance=EXCLUDED.provenance,approved_at=EXCLUDED.approved_at,"+
				"expires_at=EXCLUDED.expires_at,consumed_at=NULL",
			subject, id)
		if err != nil {
			t.Fatal(err)
		}
	}
	issue := func() (string, []byte) {
		t.Helper()
		intent, err := issuer.Begin(ctx, bearer)
		if err != nil {
			t.Fatal(err)
		}
		return intent.IntentID(), intent.Challenge()
	}
	count := func(table string) int {
		t.Helper()
		query := ""
		switch table {
		case "passkey_credentials":
			query = "SELECT count(*) FROM mamony_identity.passkey_credentials WHERE subject_id=$1"
		case "passkey_registration_notices":
			query = "SELECT count(*) FROM mamony_identity.passkey_registration_notices WHERE subject_id=$1"
		case "security_outbox":
			query = "SELECT count(*) FROM mamony_identity.security_outbox WHERE subject_id=$1 AND event_type='PASSKEY_REGISTRATION_STAGED'"
		default:
			t.Fatal("unexpected test table")
		}
		var n int
		if err := admin.QueryRow(ctx, query, subject).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	consumed := func(id string) (bool, bool) {
		t.Helper()
		var nonceSpent, admissionSpent bool
		if err := admin.QueryRow(ctx,
			"SELECT i.consumed_at IS NOT NULL,a.consumed_at IS NOT NULL "+
				"FROM mamony_identity.passkey_registration_intents i "+
				"JOIN mamony_identity.passkey_registration_admissions a ON a.subject_id=i.subject_id "+
				"WHERE i.subject_id=$1 AND i.intent_id=$2 AND a.intent_id=$2",
			subject, id).Scan(&nonceSpent, &admissionSpent); err != nil {
			t.Fatal(err)
		}
		return nonceSpent, admissionSpent
	}
	firstID, firstNonce := issue()
	firstCredential := bytes.Repeat([]byte{0x41}, 32)
	firstPayload := stagedPasskeyResponse(t, firstNonce, firstCredential)
	if err := stager.Stage(ctx, bearer, firstID, firstPayload); !errors.Is(err, usecase.ErrPasskeyStageDenied) || count("passkey_credentials") != 0 {
		t.Fatalf("missing v32 approval staged a credential: %v", err)
	}
	admitTestOnly(firstID)
	wrongNonce := append([]byte(nil), firstNonce...)
	wrongNonce[0] ^= 0x80
	if err := stager.Stage(ctx, bearer, firstID,
		stagedPasskeyResponse(t, wrongNonce, firstCredential)); !errors.Is(err, usecase.ErrPasskeyStageDenied) {
		t.Fatal("invalid authenticator challenge changed SQL state")
	}
	if err := stager.Stage(ctx, bearer, firstID, firstPayload); err != nil {
		t.Fatalf("verified staged credential was not committed: %v", err)
	}
	if count("passkey_credentials") != 1 ||
		count("passkey_registration_notices") != 1 ||
		count("security_outbox") != 1 {
		t.Fatal("credential, audit and mandatory notice are not 1:1")
	}
	nonceSpent, admissionSpent := consumed(firstID)
	if !nonceSpent || !admissionSpent {
		t.Fatal("successful commit failed to atomically consume both proofs")
	}
	var state, provenance, recipient, noticeStatus, eventType, eventID string
	if err := admin.QueryRow(ctx,
		"SELECT c.state,c.provenance,n.recipient,n.status,o.event_type,o.event_id "+
			"FROM mamony_identity.passkey_credentials c "+
			"JOIN mamony_identity.passkey_registration_notices n ON n.event_id=c.event_id "+
			"JOIN mamony_identity.security_outbox o ON o.event_id=c.event_id "+
			"WHERE c.credential_id=$1 AND c.subject_id=$2",
		firstCredential, subject).Scan(&state, &provenance, &recipient,
		&noticeStatus, &eventType, &eventID); err != nil ||
		state != "PENDING_ASSERTION" || provenance != "PASSWORD_REAUTH_ONLY" ||
		recipient != email || noticeStatus != "PENDING" ||
		eventType != "PASSKEY_REGISTRATION_STAGED" {
		t.Fatalf("privilege boundary or pending notification missing: %v", err)
	}
	if err := stager.Stage(ctx, bearer, firstID, firstPayload); !errors.Is(err, usecase.ErrPasskeyStageDenied) {
		t.Fatal("consumed nonce replayed after commit")
	}

	// A newly issued intent has its OWN admission. Reusing the same
	// credential ID fails globally, and does not consume either proof
	// or commit a second audit/notification.
	secondID, secondNonce := issue()
	admitTestOnly(secondID)
	if err := stager.Stage(ctx, bearer, secondID,
		stagedPasskeyResponse(t, secondNonce, firstCredential)); !errors.Is(err, usecase.ErrPasskeyStageUnavailable) {
		t.Fatalf("globally duplicate credential accepted: %v", err)
	}
	if nonceSpent, admissionSpent := consumed(secondID); nonceSpent || admissionSpent {
		t.Fatal("duplicate credential committed partial proof consumption")
	}
	if count("passkey_credentials") != 1 || count("security_outbox") != 1 ||
		count("passkey_registration_notices") != 1 {
		t.Fatal("late uniqueness violation committed partial notification or key")
	}

	// Audit event collision also fails atomically BEFORE writing any row.
	// This direct call simulates a compromised staging SQL principal in a
	// disposable fixture; protocol verification was done above and the
	// production usecase never accepts caller-chosen audit IDs.
	secondCredential := bytes.Repeat([]byte{0x42}, 32)
	exactEvidence, err := evidenceReader.Read(ctx, bearer, secondID)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := proof.VerifyOnly(ctx, bearer, secondID,
		stagedPasskeyResponse(t, secondNonce, secondCredential))
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := stageStore.Stage(ctx, digest, secondID, exactEvidence,
		candidate, eventID); err == nil || ok {
		t.Fatal("conflicting audit event did not roll back staging")
	}
	if nonceSpent, admissionSpent := consumed(secondID); nonceSpent || admissionSpent {
		t.Fatal("duplicate audit event consumed challenge or admission")
	}
	if err := stager.Stage(ctx, bearer, secondID,
		stagedPasskeyResponse(t, secondNonce, secondCredential)); err != nil {
		t.Fatalf("retry with unique audit and credential after rollback failed: %v", err)
	}
	if count("passkey_credentials") != 2 || count("security_outbox") != 2 ||
		count("passkey_registration_notices") != 2 {
		t.Fatal("second legitimate transaction failed atomic cardinality")
	}

	// A concurrent replay on one nonce has ONE winner and ONE denial.
	thirdID, thirdNonce := issue()
	admitTestOnly(thirdID)
	thirdPayload := stagedPasskeyResponse(t, thirdNonce, bytes.Repeat([]byte{0x43}, 32))
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { errs <- stager.Stage(ctx, bearer, thirdID, thirdPayload) })
	}
	wg.Wait()
	close(errs)
	successes, denials := 0, 0
	for e := range errs {
		if e == nil {
			successes++
		} else if errors.Is(e, usecase.ErrPasskeyStageDenied) {
			denials++
		} else {
			t.Fatalf("concurrent one-use staging returned unexpected error: %v", e)
		}
	}
	if successes != 1 || denials != 1 ||
		count("passkey_credentials") != 3 ||
		count("security_outbox") != 3 ||
		count("passkey_registration_notices") != 3 {
		t.Fatalf("concurrent replay escaped one-shot transaction: ok=%d denied=%d", successes, denials)
	}
}
