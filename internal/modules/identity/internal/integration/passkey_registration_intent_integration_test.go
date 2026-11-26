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
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityZPasskeyRegistrationIntent runs against real disposable
// PostgreSQL and a purpose-specific runtime role. A privileged test fixture
// seeds a synthetic subject/session; production has NO self-enrollment.
func TestIdentityZPasskeyRegistrationIntent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminDSN := fixture.GuardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	sessionDSN := fixture.GuardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	issuerDSN := fixture.GuardedDB(t, "TEST_IDENTITY_PASSKEY_REGISTRATION_ISSUER_URL", "mamony_identity_passkey_registration_issuer_runtime")
	readerDSN := fixture.GuardedDB(t, "TEST_IDENTITY_PASSKEY_REGISTRATION_READER_URL", "mamony_identity_passkey_registration_reader_runtime")
	base, _ := url.Parse(adminDSN)
	for _, raw := range []string{sessionDSN, issuerDSN, readerDSN} {
		u, _ := url.Parse(raw)
		if u.Host != base.Host || u.Path != base.Path {
			t.Fatal("passkey challenge test must use one disposable database")
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
	sessionStore, err := identitypg.NewOpaqueSessionStore(open(sessionDSN, "mamony_identity_session_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := usecase.NewOpaqueSessionAuthority(sessionStore)
	if err != nil {
		t.Fatal(err)
	}
	intentStore, err := identitypg.NewPasskeyRegistrationIntentStore(
		open(issuerDSN, "mamony_identity_passkey_registration_issuer_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	policy, _ := usecase.NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	issuer, err := usecase.NewPasskeyRegistrationIntentIssuer(sessions, intentStore, policy)
	if err != nil {
		t.Fatal(err)
	}
	readStore, err := identitypg.NewPasskeyRegistrationEvidenceStore(
		open(readerDSN, "mamony_identity_passkey_registration_reader_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := usecase.NewPasskeyRegistrationEvidenceReader(sessions, readStore, policy)
	if err != nil {
		t.Fatal(err)
	}
	subject := fixture.RandomID(t, "sub_")
	email := strings.ToLower(fixture.RandomID(t, "pk_")) + "@example.test"
	if _, err := admin.Exec(ctx, "INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES($1,clock_timestamp())", subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.email_aliases
		(canonical_email,subject_id,verified_at,created_at)
		VALUES($1,$2,clock_timestamp(),clock_timestamp())`, email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	var epoch int64
	if err := admin.QueryRow(ctx, "SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", subject).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	raw := bytes.Repeat([]byte{0x5d}, 32)
	bearer := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256(append([]byte("mamony:identity:session-token:v1\x00"), raw...))
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.opaque_sessions
		(token_digest,subject_id,subject_epoch,issued_at,expires_at)
		VALUES($1,$2,$3,clock_timestamp(),clock_timestamp()+interval '12 hours')`,
		digest[:], subject, epoch); err != nil {
		t.Fatal(err)
	}
	intent, err := issuer.Begin(ctx, bearer)
	if err != nil || len(intent.Challenge()) != 32 || len(intent.IntentID()) != 26 {
		t.Fatalf("session-bound registration challenge not staged: %v", err)
	}
	current, err := reader.Read(ctx, bearer, intent.IntentID())
	if err != nil || current.Subject != subject || current.Epoch != epoch ||
		!bytes.Equal(current.Nonce[:], intent.Challenge()) ||
		current.RPID != policy.RPID() || current.Origin != policy.Origin() ||
		current.PolicyGeneration != 1 {
		t.Fatalf("reader failed same-session current evidence: %v", err)
	}
	if wrong, err := reader.Read(ctx, bearer, "wrg_BBBBBBBBBBBBBBBBBBBBBB"); !errors.Is(err, usecase.ErrPasskeyRegistrationDenied) || wrong.Subject != "" {
		t.Fatal("unrelated intent exposed registration evidence")
	}
	if wrong, err := reader.Read(ctx, base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x2a}, 32)), intent.IntentID()); !errors.Is(err, usecase.ErrPasskeyRegistrationDenied) || wrong.Subject != "" {
		t.Fatal("session mismatch exposed registration evidence")
	}
	// A SECOND VALID session for the SAME subject must not inherit the
	// intent. This rules out a subject-only check instead of exact-session
	// cryptographic binding.
	otherRaw := bytes.Repeat([]byte{0x6e}, 32)
	otherBearer := base64.RawURLEncoding.EncodeToString(otherRaw)
	otherDigest := sha256.Sum256(append([]byte("mamony:identity:session-token:v1\x00"), otherRaw...))
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.opaque_sessions
		(token_digest,subject_id,subject_epoch,issued_at,expires_at)
		VALUES($1,$2,$3,clock_timestamp(),clock_timestamp()+interval '12 hours')`,
		otherDigest[:], subject, epoch); err != nil {
		t.Fatal(err)
	}
	if other, err := reader.Read(ctx, otherBearer, intent.IntentID()); !errors.Is(err, usecase.ErrPasskeyRegistrationDenied) || other.Subject != "" {
		t.Fatal("another live session for SAME subject read original registration nonce")
	}
	wrongRP, err := usecase.NewPasskeyRelyingParty("wrong.example.test", "https://wrong.example.test")
	if err != nil {
		t.Fatal(err)
	}
	wrongReader, err := usecase.NewPasskeyRegistrationEvidenceReader(sessions, readStore, wrongRP)
	if err != nil {
		t.Fatal(err)
	}
	if wrong, err := wrongReader.Read(ctx, bearer, intent.IntentID()); !errors.Is(err, usecase.ErrPasskeyRegistrationDenied) || wrong.Subject != "" {
		t.Fatal("stale/different trusted RP configuration accepted stored challenge")
	}
	var count int
	var foundID, rpID, origin, purpose string
	var generation int
	var storedEpoch int64
	var storedSession, storedChallenge []byte
	if err := admin.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM mamony_identity.passkey_registration_intents WHERE subject_id=$1),
		intent_id,rp_id,origin,purpose,policy_generation,subject_epoch,session_digest,challenge
		FROM mamony_identity.passkey_registration_intents WHERE subject_id=$1`, subject).
		Scan(&count, &foundID, &rpID, &origin, &purpose, &generation, &storedEpoch, &storedSession, &storedChallenge); err != nil {
		t.Fatal(err)
	}
	if count != 1 || foundID != intent.IntentID() || rpID != policy.RPID() ||
		origin != policy.Origin() || purpose != "PASSKEY_REGISTRATION" ||
		generation != 1 || storedEpoch != epoch ||
		!bytes.Equal(storedSession, digest[:]) || !bytes.Equal(storedChallenge, intent.Challenge()) {
		t.Fatal("registration nonce not durably bound to subject, live session, epoch and pinned RP")
	}
	// Neither same-session replay nor any second challenge may replace an
	// unexpired registration intent, including concurrently.
	var wg sync.WaitGroup
	success := make(chan error, 4)
	for k := 0; k < 4; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, issueErr := issuer.Begin(ctx, bearer)
			success <- issueErr
		}()
	}
	wg.Wait()
	close(success)
	for issueErr := range success {
		if !errors.Is(issueErr, usecase.ErrPasskeyRegistrationDenied) {
			t.Fatalf("concurrent issue replaced live intent or failed unexpectedly: %v", issueErr)
		}
	}
	// An invalid token cannot supply an arbitrary subject or identity.
	if bad, err := issuer.Begin(ctx, base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x2a}, 32))); !errors.Is(err, usecase.ErrPasskeyRegistrationDenied) || bad.IntentID() != "" {
		t.Fatal("unrecognized session minted an intent")
	}
	// A stale/expired intent may be superseded; the previous challenge is
	// no longer current even when its original nonce was genuinely random.
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.passkey_registration_intents
		SET expires_at=issued_at+interval '1 millisecond' WHERE subject_id=$1`, subject); err != nil {
		t.Fatal(err)
	}
	if expired, err := reader.Read(ctx, bearer, intent.IntentID()); !errors.Is(err, usecase.ErrPasskeyRegistrationDenied) || expired.Subject != "" {
		t.Fatal("expired challenge remained readable")
	}
	second, err := issuer.Begin(ctx, bearer)
	if err != nil || second.IntentID() == intent.IntentID() || bytes.Equal(second.Challenge(), intent.Challenge()) {
		t.Fatalf("expired pending intent was not safely replaced: %v", err)
	}
	if old, err := reader.Read(ctx, bearer, intent.IntentID()); !errors.Is(err, usecase.ErrPasskeyRegistrationDenied) || old.Subject != "" {
		t.Fatal("superseded challenge returned previous attestation context")
	}
	if latest, err := reader.Read(ctx, bearer, second.IntentID()); err != nil || latest.Subject != subject || !bytes.Equal(latest.Nonce[:], second.Challenge()) {
		t.Fatalf("newly issued nonce unavailable through isolated reader: %v", err)
	}
	// Epoch advancement invalidates even an otherwise live prior session.
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET session_epoch=session_epoch+1 WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	if stale, err := issuer.Begin(ctx, bearer); !errors.Is(err, usecase.ErrPasskeyRegistrationDenied) || stale.IntentID() != "" {
		t.Fatal("stale session epoch minted registration intent")
	}
	if stale, err := reader.Read(ctx, bearer, second.IntentID()); !errors.Is(err, usecase.ErrPasskeyRegistrationDenied) || stale.Subject != "" {
		t.Fatal("stale session epoch exposed registration challenge")
	}
	// No WebAuthn credential, independent factor approval, or audit of a
	// successful registration can be issued by this code path.
	var approvals int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.password_recovery_code_enrollment_approvals
		WHERE subject_id=$1`, subject).Scan(&approvals); err != nil || approvals != 0 {
		t.Fatal("unverified WebAuthn intent minted recovery enrollment approval")
	}
}
