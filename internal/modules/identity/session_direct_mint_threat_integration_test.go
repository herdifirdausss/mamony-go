//go:build integration

package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestIdentityZDirectSessionMintThreatFixture is evidence of a known v10
// weakness, not a passing security control. A holder of ONLY the session
// runtime SQL credential can mint an opaque session for an eligible subject
// without calling PasswordAuthentication, PasswordLoginFlow or Argon2id.
//
// This test intentionally passes while the vulnerability exists. When the
// boundary is redesigned, replace it with a *negative* role-capability test
// that asserts the former direct SQL entrypoint is inaccessible.
func TestIdentityZDirectSessionMintThreatFixture(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	adminURL, _ := url.Parse(adminDSN)
	sessionURL, _ := url.Parse(sessionDSN)
	if adminURL.Host != sessionURL.Host || adminURL.Path != sessionURL.Path {
		t.Fatal("threat fixture must use the same disposable loopback database")
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	session, err := pgx.Connect(ctx, sessionDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close(context.Background()) }()

	// Administrator-only synthetic fixture: ACTIVE + verified contact, but
	// deliberately NO password-verifier row and NO successful login attempt.
	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "direct_mint_")) + "@example.test"
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.subjects (subject_id,created_at) VALUES ($1,clock_timestamp())",
		subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.email_aliases (canonical_email,subject_id,verified_at,created_at) VALUES ($1,$2,clock_timestamp(),clock_timestamp())",
		email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	var verifiers int
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM mamony_identity.password_verifiers WHERE subject_id=$1",
		subject).Scan(&verifiers); err != nil || verifiers != 0 {
		t.Fatalf("threat fixture unexpectedly contains a password verifier: count=%d err=%v", verifiers, err)
	}

	// Attacker chooses a 256-bit session bearer and supplies only its digest
	// to the SQL function; this gives the attacker full knowledge of the
	// browser bearer even though the DB never stores the bearer plaintext.
	var bearer [32]byte
	if _, err := rand.Read(bearer[:]); err != nil {
		t.Fatal(err)
	}
	blob := append([]byte("mamony:identity:session-token:v1\x00"), bearer[:]...)
	digest := sha256.Sum256(blob)
	eventID := randomSyntheticID(t, "evt_")
	var issued bool
	if err := session.QueryRow(ctx,
		"SELECT mamony_identity.issue_opaque_session($1,$2,$3)",
		subject, digest[:], eventID).Scan(&issued); err != nil || !issued {
		t.Fatalf("v10 direct-mint fixture unexpectedly denied known SQL capability: allowed=%t err=%v", issued, err)
	}
	var resolved string
	if err := session.QueryRow(ctx,
		"SELECT coalesce(mamony_identity.resolve_opaque_session($1),'')",
		digest[:]).Scan(&resolved); err != nil || resolved != subject {
		t.Fatal("directly minted digest did not authenticate as target subject")
	}
	var sessions, events int
	if err := admin.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM mamony_identity.opaque_sessions WHERE token_digest=$1),
		(SELECT count(*) FROM mamony_identity.security_outbox WHERE event_id=$2 AND event_type='SESSION_CREATED')`,
		digest[:], eventID).Scan(&sessions, &events); err != nil || sessions != 1 || events != 1 {
		t.Fatalf("attack fixture did not persist session and event: sessions=%d events=%d err=%v",
			sessions, events, err)
	}

	t.Log("EXPECTED VULNERABILITY: session SQL EXECUTE alone minted an authenticated session without a password verifier or successful KDF")
}
