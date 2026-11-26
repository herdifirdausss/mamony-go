//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/integration/internal/fixture"
	"github.com/jackc/pgx/v5"
)

// TestIdentityZAuthenticationGrants exercises the disposable v11 SQL authority.
// Direct grant staging here intentionally models privileged server assertion,
// NOT a password comparison. The full Argon2id chain has separate tests.
func TestIdentityZAuthenticationGrants(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminDSN := fixture.GuardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	grantDSN := fixture.GuardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := fixture.GuardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, dsn := range []string{grantDSN, sessionDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("grant test must use one disposable loopback database")
		}
	}
	connect := func(dsn string) *pgx.Conn {
		t.Helper()
		c, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close(context.Background()) })
		return c
	}
	admin := connect(adminDSN)
	granter := connect(grantDSN)
	sessionA := connect(sessionDSN)
	sessionB := connect(sessionDSN)

	subject := fixture.RandomID(t, "sub_")
	email := strings.ToLower(fixture.RandomID(t, "auth_grant_")) + "@example.test"
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
	grant := func(label string) []byte {
		t.Helper()
		d := sha256.Sum256([]byte("synthetic:auth:grant:" + subject + ":" + label))
		var epoch int64
		if err := admin.QueryRow(ctx, "SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", subject).Scan(&epoch); err != nil {
			t.Fatal(err)
		}
		var allowed bool
		err := granter.QueryRow(ctx, "SELECT mamony_identity.stage_password_auth_grant($1,$2,$3)",
			subject, epoch, d[:]).Scan(&allowed)
		if err != nil || !allowed {
			t.Fatalf("test grant stage failed (%s): allowed=%t err=%v", label, allowed, err)
		}
		return d[:]
	}
	mint := func(conn *pgx.Conn, g []byte, label string) bool {
		t.Helper()
		token := sha256.Sum256([]byte("synthetic:session:" + subject + ":" + label))
		var allowed bool
		err := conn.QueryRow(ctx,
			"SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
			g, token[:], fixture.RandomID(t, "evt_")).Scan(&allowed)
		if err != nil {
			t.Fatalf("grant consume failed (%s): %v", label, err)
		}
		return allowed
	}
	counts := func() (int, int, int) {
		t.Helper()
		var sessions, events, used int
		err := admin.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM mamony_identity.opaque_sessions WHERE subject_id=$1),
			(SELECT count(*) FROM mamony_identity.security_outbox WHERE subject_id=$1 AND event_type='SESSION_CREATED'),
			(SELECT count(*) FROM mamony_identity.password_login_grants WHERE subject_id=$1 AND consumed_at IS NOT NULL)`,
			subject).Scan(&sessions, &events, &used)
		if err != nil {
			t.Fatal(err)
		}
		return sessions, events, used
	}

	g1 := grant("one-use")
	if !mint(sessionA, g1, "first") || mint(sessionB, g1, "replay") {
		t.Fatal("grant was not single-use")
	}
	if sessions, events, used := counts(); sessions != 1 || events != 1 || used != 1 {
		t.Fatalf("grant/session/event not atomic: %d %d %d", sessions, events, used)
	}
	bogus := sha256.Sum256([]byte("never issued by grant authority"))
	if mint(sessionA, bogus[:], "bogus") {
		t.Fatal("unissued grant minted session")
	}
	expired := grant("expired")
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_login_grants
		SET issued_at=clock_timestamp()-interval '2 minutes',
		    expires_at=clock_timestamp()-interval '1 minute'
		WHERE grant_digest=$1`, expired); err != nil {
		t.Fatal(err)
	}
	if mint(sessionA, expired, "expired") {
		t.Fatal("expired grant minted session")
	}
	stale := grant("stale-epoch")
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='FROZEN' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	if mint(sessionA, stale, "stale-epoch") {
		t.Fatal("grant resurrected after freeze/thaw")
	}

	rollback := grant("rollback")
	tx, err := sessionA.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	token := sha256.Sum256([]byte("synthetic:session:rollback:" + subject))
	event := fixture.RandomID(t, "evt_")
	var allowed bool
	err = tx.QueryRow(ctx,
		"SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
		rollback, token[:], event).Scan(&allowed)
	if err != nil || !allowed {
		_ = tx.Rollback(context.Background())
		t.Fatalf("cannot issue inside rollback fixture: allowed=%t err=%v", allowed, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if sessions, events, used := counts(); sessions != 1 || events != 1 || used != 1 {
		t.Fatalf("rolled back grant/session/event persisted: %d %d %d", sessions, events, used)
	}
	if !mint(sessionA, rollback, "after-rollback") {
		t.Fatal("rollback did not release grant for safe explicit recovery")
	}

	concurrent := grant("concurrent")
	start := make(chan struct{})
	type result struct {
		allowed bool
		err     error
	}
	out := make(chan result, 2)
	for i, conn := range []*pgx.Conn{sessionA, sessionB} {
		token := sha256.Sum256([]byte("synthetic:concurrent:" + subject + ":" + string(rune('0'+i))))
		event := fixture.RandomID(t, "evt_")
		go func(c *pgx.Conn, digest [32]byte, eventID string) {
			<-start
			var ok bool
			err := c.QueryRow(ctx,
				"SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
				concurrent, digest[:], eventID).Scan(&ok)
			out <- result{ok, err}
		}(conn, token, event)
	}
	close(start)
	winners := 0
	for range 2 {
		r := <-out
		if r.err != nil {
			t.Fatalf("concurrent grant use unavailable: %v", r.err)
		}
		if r.allowed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("one grant minted %d concurrent sessions", winners)
	}
	if sessions, events, used := counts(); sessions != 3 || events != 3 || used != 3 {
		t.Fatalf("concurrent/rollback evidence mismatch: %d %d %d", sessions, events, used)
	}
}
