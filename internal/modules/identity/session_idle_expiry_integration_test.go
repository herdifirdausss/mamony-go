//go:build integration

package identity

import (
	"context"
	"crypto/sha256"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestIdentityZSessionIdleExpiry proves that a PostgreSQL clock, not the
// browser's cookie Max-Age, governs 30-minute inactivity and the independent
// 12-hour absolute limit. All timestamps are synthetic fixture mutations;
// no real test waits 30 minutes and no role besides admin can modify them.
func TestIdentityZSessionIdleExpiry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	grantDSN := guardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, dsn := range []string{grantDSN, sessionDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("idle-expiry tests require a shared disposable database")
		}
	}
	open := func(dsn string) *pgx.Conn {
		t.Helper()
		c, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close(context.Background()) })
		return c
	}
	admin, grantConn, sessionConn, blocker := open(adminDSN), open(grantDSN), open(sessionDSN), open(adminDSN)
	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "idle_")) + "@example.test"
	if _, err := admin.Exec(ctx, "INSERT INTO mamony_identity.subjects (subject_id,created_at) VALUES ($1,clock_timestamp())", subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "INSERT INTO mamony_identity.email_aliases (canonical_email,subject_id,verified_at,created_at) VALUES ($1,$2,clock_timestamp(),clock_timestamp())", email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	issue := func(label string) []byte {
		t.Helper()
		grant := sha256.Sum256([]byte("test:idle:grant:" + subject + ":" + label))
		token := sha256.Sum256([]byte("test:idle:session:" + subject + ":" + label))
		var allowed bool
		if err := grantConn.QueryRow(ctx, "SELECT mamony_identity.stage_password_auth_grant($1,$2)", subject, grant[:]).Scan(&allowed); err != nil || !allowed {
			t.Fatalf("cannot stage synthetic grant: label=%s allowed=%t err=%v", label, allowed, err)
		}
		if err := sessionConn.QueryRow(ctx, "SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
			grant[:], token[:], randomSyntheticID(t, "evt_")).Scan(&allowed); err != nil || !allowed {
			t.Fatalf("cannot issue synthetic session: label=%s allowed=%t err=%v", label, allowed, err)
		}
		return token[:]
	}
	resolve := func(digest []byte) string {
		t.Helper()
		var got string
		if err := sessionConn.QueryRow(ctx,
			"SELECT coalesce(mamony_identity.resolve_opaque_session($1),'')", digest).Scan(&got); err != nil {
			t.Fatalf("session resolution unavailable: %v", err)
		}
		return got
	}
	lastSeen := func(digest []byte) time.Time {
		t.Helper()
		var stamp time.Time
		if err := admin.QueryRow(ctx,
			"SELECT last_seen_at FROM mamony_identity.opaque_sessions WHERE token_digest=$1",
			digest).Scan(&stamp); err != nil {
			t.Fatal(err)
		}
		return stamp
	}

	t.Run("new session resolves and commits activity", func(t *testing.T) {
		digest := issue("new")
		old := lastSeen(digest)
		if got := resolve(digest); got != subject {
			t.Fatal("newly minted session did not resolve")
		}
		if refreshed := lastSeen(digest); refreshed.Before(old) {
			t.Fatal("resolve moved database activity timestamp backwards")
		}
	})

	t.Run("activity inside the idle window refreshes but does not change absolute expiry", func(t *testing.T) {
		digest := issue("active")
		var absoluteBefore, absoluteAfter time.Time
		if err := admin.QueryRow(ctx,
			"SELECT expires_at FROM mamony_identity.opaque_sessions WHERE token_digest=$1", digest).Scan(&absoluteBefore); err != nil {
			t.Fatal(err)
		}
		_, err := admin.Exec(ctx, `UPDATE mamony_identity.opaque_sessions
			SET issued_at=clock_timestamp()-interval '35 minutes',
			    last_seen_at=clock_timestamp()-interval '29 minutes'
			WHERE token_digest=$1`, digest)
		if err != nil {
			t.Fatal(err)
		}
		if got := resolve(digest); got != subject {
			t.Fatal("in-window activity did not resolve")
		}
		if time.Since(lastSeen(digest)) > 30*time.Second {
			t.Fatal("valid session activity was not updated by the DB clock")
		}
		if err := admin.QueryRow(ctx,
			"SELECT expires_at FROM mamony_identity.opaque_sessions WHERE token_digest=$1", digest).Scan(&absoluteAfter); err != nil {
			t.Fatal(err)
		}
		if !absoluteBefore.Equal(absoluteAfter) {
			t.Fatal("sliding activity incorrectly extended absolute session expiry")
		}
	})

	t.Run("idle past thirty minutes remains rejected on repeated attempts", func(t *testing.T) {
		digest := issue("idle")
		_, err := admin.Exec(ctx, `UPDATE mamony_identity.opaque_sessions
			SET issued_at=clock_timestamp()-interval '40 minutes',
			    last_seen_at=clock_timestamp()-interval '31 minutes'
			WHERE token_digest=$1`, digest)
		if err != nil {
			t.Fatal(err)
		}
		before := lastSeen(digest)
		for range 2 {
			if got := resolve(digest); got != "" {
				t.Fatal("idle expired session was accepted or resurrected")
			}
		}
		if after := lastSeen(digest); !before.Equal(after) {
			t.Fatal("failed idle resolution refreshed expired session activity")
		}
	})

	t.Run("absolute expiry wins even with very recent activity", func(t *testing.T) {
		digest := issue("absolute")
		_, err := admin.Exec(ctx, `UPDATE mamony_identity.opaque_sessions
			SET issued_at=clock_timestamp()-interval '13 hours',
			    last_seen_at=clock_timestamp()-interval '2 seconds',
			    expires_at=clock_timestamp()-interval '1 second'
			WHERE token_digest=$1`, digest)
		if err != nil {
			t.Fatal(err)
		}
		if got := resolve(digest); got != "" {
			t.Fatal("active-looking session bypassed fixed absolute expiry")
		}
	})

	t.Run("resolver blocked behind expiry rechecks database clock after acquiring lock", func(t *testing.T) {
		digest := issue("blocked-resolve")
		var sessionPID, blockerPID int32
		if err := sessionConn.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&sessionPID); err != nil {
			t.Fatal(err)
		}
		if err := blocker.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&blockerPID); err != nil {
			t.Fatal(err)
		}
		tx, err := blocker.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		// Another transaction has advanced authoritative inactivity while
		// retaining the row lock. A pending Resolve must NOT use a stale
		// pre-wait timestamp to revalidate this now-expired credential.
		if _, err := tx.Exec(ctx, `UPDATE mamony_identity.opaque_sessions
			SET issued_at=clock_timestamp()-interval '40 minutes',
			    last_seen_at=clock_timestamp()-interval '31 minutes'
			WHERE token_digest=$1`, digest); err != nil {
			t.Fatal(err)
		}
		type result struct {
			subject string
			err     error
		}
		ch := make(chan result, 1)
		go func() {
			var got string
			err := sessionConn.QueryRow(ctx,
				"SELECT coalesce(mamony_identity.resolve_opaque_session($1),'')",
				digest).Scan(&got)
			ch <- result{got, err}
		}()
		waitForSessionLock(t, ctx, admin, sessionPID, blockerPID)
		select {
		case got := <-ch:
			t.Fatalf("resolver bypassed uncommitted inactivity lock: %+v", got)
		default:
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-ch:
			if got.err != nil || got.subject != "" {
				t.Fatalf("resolver admitted expired session after lock wait: subject=%q err=%v",
					got.subject, got.err)
			}
		case <-ctx.Done():
			t.Fatal("resolver did not return after lock holder committed")
		}
		if time.Since(lastSeen(digest)) < 30*time.Minute {
			t.Fatal("blocked expired resolution incorrectly refreshed last activity")
		}
	})

	t.Run("freeze then thaw does not resurrect an idle session", func(t *testing.T) {
		digest := issue("freeze")
		if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='FROZEN' WHERE subject_id=$1", subject); err != nil {
			t.Fatal(err)
		}
		if got := resolve(digest); got != "" {
			t.Fatal("frozen subject was resolved")
		}
		if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
			t.Fatal(err)
		}
		if got := resolve(digest); got != "" {
			t.Fatal("thaw revived a previous epoch")
		}
	})
}
