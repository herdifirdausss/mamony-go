//go:build integration

package identity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	identitywa "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/webauthn"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// syntheticPostCommitAckLoss runs the real PostgreSQL SQL transaction to
// confirmed COMMIT, then deliberately discards the success acknowledgement
// before the repository adapter sees it. This test-only transport boundary
// is not a simulated network/TCP failure; it proves the durable SQL state
// is not retried when application acknowledgement is uncertain.
type syntheticPostCommitAckLoss struct {
	delegate identitypg.Transactor
}

func (s syntheticPostCommitAckLoss) WithTx(ctx context.Context, work func(pgx.Tx) error) error {
	if err := s.delegate.WithTx(ctx, work); err != nil {
		return err
	}
	return errors.New("synthetic lost acknowledgement after definite database commit")
}

// wireCommitCutPoint is a deterministic, TEST-ONLY TCP failure point.
// Pre-COMMIT prevents even sending the request. Post-write closes
// immediately after the full COMMIT request is accepted by the local TCP
// socket but before any server response; the outcome is intentionally
// uncertain. Post-COMMIT closes after PostgreSQL confirms COMMIT.
type wireCommitCutPoint uint8

const (
	wireNoCut wireCommitCutPoint = iota
	wireBeforeCommit
	wireAfterCommit
	wireAfterCommitWrite
)

// wireCommitFaultConn reads raw PostgreSQL frames on disposable loopback
// only (TLS disabled); it cannot be used on production connections.
type wireCommitFaultConn struct {
	net.Conn
	cut              wireCommitCutPoint
	armed            atomic.Bool
	sawCommitRequest atomic.Bool
	sentCommitWrite  atomic.Bool
	sawCommitted     atomic.Bool
	pending          []byte
}

func (c *wireCommitFaultConn) Write(p []byte) (int, error) {
	if bytes.Contains(bytes.ToUpper(p), []byte("COMMIT\x00")) {
		c.sawCommitRequest.Store(true)
		if c.cut == wireBeforeCommit {
			// No COMMIT request bytes reach the server. Closing this
			// session aborts its outstanding transaction.
			_ = c.Conn.Close()
			return 0, io.ErrClosedPipe
		}
		if c.cut == wireAfterCommit {
			c.armed.Store(true)
		}
		if c.cut == wireAfterCommitWrite {
			n, err := c.Conn.Write(p)
			if n == len(p) && err == nil {
				// The local TCP stack accepted the complete COMMIT
				// request. The PostgreSQL server may or may not have
				// committed before its socket is disconnected.
				c.sentCommitWrite.Store(true)
				_ = c.Conn.Close()
			}
			return n, err
		}
	}
	return c.Conn.Write(p)
}

