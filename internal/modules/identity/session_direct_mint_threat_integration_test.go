//go:build integration

package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestIdentityZDirectSessionMintThreatFixture is a NEGATIVE regression after
// forward migration v11. The previously demonstrated v10 SQL direct-mint
// capability must be gone even for a known ACTIVE/verified subject with no
// password verifier or successful login.
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
		t.Fatalf("fixture unexpectedly contains password verifier: count=%d err=%v", verifiers, err)
	}

	var attackerSecret [32]byte
	if _, err := rand.Read(attackerSecret[:]); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(append([]byte("mamony:identity:session-token:v1\x00"), attackerSecret[:]...))
	grantDigest := sha256.Sum256([]byte("attacker has no actual authenticated grant"))
	eventID := randomSyntheticID(t, "evt_")

	// The subject-selected API must not merely become a synonym/overload.
	var issued bool
	err = session.QueryRow(ctx,
		"SELECT mamony_identity.issue_opaque_session($1,$2,$3)",
		subject, digest[:], eventID).Scan(&issued)
	var pgErr *pgconn.PgError
	if err == nil || !errors.As(err, &pgErr) ||
		(pgErr.Code != "42883" && pgErr.Code != "42501") {
		t.Fatalf("old subject-selected issuer remains accessible: %v", err)
	}

	// The session role must not be able to fabricate a matching grant.
	err = session.QueryRow(ctx,
		"SELECT mamony_identity.stage_password_auth_grant($1,$2,$3)",
		subject, int64(1), grantDigest[:]).Scan(&issued)
	if err == nil || !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("session SQL role could stage arbitrary authentication: %v", err)
	}
	// Without the separate grant authority's secret, arbitrary digest minting
	// fails closed and cannot create outbox or session rows.
	if err := session.QueryRow(ctx,
		"SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
		grantDigest[:], digest[:], eventID).Scan(&issued); err != nil || issued {
		t.Fatalf("session role minted without issued grant: issued=%t err=%v", issued, err)
	}
	var sessions, events, grants int
	if err := admin.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM mamony_identity.opaque_sessions WHERE token_digest=$1),
		(SELECT count(*) FROM mamony_identity.security_outbox WHERE event_id=$2),
		(SELECT count(*) FROM mamony_identity.password_login_grants WHERE grant_digest=$3)`,
		digest[:], eventID, grantDigest[:]).Scan(&sessions, &events, &grants); err != nil ||
		sessions != 0 || events != 0 || grants != 0 {
		t.Fatalf("unauthorized SQL changed authority state: sessions=%d events=%d grants=%d err=%v",
			sessions, events, grants, err)
	}
}
