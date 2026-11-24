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
	identitywa "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/webauthn"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityZPasskeyAtomicPossession exercises a REAL ES256 authenticator
// signature plus isolated PostgreSQL roles and the v36 atomic finish. This
// is a synthetic, unmounted fixture; it does not test production login.
func TestIdentityZPasskeyAtomicPossession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 155*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	regIssuerDSN := guardedDB(t, "TEST_IDENTITY_PASSKEY_REGISTRATION_ISSUER_URL", "mamony_identity_passkey_registration_issuer_runtime")
	stageDSN := guardedDB(t, "TEST_IDENTITY_PASSKEY_REGISTRATION_STAGE_URL", "mamony_identity_passkey_registration_stage_runtime")
	activationIssuerDSN := guardedDB(t, "TEST_IDENTITY_PASSKEY_ACTIVATION_ISSUER_URL", "mamony_identity_passkey_activation_issuer_runtime")
	activationReaderDSN := guardedDB(t, "TEST_IDENTITY_PASSKEY_ACTIVATION_READER_URL", "mamony_identity_passkey_activation_reader_runtime")
	finishDSN := guardedDB(t, "TEST_IDENTITY_PASSKEY_ACTIVATION_FINISHER_URL", "mamony_identity_passkey_activation_finisher_runtime")

	root, _ := url.Parse(adminDSN)
	for _, raw := range []string{sessionDSN, regIssuerDSN, stageDSN, activationIssuerDSN, activationReaderDSN, finishDSN} {
		u, _ := url.Parse(raw)
		if u.Host != root.Host || u.Path != root.Path {
			t.Fatal("activation finisher roles must share one disposable database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	open := func(dsn, role string, max int32) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: max})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	sessStore, err := identitypg.NewOpaqueSessionStore(open(sessionDSN, "mamony_identity_session_runtime", 8))
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
	regStore, err := identitypg.NewPasskeyRegistrationIntentStore(open(regIssuerDSN, "mamony_identity_passkey_registration_issuer_runtime", 3))
	if err != nil {
		t.Fatal(err)
	}
	regIssuer, err := usecase.NewPasskeyRegistrationIntentIssuer(sessions, regStore, policy)
	if err != nil {
		t.Fatal(err)
	}
	stageStore, err := identitypg.NewPasskeyRegistrationStageStore(open(stageDSN, "mamony_identity_passkey_registration_stage_runtime", 3))
	if err != nil {
		t.Fatal(err)
	}
	nonceStore, err := identitypg.NewPasskeyActivationChallengeStore(open(activationIssuerDSN, "mamony_identity_passkey_activation_issuer_runtime", 3))
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := usecase.NewPasskeyActivationChallengeIssuer(sessions, nonceStore, policy)
	if err != nil {
		t.Fatal(err)
	}
	readStore, err := identitypg.NewPasskeyActivationEvidenceStore(open(activationReaderDSN, "mamony_identity_passkey_activation_reader_runtime", 8))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := usecase.NewPasskeyActivationEvidenceReader(sessions, readStore, policy)
	if err != nil {
		t.Fatal(err)
	}
	protocol, err := identitywa.NewActivationAssertionVerifier(policy)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := usecase.NewPasskeyActivationAssertionVerifier(reader, protocol)
	if err != nil {
		t.Fatal(err)
	}
	finishStore, err := identitypg.NewPasskeyActivationFinishStore(
		open(finishDSN, "mamony_identity_passkey_activation_finisher_runtime", 8))
	if err != nil {
		t.Fatal(err)
	}
	committer, err := usecase.NewPasskeyActivationCommitter(proof, finishStore)
	if err != nil {
		t.Fatal(err)
	}
	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "activated_")) + "@example.test"
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
	if err := admin.QueryRow(ctx,
		"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
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
	signer, err := identitywa.NewSyntheticActivationSigner()
	if err != nil {
		t.Fatal(err)
	}
	publicCOSE, err := signer.PublicCOSE()
	if err != nil {
		t.Fatal(err)
	}
	credID := make([]byte, 32)
	if _, err := rand.Read(credID); err != nil {
		t.Fatal(err)
	}
	var nonce [32]byte
	copy(nonce[:], registration.Challenge())
	staged, err := stageStore.Stage(ctx, digest, registration.IntentID(),
		usecase.PasskeyRegistrationEvidence{
			Subject: subject, Epoch: epoch, Nonce: nonce,
			RPID: policy.RPID(), Origin: policy.Origin(), PolicyGeneration: 1,
		},
		usecase.PasskeyRegistrationCandidate{
			CredentialID: credID, PublicKeyCOSE: publicCOSE, AAGUID: make([]byte, 16),
			AttestationFormat: "none", AttestationType: "none",
			UserPresent: true, UserVerified: true,
		}, randomSyntheticID(t, "evt_"))
	if err != nil || !staged {
		t.Fatalf("v33 stage fixture failed: %v", err)
	}
	challenge, err := issuer.Begin(ctx, bearer, credID)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := reader.Read(ctx, bearer, challenge.ChallengeID())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := signer.SignedResponse(evidence.Nonce, credID, subject, 1)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := proof.VerifyOnly(ctx, bearer, challenge.ChallengeID(), raw)
	if err != nil || !verified.UserVerified || verified.Counter != 1 {
		t.Fatalf("real signed WebAuthn fixture failed: %v", err)
	}
	request := usecase.PasskeyActivationFinishRequest{
		SessionDigest: digest, ChallengeID: challenge.ChallengeID(),
		Expected: evidence, Verified: verified,
		EventID: randomSyntheticID(t, "evt_"),
	}
	// Different live session for same subject cannot finish original proof.
	otherBearer, otherDigest := newBearer()
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.opaque_sessions(token_digest,subject_id,subject_epoch,issued_at,expires_at) "+
			"VALUES($1,$2,$3,clock_timestamp(),clock_timestamp()+interval '12 hours')",
		otherDigest[:], subject, epoch); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name          string
		edit          func(*usecase.PasskeyActivationFinishRequest)
		adapterReject bool
	}{
		{"other session", func(x *usecase.PasskeyActivationFinishRequest) { x.SessionDigest = otherDigest }, false},
		{"wrong nonce", func(x *usecase.PasskeyActivationFinishRequest) { x.Expected.Nonce[0] ^= 1 }, false},
		{"wrong RP", func(x *usecase.PasskeyActivationFinishRequest) {
			x.Expected.RPID = "different.example.test"
			x.Expected.Origin = "https://different.example.test"
		}, false},
		{"wrong COSE", func(x *usecase.PasskeyActivationFinishRequest) {
			x.Expected.PublicKeyCOSE = bytes.Repeat([]byte{0xa1}, len(publicCOSE))
		}, false},
		{"stale sign count", func(x *usecase.PasskeyActivationFinishRequest) {
			x.Expected.SignCount = 1
			x.Verified.Counter = 2
		}, false},
		{"missing UV", func(x *usecase.PasskeyActivationFinishRequest) { x.Verified.UserVerified = false }, true},
		{"reversed counter", func(x *usecase.PasskeyActivationFinishRequest) {
			x.Expected.SignCount = 2
			x.Verified.Counter = 1
		}, true},
		{"wrong backup eligibility", func(x *usecase.PasskeyActivationFinishRequest) { x.Verified.BackupEligible = true }, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			claim := request
			claim.Expected.PublicKeyCOSE = append([]byte(nil), request.Expected.PublicKeyCOSE...)
			tt.edit(&claim)
			allowed, err := finishStore.Finish(ctx, claim)
			// Precisely distinguish a well-formed stale SQL claim from a
			// malformed signed claim rejected before the privileged SQL call.
			// A generic error cannot substitute for an actual SQL denial.
			if tt.adapterReject {
				if allowed || !errors.Is(err, usecase.ErrPasskeyActivationCommitUnavailable) {
					t.Fatalf("adapter did not reject invalid signed facts: allowed=%v err=%v", allowed, err)
				}
			} else if allowed || err != nil {
				t.Fatalf("fresh SQL fence did not deny stale evidence: allowed=%v err=%v", allowed, err)
			}
		})
	}
	// The signature was verified above while the original session was
	// live. Revoking that session after verification must deny the trusted
	// SQL snapshot rather than permit a commit based on earlier evidence.
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.opaque_sessions SET revoked_at=clock_timestamp() "+
			"WHERE token_digest=$1", digest[:]); err != nil {
		t.Fatal(err)
	}
	revokedClaim := request
	revokedClaim.EventID = randomSyntheticID(t, "evt_")
	if allowed, err := finishStore.Finish(ctx, revokedClaim); err != nil || allowed {
		t.Fatalf("revoked original session finished signed assertion: allowed=%v err=%v", allowed, err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.opaque_sessions SET revoked_at=NULL "+
			"WHERE token_digest=$1", digest[:]); err != nil {
		t.Fatal(err)
	}

	// A fresh challenge must not extend the original staged credential's
	// ten-minute lifetime. This test-only owner rewrite intentionally
	// simulates a forgotten PENDING_ASSERTION row and restores it afterward.
	var originalCreated time.Time
	if err := admin.QueryRow(ctx,
		"SELECT created_at FROM mamony_identity.passkey_credentials WHERE credential_id=$1",
		credID).Scan(&originalCreated); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.passkey_credentials "+
			"SET created_at=clock_timestamp()-interval '11 minutes' "+
			"WHERE credential_id=$1", credID); err != nil {
		t.Fatal(err)
	}
	staleStageClaim := request
	staleStageClaim.EventID = randomSyntheticID(t, "evt_")
	if allowed, err := finishStore.Finish(ctx, staleStageClaim); err != nil || allowed {
		t.Fatalf("stale staged credential finished signed assertion: allowed=%v err=%v", allowed, err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.passkey_credentials SET created_at=$1 "+
			"WHERE credential_id=$2", originalCreated, credID); err != nil {
		t.Fatal(err)
	}

	// A signed claim can be valid at v35 read time yet expire while the
	// SQL finisher waits for the subject lock. The only acceptable decision
	// uses PostgreSQL clock_timestamp AFTER the wait, not tx start time.
	var originalExpires time.Time
	if err := admin.QueryRow(ctx,
		"SELECT expires_at FROM mamony_identity.passkey_activation_challenges WHERE subject_id=$1",
		subject).Scan(&originalExpires); err != nil {
		t.Fatal(err)
	}
	lockTx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var lockedSubject string
	if err := lockTx.QueryRow(ctx,
		"SELECT subject_id FROM mamony_identity.subjects WHERE subject_id=$1 FOR UPDATE",
		subject).Scan(&lockedSubject); err != nil || lockedSubject != subject {
		_ = lockTx.Rollback(context.Background())
		t.Fatalf("could not establish subject lock fixture: %v", err)
	}
	if _, err := lockTx.Exec(ctx,
		"UPDATE mamony_identity.passkey_activation_challenges "+
			"SET expires_at=clock_timestamp()+interval '250 milliseconds' "+
			"WHERE subject_id=$1", subject); err != nil {
		_ = lockTx.Rollback(context.Background())
		t.Fatal(err)
	}
	lockResult := make(chan struct {
		ok  bool
		err error
	}, 1)
	expiringClaim := request
	expiringClaim.EventID = randomSyntheticID(t, "evt_")
	go func() {
		ok, err := finishStore.Finish(ctx, expiringClaim)
		lockResult <- struct {
			ok  bool
			err error
		}{ok, err}
	}()
	// With the subject lock held, the claim must not finish prematurely.
	select {
	case got := <-lockResult:
		_ = lockTx.Rollback(context.Background())
		t.Fatalf("finisher bypassed live subject lock: allowed=%v err=%v", got.ok, got.err)
	case <-time.After(100 * time.Millisecond):
	}
	// The challenge expires during the wait; releasing the lock must not
	// let the stale signed evidence mutate state or consume the nonce.
	time.Sleep(300 * time.Millisecond)
	if err := lockTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-lockResult:
		if got.err != nil || got.ok {
			t.Fatalf("post-lock expiry was not rejected: allowed=%v err=%v", got.ok, got.err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("post-lock expiry finish did not return")
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.passkey_activation_challenges SET expires_at=$1 "+
			"WHERE subject_id=$2", originalExpires, subject); err != nil {
		t.Fatal(err)
	}

	// Inject a synthetic failure at the mandatory notice boundary. The
	// DB function inserted the audit event first, but the statement must
	// roll back every side effect including that event on trigger failure.
	const failFunction = "mamony_identity.test_reject_possession_notice"
	if _, err := admin.Exec(ctx,
		"CREATE FUNCTION "+failFunction+"() RETURNS trigger LANGUAGE plpgsql "+
			"AS $noticebody$ BEGIN RAISE EXCEPTION 'synthetic notice failure' "+
			"USING ERRCODE='23514'; END; $noticebody$"); err != nil {
		t.Fatal(err)
	}
	const failTrigger = "test_passkey_notice_failure"
	if _, err := admin.Exec(ctx,
		"CREATE TRIGGER "+failTrigger+" BEFORE INSERT ON mamony_identity.passkey_activation_notices "+
			"FOR EACH ROW EXECUTE FUNCTION "+failFunction+"()"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = admin.Exec(context.Background(),
			"DROP TRIGGER IF EXISTS "+failTrigger+" ON mamony_identity.passkey_activation_notices")
		_, _ = admin.Exec(context.Background(), "DROP FUNCTION IF EXISTS "+failFunction+"()")
	}()
	noticeFailure := request
	noticeFailure.EventID = randomSyntheticID(t, "evt_")
	if allowed, err := finishStore.Finish(ctx, noticeFailure); err == nil || allowed {
		t.Fatal("synthetic mandatory notice failure did not fail the SQL transaction")
	}
	var state string
	var consumed bool
	var failedEventCount int
	if err := admin.QueryRow(ctx,
		"SELECT state FROM mamony_identity.passkey_credentials WHERE credential_id=$1",
		credID).Scan(&state); err != nil || state != "PENDING_ASSERTION" {
		t.Fatal("mandatory notice failure changed credential state")
	}
	if err := admin.QueryRow(ctx,
		"SELECT consumed_at IS NOT NULL FROM mamony_identity.passkey_activation_challenges WHERE subject_id=$1",
		subject).Scan(&consumed); err != nil || consumed {
		t.Fatal("mandatory notice failure consumed challenge")
	}
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM mamony_identity.security_outbox WHERE event_id=$1",
		noticeFailure.EventID).Scan(&failedEventCount); err != nil || failedEventCount != 0 {
		t.Fatal("mandatory notice failure left a committed audit event")
	}
	if _, err := admin.Exec(ctx,
		"DROP TRIGGER "+failTrigger+" ON mamony_identity.passkey_activation_notices"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "DROP FUNCTION "+failFunction+"()"); err != nil {
		t.Fatal(err)
	}

	// Late event collision must roll back nonce, credential and notice.
	event := randomSyntheticID(t, "evt_")
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.security_outbox(event_id,subject_id,event_type,occurred_at) "+
			"VALUES($1,$2,'ENROLLMENT_PENDING',clock_timestamp())", event, subject); err != nil {
		t.Fatal(err)
	}
	claim := request
	claim.EventID = event
	if ok, err := finishStore.Finish(ctx, claim); err == nil || ok {
		t.Fatal("duplicate event did not abort whole passkey commit")
	}
	if err := admin.QueryRow(ctx,
		"SELECT state FROM mamony_identity.passkey_credentials WHERE credential_id=$1", credID).Scan(&state); err != nil || state != "PENDING_ASSERTION" {
		t.Fatal("event collision left a partially activated credential")
	}
	if err := admin.QueryRow(ctx,
		"SELECT consumed_at IS NOT NULL FROM mamony_identity.passkey_activation_challenges WHERE subject_id=$1",
		subject).Scan(&consumed); err != nil || consumed {
		t.Fatal("event collision consumed activation nonce")
	}
	// Pre-verify EVERY caller's ES256 proof while v34 is still readable.
	// This tests 16 distinct SQL finish attempts at the same one-use fence,
	// not 15 requests denied early by the read-only v35 evidence reader.
	const count = 16
	claims := make([]usecase.PasskeyActivationFinishRequest, count)
	for i := range claims {
		signed, err := proof.VerifyOnly(ctx, bearer, challenge.ChallengeID(), raw)
		if err != nil || signed.Counter != 1 || !signed.UserVerified {
			t.Fatalf("pre-race signed proof %d was not verified: %v", i, err)
		}
		claims[i] = request
		claims[i].Verified = signed
		claims[i].EventID = randomSyntheticID(t, "evt_")
	}
	type dbFinishResult struct {
		allowed bool
		err     error
	}
	results := make(chan dbFinishResult, count)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, claim := range claims {
		wg.Go(func() {
			<-start
			allowed, err := finishStore.Finish(ctx, claim)
			results <- dbFinishResult{allowed: allowed, err: err}
		})
	}
	close(start)
	wg.Wait()
	close(results)
	successes, denials := 0, 0
	for got := range results {
		switch {
		case got.err == nil && got.allowed:
			successes++
		case got.err == nil && !got.allowed:
			denials++
		default:
			t.Fatalf("SQL one-use race returned an unexpected error: %v", got.err)
		}
	}
	if successes != 1 || denials != count-1 {
		t.Fatalf("16 signed SQL finish claims were not one-winner: success=%d denied=%d", successes, denials)
	}
	var storedCounter int64
	var storedEvent, storedChallenge, noticeRecipient string
	if err := admin.QueryRow(ctx,
		"SELECT state,sign_count,possession_event_id,possession_challenge_id "+
			"FROM mamony_identity.passkey_credentials WHERE credential_id=$1",
		credID).Scan(&state, &storedCounter, &storedEvent, &storedChallenge); err != nil ||
		state != "POSSESSION_VERIFIED" || storedCounter != 1 ||
		storedChallenge != challenge.ChallengeID() {
		t.Fatal("atomic possession evidence not persisted")
	}
	var eventCount, noticeCount int
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM mamony_identity.security_outbox "+
			"WHERE subject_id=$1 AND event_type='PASSKEY_POSSESSION_VERIFIED'",
		subject).Scan(&eventCount); err != nil || eventCount != 1 {
		t.Fatal("activation audit obligation was not unique")
	}
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM mamony_identity.passkey_activation_notices WHERE subject_id=$1 AND event_id=$2",
		subject, storedEvent).Scan(&noticeCount); err != nil || noticeCount != 1 {
		t.Fatal("mandatory activation notice did not atomically commit")
	}
	if err := admin.QueryRow(ctx,
		"SELECT recipient FROM mamony_identity.passkey_activation_notices WHERE event_id=$1",
		storedEvent).Scan(&noticeRecipient); err != nil || noticeRecipient != email {
		t.Fatal("activation notice was not bound to verified original contact")
	}
	if err := admin.QueryRow(ctx,
		"SELECT consumed_at IS NOT NULL FROM mamony_identity.passkey_activation_challenges WHERE subject_id=$1",
		subject).Scan(&consumed); err != nil || !consumed {
		t.Fatal("original activation nonce was not consumed")
	}
	if err := committer.Finish(ctx, bearer, challenge.ChallengeID(), raw); !errors.Is(err, usecase.ErrPasskeyActivationCommitDenied) {
		t.Fatal("same signed assertion replayed after committed possession")
	}
	if _, err := issuer.Begin(ctx, otherBearer, credID); !errors.Is(err, usecase.ErrPasskeyActivationDenied) {
		t.Fatal("second session promoted an already-verified credential")
	}
}