func (c *wireCommitFaultConn) Read(p []byte) (int, error) {
	if c.cut != wireAfterCommit || !c.armed.Load() {
		return c.Conn.Read(p)
	}
	if len(c.pending) == 0 {
		var header [5]byte
		if _, err := io.ReadFull(c.Conn, header[:]); err != nil {
			return 0, err
		}
		length := binary.BigEndian.Uint32(header[1:])
		if length < 4 || length > 1<<20 {
			_ = c.Conn.Close()
			return 0, errors.New("invalid PostgreSQL commit response frame")
		}
		body := make([]byte, int(length)-4)
		if _, err := io.ReadFull(c.Conn, body); err != nil {
			return 0, err
		}
		if header[0] == 'C' && bytes.Equal(body, []byte("COMMIT\x00")) {
			c.sawCommitted.Store(true)
			_ = c.Conn.Close()
			return 0, io.ErrUnexpectedEOF
		}
		c.pending = append(header[:], body...)
	}
	n := copy(p, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}

// wireCommitFaultTransactor uses the original restricted finisher role,
// real v36 SQL and repository adapter with a TEST-ONLY TCP dialer.
// It does not alter the platform pool, privilege grants or public routes.
type wireCommitFaultTransactor struct {
	dsn        string
	cut        wireCommitCutPoint
	wire       *wireCommitFaultConn
	backendPID int32
}

func (t *wireCommitFaultTransactor) WithTx(
	ctx context.Context, operation func(pgx.Tx) error) error {
	cfg, err := pgx.ParseConfig(t.dsn)
	if err != nil {
		return err
	}
	cfg.Fallbacks = nil
	cfg.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil || (host != "localhost" && host != "127.0.0.1") {
			return nil, errors.New("wire-fault test must use a loopback PostgreSQL endpoint")
		}
		raw, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		t.wire = &wireCommitFaultConn{Conn: raw, cut: t.cut}
		return t.wire, nil
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var principal string
	if err := conn.QueryRow(ctx, "SELECT current_user, pg_backend_pid()").
		Scan(&principal, &t.backendPID); err != nil {
		return err
	}
	if principal != "mamony_identity_passkey_activation_finisher_runtime" {
		return errors.New("wire-fault test did not connect as the isolated finisher")
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := operation(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// resolveThenGateActivationIssuer proves that the genuine v10 session
// Resolve completed BEFORE the v34 SQL issuer is released. Without this
// barrier, an admin-held subject row would block v10's FOR SHARE rather
// than the actual issuer SQL whose lock ordering we need to exercise.
type resolveThenGateActivationIssuer struct {
	delegate usecase.PasskeyActivationChallengeStore
	arrived  chan<- struct{}
	release  <-chan struct{}
}

func (s resolveThenGateActivationIssuer) Issue(ctx context.Context,
	sessionDigest [32]byte, credentialID []byte, challengeID string,
	nonce [32]byte, rpID, origin string) (bool, error) {
	select {
	case s.arrived <- struct{}{}:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	select {
	case <-s.release:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return s.delegate.Issue(ctx, sessionDigest, credentialID, challengeID,
		nonce, rpID, origin)
}

// resolveThenGateActivationReader uses the same post-Resolve barrier to
// ensure that the real v34 reader, not only the shared session resolver,
// contends for the exact subject lock held by the disposable test owner.
type resolveThenGateActivationReader struct {
	delegate usecase.PasskeyActivationEvidenceStore
	arrived  chan<- struct{}
	release  <-chan struct{}
}

func (s resolveThenGateActivationReader) Read(ctx context.Context,
	sessionDigest [32]byte, challengeID string) (usecase.PasskeyActivationEvidence, bool, error) {
	select {
	case s.arrived <- struct{}{}:
	case <-ctx.Done():
		return usecase.PasskeyActivationEvidence{}, false, ctx.Err()
	}
	select {
	case <-s.release:
	case <-ctx.Done():
		return usecase.PasskeyActivationEvidence{}, false, ctx.Err()
	}
	return s.delegate.Read(ctx, sessionDigest, challengeID)
}

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
	finisherPool := open(finishDSN, "mamony_identity_passkey_activation_finisher_runtime", 8)
	finishStore, err := identitypg.NewPasskeyActivationFinishStore(finisherPool)
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

	// An unrelated account may have a fully valid separate session and
	// the same session epoch. The SQL finisher must derive the subject from
	// the live digest and reject the original credential/nonce ownership.
	unrelatedSubject := randomSyntheticID(t, "sub_")
	unrelatedEmail := strings.ToLower(randomSyntheticID(t, "unrelated_")) + "@example.test"
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.subjects(subject_id,created_at) "+
			"VALUES($1,clock_timestamp())", unrelatedSubject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.email_aliases(canonical_email,subject_id,verified_at,created_at) "+
			"VALUES($1,$2,clock_timestamp(),clock_timestamp())",
		unrelatedEmail, unrelatedSubject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1",
		unrelatedSubject); err != nil {
		t.Fatal(err)
	}
	var unrelatedEpoch int64
	if err := admin.QueryRow(ctx,
		"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
		unrelatedSubject).Scan(&unrelatedEpoch); err != nil || unrelatedEpoch != epoch {
		t.Fatalf("unrelated account must have matching valid epoch for ownership test: %d %v",
			unrelatedEpoch, err)
	}
	_, unrelatedDigest := newBearer()
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.opaque_sessions(token_digest,subject_id,subject_epoch,issued_at,expires_at) "+
			"VALUES($1,$2,$3,clock_timestamp(),clock_timestamp()+interval '12 hours')",
		unrelatedDigest[:], unrelatedSubject, unrelatedEpoch); err != nil {
		t.Fatal(err)
	}
	crossSubjectClaim := request
	crossSubjectClaim.SessionDigest = unrelatedDigest
	crossSubjectClaim.EventID = randomSyntheticID(t, "evt_")
	if allowed, err := finishStore.Finish(ctx, crossSubjectClaim); err != nil || allowed {
		t.Fatalf("another subject's live session adopted a staged passkey: allowed=%v err=%v", allowed, err)
	}

	// A signature over the original epoch is not authorization after a
	// subject session-epoch rollover. Even a still-live v34 nonce must fail
	// the authoritative SQL recheck, independent of the earlier v35 reader.
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.subjects SET session_epoch=session_epoch+1 "+
			"WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	epochClaim := request
	epochClaim.EventID = randomSyntheticID(t, "evt_")
	if allowed, err := finishStore.Finish(ctx, epochClaim); err != nil || allowed {
		t.Fatalf("old verified signature survived epoch rotation: allowed=%v err=%v", allowed, err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.subjects SET session_epoch=$1 WHERE subject_id=$2",
		epoch, subject); err != nil {
		t.Fatal(err)
	}

	// Changing a verified contact invokes the real v14 epoch-bump trigger.
	// The test owner restores the original alias and epoch after verifying
	// the stale proof is denied; no runtime can perform this restoration.
	replacedEmail := strings.ToLower(randomSyntheticID(t, "changed_")) + "@example.test"
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.email_aliases SET canonical_email=$1 "+
			"WHERE subject_id=$2", replacedEmail, subject); err != nil {
		t.Fatal(err)
	}
	var contactChangedEpoch int64
	if err := admin.QueryRow(ctx,
		"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
		subject).Scan(&contactChangedEpoch); err != nil || contactChangedEpoch <= epoch {
		t.Fatalf("contact update did not revoke original epoch: epoch=%d err=%v", contactChangedEpoch, err)
	}
	contactClaim := request
	contactClaim.EventID = randomSyntheticID(t, "evt_")
	if allowed, err := finishStore.Finish(ctx, contactClaim); err != nil || allowed {
		t.Fatalf("old verified signature survived contact replacement: allowed=%v err=%v", allowed, err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.email_aliases SET canonical_email=$1 "+
			"WHERE subject_id=$2", email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.subjects SET session_epoch=$1 WHERE subject_id=$2",
		epoch, subject); err != nil {
		t.Fatal(err)
	}

	// v30 upserts may supersede a consumed intent; old staged credentials
	// must NEVER inherit a newer registration's admission or password proof.
	supersedingIntent := randomSyntheticID(t, "wrg_")
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.passkey_registration_intents "+
			"SET intent_id=$1 WHERE subject_id=$2", supersedingIntent, subject); err != nil {
		t.Fatal(err)
	}
	intentClaim := request
	intentClaim.EventID = randomSyntheticID(t, "evt_")
	if allowed, err := finishStore.Finish(ctx, intentClaim); err != nil || allowed {
		t.Fatalf("old signature survived replaced registration intent: allowed=%v err=%v", allowed, err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.passkey_registration_intents "+
			"SET intent_id=$1 WHERE subject_id=$2", registration.IntentID(), subject); err != nil {
		t.Fatal(err)
	}

	// Original v32 password-reauth admission must remain consumed, not
	// merely present as a row that happens to have the same intent ID.
	var originalAdmissionConsumed time.Time
	if err := admin.QueryRow(ctx,
		"SELECT consumed_at FROM mamony_identity.passkey_registration_admissions "+
			"WHERE subject_id=$1", subject).Scan(&originalAdmissionConsumed); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.passkey_registration_admissions "+
			"SET consumed_at=NULL WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	admissionClaim := request
	admissionClaim.EventID = randomSyntheticID(t, "evt_")
	if allowed, err := finishStore.Finish(ctx, admissionClaim); err != nil || allowed {
		t.Fatalf("old signature survived unconsumed password admission: allowed=%v err=%v", allowed, err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.passkey_registration_admissions "+
			"SET consumed_at=$1 WHERE subject_id=$2", originalAdmissionConsumed, subject); err != nil {
		t.Fatal(err)
	}

	// Rotation of the v34 random challenge bytes invalidates a signature
	// even if the record keeps its original public challenge ID.
	rotatedNonce := make([]byte, 32)
	if _, err := rand.Read(rotatedNonce); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.passkey_activation_challenges "+
			"SET challenge=$1 WHERE subject_id=$2", rotatedNonce, subject); err != nil {
		t.Fatal(err)
	}
	nonceClaim := request
	nonceClaim.EventID = randomSyntheticID(t, "evt_")
	if allowed, err := finishStore.Finish(ctx, nonceClaim); err != nil || allowed {
		t.Fatalf("old signature survived changed database nonce: allowed=%v err=%v", allowed, err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.passkey_activation_challenges "+
			"SET challenge=$1 WHERE subject_id=$2", evidence.Nonce[:], subject); err != nil {
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
	// First let v34 issuer/reader each finish real v10 session resolution,
	// then hold the subject row before releasing their specialized SQL
	// functions. This prevents a false-positive observation in which only
	// session resolution waits for the subject lock.
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
	activationArrived := make(chan struct{}, 2)
	activationGate := make(chan struct{})
	var activationGateOnce sync.Once
	releaseActivation := func() { activationGateOnce.Do(func() { close(activationGate) }) }
	defer releaseActivation()
	contendingIssuer, err := usecase.NewPasskeyActivationChallengeIssuer(
		sessions, resolveThenGateActivationIssuer{
			delegate: nonceStore, arrived: activationArrived, release: activationGate,
		}, policy)
	if err != nil {
		t.Fatal(err)
	}
	contendingReader, err := usecase.NewPasskeyActivationEvidenceReader(
		sessions, resolveThenGateActivationReader{
			delegate: readStore, arrived: activationArrived, release: activationGate,
		}, policy)
	if err != nil {
		t.Fatal(err)
	}
	type readResult struct {
		evidence usecase.PasskeyActivationEvidence
		err      error
	}
	readResults := make(chan readResult, 1)
	issueResults := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Go(func() {
		evidence, err := contendingReader.Read(ctx, bearer, challenge.ChallengeID())
		readResults <- readResult{evidence: evidence, err: err}
	})
	wg.Go(func() {
		_, err := contendingIssuer.Begin(ctx, bearer, credID)
		issueResults <- err
	})
	for i := 0; i < 2; i++ {
		select {
		case <-activationArrived:
		case <-ctx.Done():
			t.Fatalf("v34 contender failed to pass the session resolver: %v", ctx.Err())
		}
	}
	lockTx, err = admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lockTx.Rollback(context.Background()) }()
	var blockerPID int32
	if err := lockTx.QueryRow(ctx,
		"SELECT pg_backend_pid()").Scan(&blockerPID); err != nil {
		t.Fatal(err)
	}
	if err := lockTx.QueryRow(ctx,
		"SELECT subject_id FROM mamony_identity.subjects "+
			"WHERE subject_id=$1 FOR UPDATE", subject).Scan(&lockedSubject); err != nil ||
		lockedSubject != subject {
		t.Fatalf("cannot acquire real subject lock for three-way contention: %v", err)
	}
	observer, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observer.Close(context.Background()) }()
	type dbFinishResult struct {
		allowed bool
		err     error
	}
	results := make(chan dbFinishResult, count)
	start := make(chan struct{})
	arrived := make(chan struct{}, count)
	for _, claim := range claims {
		wg.Go(func() {
			arrived <- struct{}{}
			<-start
			allowed, err := finishStore.Finish(ctx, claim)
			results <- dbFinishResult{allowed: allowed, err: err}
		})
	}
	// Both v34 actors have already passed v10 Resolve and are gated
	// immediately before their dedicated SQL calls. All 16 finisher
	// goroutines are live before opening the start barrier. Pools stay
	// limited to 8 physical connections each.
	for i := 0; i < count; i++ {
		select {
		case <-arrived:
		case <-ctx.Done():
			t.Fatalf("could not start all three-way lock contenders: %v", ctx.Err())
		}
	}
	close(start)
	releaseActivation()
	// PostgreSQL reports both hard and soft blockers: later waiters may
	// be queued behind other waiting transactions, not directly behind the
	// admin PID. Assert all three principals have an actual server-side
	// Lock wait AND the held admin lock is a direct blocker for at least
	// one. The gated v34 ports guarantee their SQL starts at the subject.
	var finisherBlocked, readerBlocked, issuerBlocked, adminBlocking bool
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		var nf, nr, ni, direct int
		err := observer.QueryRow(ctx,
			"SELECT count(*) FILTER (WHERE usename=$2), "+
				"count(*) FILTER (WHERE usename=$3), "+
				"count(*) FILTER (WHERE usename=$4), "+
				"count(*) FILTER (WHERE $1=ANY(pg_blocking_pids(pid))) "+
				"FROM pg_stat_activity WHERE datname=current_database() "+
				"AND wait_event_type='Lock' "+
				"AND usename IN ($2,$3,$4)",
			blockerPID,
			"mamony_identity_passkey_activation_finisher_runtime",
			"mamony_identity_passkey_activation_reader_runtime",
			"mamony_identity_passkey_activation_issuer_runtime",
		).Scan(&nf, &nr, &ni, &direct)
		if err != nil {
			t.Fatalf("cannot observe PostgreSQL lock waiters: %v", err)
		}
		finisherBlocked = finisherBlocked || nf > 0
		readerBlocked = readerBlocked || nr > 0
		issuerBlocked = issuerBlocked || ni > 0
		adminBlocking = adminBlocking || direct > 0
		if finisherBlocked && readerBlocked && issuerBlocked && adminBlocking {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !finisherBlocked || !readerBlocked || !issuerBlocked || !adminBlocking {
		t.Fatalf("v34/v36 SQL roles did not all wait under the real subject lock: "+
			"finisher=%v reader=%v issuer=%v adminBlocking=%v",
			finisherBlocked, readerBlocked, issuerBlocked, adminBlocking)
	}
	if len(results) != 0 || len(readResults) != 0 || len(issueResults) != 0 {
		t.Fatal("an actor returned before the PostgreSQL row lock was released")
	}
	if err := lockTx.Commit(ctx); err != nil {
		t.Fatalf("could not release subject lock for three-way race: %v", err)
	}
	wg.Wait()
	close(results)
	seenReader := <-readResults
	if seenReader.err != nil && !errors.Is(seenReader.err, usecase.ErrPasskeyActivationDenied) {
		t.Fatalf("v34 reader encountered an unexpected lock/deadlock failure: %v", seenReader.err)
	}
	if seenReader.err == nil &&
		(seenReader.evidence.Subject != subject ||
			seenReader.evidence.Nonce != evidence.Nonce ||
			!bytes.Equal(seenReader.evidence.CredentialID, credID)) {
		t.Fatal("v34 reader observed partially altered activation evidence")
	}
	if issuedErr := <-issueResults; !errors.Is(issuedErr, usecase.ErrPasskeyActivationDenied) {
		t.Fatalf("v34 issuer raced existing nonce or finished credential incorrectly: %v", issuedErr)
	}
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

	// Distinct fully staged, real-ES256 credentials for counter and backup
	// semantics. Do not rewrite the already-verified credential to fake a
	// new ceremony: every case independently uses the original v30/v32/v33
	// provenance, fresh v34 nonce, v35 signature and v36 SQL transaction.
	for _, tc := range []struct {
		name          string
		originalCount uint32
		signedCount   uint32
		originalBE    bool
		originalBS    bool
		signedFlags   byte
		allow         bool
		loseCommitAck bool
		wireCut       wireCommitCutPoint
	}{
		{"counter_zero_to_zero", 0, 0, false, false, 0x05, true, false, wireNoCut},
		{"counter_zero_to_one", 0, 1, false, false, 0x05, true, false, wireNoCut},
		{"counter_one_to_two", 1, 2, false, false, 0x05, true, false, wireNoCut},
		{"counter_two_to_two_denied", 2, 2, false, false, 0x05, false, false, wireNoCut},
		{"counter_two_to_one_denied", 2, 1, false, false, 0x05, false, false, wireNoCut},
		{"backup_false_to_true", 0, 1, true, false, 0x1d, true, false, wireNoCut},
		{"backup_true_to_false", 0, 1, true, true, 0x0d, true, false, wireNoCut},
		{"backup_eligibility_changed_denied", 0, 1, true, false, 0x05, false, false, wireNoCut},
		{"backup_state_without_eligibility_denied", 0, 1, false, false, 0x15, false, false, wireNoCut},
		{"counter_one_to_two_definite_commit_ack_lost", 1, 2, false, false, 0x05, true, true, wireNoCut},
		{"counter_one_to_two_tcp_commit_ack_lost", 1, 2, false, false, 0x05, true, false, wireAfterCommit},
		{"counter_one_to_two_tcp_commit_request_dropped", 1, 2, false, false, 0x05, true, false, wireBeforeCommit},
		{"counter_one_to_two_tcp_commit_write_no_response", 1, 2, false, false, 0x05, true, false, wireAfterCommitWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := randomSyntheticID(t, "sub_")
			contact := strings.ToLower(randomSyntheticID(t, "counter_")) + "@example.test"
			if _, err := admin.Exec(ctx,
				"INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES($1,clock_timestamp())",
				account); err != nil {
				t.Fatal(err)
			}
			if _, err := admin.Exec(ctx,
				"INSERT INTO mamony_identity.email_aliases(canonical_email,subject_id,verified_at,created_at) "+
					"VALUES($1,$2,clock_timestamp(),clock_timestamp())", contact, account); err != nil {
				t.Fatal(err)
			}
			if _, err := admin.Exec(ctx,
				"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1",
				account); err != nil {
				t.Fatal(err)
			}
			var accountEpoch int64
			if err := admin.QueryRow(ctx,
				"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
				account).Scan(&accountEpoch); err != nil {
				t.Fatal(err)
			}
			session, sessionHash := newBearer()
			if _, err := admin.Exec(ctx,
				"INSERT INTO mamony_identity.opaque_sessions(token_digest,subject_id,subject_epoch,issued_at,expires_at) "+
					"VALUES($1,$2,$3,clock_timestamp(),clock_timestamp()+interval '12 hours')",
				sessionHash[:], account, accountEpoch); err != nil {
				t.Fatal(err)
			}
			reg, err := regIssuer.Begin(ctx, session)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := admin.Exec(ctx,
				"INSERT INTO mamony_identity.passkey_registration_admissions "+
					"(subject_id,intent_id,session_digest,subject_epoch,contact_version,provenance,approved_at,expires_at) "+
					"SELECT subject_id,intent_id,session_digest,subject_epoch,contact_version,"+
					"'PASSWORD_REAUTH_ONLY',clock_timestamp(),expires_at "+
					"FROM mamony_identity.passkey_registration_intents WHERE subject_id=$1 AND intent_id=$2",
				account, reg.IntentID()); err != nil {
				t.Fatal(err)
			}
			authenticator, err := identitywa.NewSyntheticActivationSigner()
			if err != nil {
				t.Fatal(err)
			}
			cose, err := authenticator.PublicCOSE()
			if err != nil {
				t.Fatal(err)
			}
			credential := make([]byte, 32)
			if _, err := rand.Read(credential); err != nil {
				t.Fatal(err)
			}
			var regNonce [32]byte
			copy(regNonce[:], reg.Challenge())
			ok, err := stageStore.Stage(ctx, sessionHash, reg.IntentID(),
				usecase.PasskeyRegistrationEvidence{
					Subject: account, Epoch: accountEpoch, Nonce: regNonce,
					RPID: policy.RPID(), Origin: policy.Origin(), PolicyGeneration: 1,
				},
				usecase.PasskeyRegistrationCandidate{
					CredentialID: credential, PublicKeyCOSE: cose,
					AAGUID: make([]byte, 16), AttestationFormat: "none",
					AttestationType: "none", UserPresent: true, UserVerified: true,
					BackupEligible: tc.originalBE, BackupState: tc.originalBS,
					AuthenticatorCount: tc.originalCount,
				}, randomSyntheticID(t, "evt_"))
			if err != nil || !ok {
				t.Fatalf("staging real counter/backup fixture: allowed=%v err=%v", ok, err)
			}
			ch, err := issuer.Begin(ctx, session, credential)
			if err != nil {
				t.Fatal(err)
			}
			original, err := reader.Read(ctx, session, ch.ChallengeID())
			if err != nil || original.SignCount != tc.originalCount ||
				original.BackupEligible != tc.originalBE ||
				original.BackupState != tc.originalBS {
				t.Fatalf("original signed evidence mismatch: %+v err=%v", original, err)
			}
			assertion, err := authenticator.SignedResponseWithFlags(
				original.Nonce, credential, account, tc.signedCount, tc.signedFlags)
			if err != nil {
				t.Fatal(err)
			}
			// Verify real maintained-library signature acceptance, not
			// only the SQL adapter's independently asserted flag booleans.
			checked, verifyErr := proof.VerifyOnly(ctx, session, ch.ChallengeID(), assertion)
			if tc.allow && (verifyErr != nil || checked.Counter != tc.signedCount ||
				checked.BackupEligible != tc.originalBE ||
				checked.BackupState != (tc.signedFlags&0x10 != 0)) {
				t.Fatalf("signed counter/backup assertion was not verified: %+v err=%v", checked, verifyErr)
			}
			if !tc.allow && !errors.Is(verifyErr, usecase.ErrPasskeyAssertionDenied) {
				t.Fatalf("invalid signed counter/flags were not denied: %v", verifyErr)
			}
			var finishErr error
			if tc.wireCut != wireNoCut {
				wire := &wireCommitFaultTransactor{dsn: finishDSN, cut: tc.wireCut}
				uncertainStore, err := identitypg.NewPasskeyActivationFinishStore(wire)
				if err != nil {
					t.Fatal(err)
				}
				uncertainCommitter, err := usecase.NewPasskeyActivationCommitter(proof, uncertainStore)
				if err != nil {
					t.Fatal(err)
				}
				finishErr = uncertainCommitter.Finish(ctx, session, ch.ChallengeID(), assertion)
				if !errors.Is(finishErr, usecase.ErrPasskeyActivationCommitUnavailable) ||
					wire.wire == nil || !wire.wire.sawCommitRequest.Load() ||
					wire.wire.sawCommitted.Load() != (tc.wireCut == wireAfterCommit) ||
					wire.wire.sentCommitWrite.Load() != (tc.wireCut == wireAfterCommitWrite) {
					t.Fatalf("TCP COMMIT cut not observed and sanitized at expected phase: "+
						"err=%v cut=%v request=%v sent=%v confirmed=%v",
						finishErr, tc.wireCut,
						wire.wire != nil && wire.wire.sawCommitRequest.Load(),
						wire.wire != nil && wire.wire.sentCommitWrite.Load(),
						wire.wire != nil && wire.wire.sawCommitted.Load())
				}
				if tc.wireCut == wireAfterCommitWrite {
					// Ensure the disconnected server backend is gone
					// before independently observing committed state.
					// Otherwise a slow server could make a valid commit
					// look like a temporarily uncommitted transaction.
					if wire.backendPID <= 0 {
						t.Fatal("missing real PostgreSQL backend PID")
					}
					deadline := time.Now().Add(5 * time.Second)
					for {
						var gone bool
						if err := admin.QueryRow(ctx,
							"SELECT NOT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid=$1)",
							wire.backendPID).Scan(&gone); err != nil {
							t.Fatal(err)
						}
						if gone {
							break
						}
						if time.Now().After(deadline) {
							t.Fatal("TCP-cut PostgreSQL backend did not terminate")
						}
						time.Sleep(20 * time.Millisecond)
					}
				}
			} else if tc.loseCommitAck {
				uncertainStore, err := identitypg.NewPasskeyActivationFinishStore(
					syntheticPostCommitAckLoss{delegate: finisherPool})
				if err != nil {
					t.Fatal(err)
				}
				uncertainCommitter, err := usecase.NewPasskeyActivationCommitter(proof, uncertainStore)
				if err != nil {
					t.Fatal(err)
				}
				finishErr = uncertainCommitter.Finish(ctx, session, ch.ChallengeID(), assertion)
				if !errors.Is(finishErr, usecase.ErrPasskeyActivationCommitUnavailable) {
					t.Fatalf("definite SQL commit with lost acknowledgement leaked a success: %v", finishErr)
				}
			} else {
				finishErr = committer.Finish(ctx, session, ch.ChallengeID(), assertion)
			}
			if tc.allow && !tc.loseCommitAck && tc.wireCut == wireNoCut && finishErr != nil {
				t.Fatalf("valid signed counter/backup assertion did not commit: %v", finishErr)
			}
			if !tc.allow && !errors.Is(finishErr, usecase.ErrPasskeyActivationCommitDenied) {
				t.Fatalf("invalid counter/backup assertion reached commit: %v", finishErr)
			}
			var finalState string
			var finalCount int64
			var finalBE, finalBS, consumedOnce bool
			if err := admin.QueryRow(ctx,
				"SELECT state,sign_count,backup_eligible,backup_state "+
					"FROM mamony_identity.passkey_credentials WHERE credential_id=$1",
				credential).Scan(&finalState, &finalCount, &finalBE, &finalBS); err != nil {
				t.Fatal(err)
			}
			if err := admin.QueryRow(ctx,
				"SELECT consumed_at IS NOT NULL FROM mamony_identity.passkey_activation_challenges "+
					"WHERE subject_id=$1", account).Scan(&consumedOnce); err != nil {
				t.Fatal(err)
			}
			var auditCount, noticeCount int
			if err := admin.QueryRow(ctx,
				"SELECT count(*) FROM mamony_identity.security_outbox "+
					"WHERE subject_id=$1 AND event_type='PASSKEY_POSSESSION_VERIFIED'",
				account).Scan(&auditCount); err != nil {
				t.Fatal(err)
			}
			if err := admin.QueryRow(ctx,
				"SELECT count(*) FROM mamony_identity.passkey_activation_notices WHERE subject_id=$1",
				account).Scan(&noticeCount); err != nil {
				t.Fatal(err)
			}
			// A fully sent COMMIT request with no server response is
			// deliberately ambiguous. Accept ONLY fully rolled back
			// or fully committed database states; all related rows
			// must agree, and the usecase must not auto-retry.
			committed := tc.allow && tc.wireCut != wireBeforeCommit &&
				(tc.wireCut != wireAfterCommitWrite || finalState == "POSSESSION_VERIFIED")
			if committed {
				signedBS := tc.signedFlags&0x10 != 0
				if finalState != "POSSESSION_VERIFIED" || finalCount != int64(tc.signedCount) ||
					finalBE != tc.originalBE || finalBS != signedBS || !consumedOnce ||
					auditCount != 1 || noticeCount != 1 {
					t.Fatalf("valid signed counter/backup state not atomically committed: "+
						"state=%s counter=%d BE=%v BS=%v consumed=%v events=%d notices=%d",
						finalState, finalCount, finalBE, finalBS, consumedOnce, auditCount, noticeCount)
				}
				if err := committer.Finish(ctx, session, ch.ChallengeID(), assertion); !errors.Is(err, usecase.ErrPasskeyActivationCommitDenied) {
					t.Fatalf("counter/backup signature replay was not denied: %v", err)
				}
			} else if finalState != "PENDING_ASSERTION" ||
				finalCount != int64(tc.originalCount) ||
				finalBE != tc.originalBE || finalBS != tc.originalBS ||
				consumedOnce || auditCount != 0 || noticeCount != 0 {
				t.Fatalf("invalid counter/backup assertion mutated credential/nonce/notice: "+
					"state=%s counter=%d BE=%v BS=%v consumed=%v events=%d notices=%d",
					finalState, finalCount, finalBE, finalBS, consumedOnce, auditCount, noticeCount)
			}
		})
	}
}
