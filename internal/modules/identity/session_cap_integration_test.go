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

// TestIdentityZMaxActiveSessions uses only disposable PostgreSQL and synthetic
// privileged grant fixtures. It does NOT assert the password was verified.
// A full Argon2id -> grant -> browser session flow remains a separate gate.
func TestIdentityZMaxActiveSessions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	grantDSN := guardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, dsn := range []string{grantDSN, sessionDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("session cap requires one disposable loopback database")
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
	admin, granter, sessionA, sessionB, blocker := open(adminDSN), open(grantDSN), open(sessionDSN), open(sessionDSN), open(adminDSN)
	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "cap_")) + "@example.test"
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
	stage := func(label string) []byte {
		t.Helper()
		proof := sha256.Sum256([]byte("synthetic:cap:grant:" + subject + ":" + label))
		var epoch int64
		if err := admin.QueryRow(ctx, "SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", subject).Scan(&epoch); err != nil {
			t.Fatal(err)
		}
		var allowed bool
		if err := granter.QueryRow(ctx,
			"SELECT mamony_identity.stage_password_auth_grant($1,$2,$3)", subject, epoch, proof[:]).Scan(&allowed); err != nil || !allowed {
			t.Fatalf("could not stage grant %s: allowed=%t err=%v", label, allowed, err)
		}
		return proof[:]
	}
	token := func(label string) []byte {
		t.Helper()
		digest := sha256.Sum256([]byte("synthetic:cap:token:" + subject + ":" + label))
		return digest[:]
	}
	mint := func(c *pgx.Conn, proof []byte, label string) (bool, error) {
		var allowed bool
		err := c.QueryRow(ctx,
			"SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
			proof, token(label), randomSyntheticID(t, "evt_")).Scan(&allowed)
		return allowed, err
	}
	issue := func(label string) []byte {
		t.Helper()
		proof := stage(label)
		ok, err := mint(sessionA, proof, label)
		if err != nil || !ok {
			t.Fatalf("mint failed %s: ok=%t err=%v", label, ok, err)
		}
		return token(label)
	}
	live := func() int {
		t.Helper()
		var count int
		err := admin.QueryRow(ctx, `SELECT count(*)
			FROM mamony_identity.opaque_sessions ss
			JOIN mamony_identity.subjects s ON s.subject_id=ss.subject_id
			WHERE ss.subject_id=$1 AND ss.subject_epoch=s.session_epoch
			  AND ss.revoked_at IS NULL AND ss.expires_at > clock_timestamp()
			  AND ss.last_seen_at <= clock_timestamp()
			  AND ss.last_seen_at + interval '30 minutes' > clock_timestamp()`,
			subject).Scan(&count)
		if err != nil {
			t.Fatal(err)
		}
		return count
	}
	audit := func(event string) int {
		t.Helper()
		var count int
		err := admin.QueryRow(ctx,
			"SELECT count(*) FROM mamony_identity.security_outbox WHERE subject_id=$1 AND event_type=$2",
			subject, event).Scan(&count)
		if err != nil {
			t.Fatal(err)
		}
		return count
	}
	resolve := func(digest []byte) string {
		t.Helper()
		var subjectID string
		if err := sessionA.QueryRow(ctx,
			"SELECT coalesce(mamony_identity.resolve_opaque_session($1),'')", digest).Scan(&subjectID); err != nil {
			t.Fatal(err)
		}
		return subjectID
	}
	tokens := make([][]byte, 0, 8)
	for _, label := range []string{"one", "two", "three", "four", "five"} {
		tokens = append(tokens, issue(label))
	}
	if got := live(); got != 5 || audit("SESSION_CREATED") != 5 || audit("SESSION_REVOKED") != 0 {
		t.Fatalf("initial five sessions: live=%d created=%d revoked=%d", got, audit("SESSION_CREATED"), audit("SESSION_REVOKED"))
	}
	if got := resolve(tokens[0]); got != subject {
		t.Fatal("oldest session unexpectedly failed before sixth login")
	}
	tokens = append(tokens, issue("six"))
	if got := live(); got != 5 {
		t.Fatalf("sixth login exceeded cap: live=%d", got)
	}
	if got := resolve(tokens[0]); got != "" {
		t.Fatal("oldest session not invalidated by sixth login")
	}
	if got := resolve(tokens[5]); got != subject {
		t.Fatal("newly created session was rejected")
	}
	if created, revoked := audit("SESSION_CREATED"), audit("SESSION_REVOKED"); created != 6 || revoked != 1 {
		t.Fatalf("eviction audit not atomic: created=%d revoked=%d", created, revoked)
	}
	var recorded int
	err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.security_outbox
		WHERE subject_id=$1 AND event_type='SESSION_REVOKED'
		  AND event_id ~ '^evt_[A-Za-z0-9_-]{22}$'`, subject).Scan(&recorded)
	if err != nil || recorded != 1 {
		t.Fatalf("eviction event missing a distinct bounded identifier: count=%d err=%v", recorded, err)
	}

	// Session counts are based on actual eligibility; an idle or absolutely
	// expired session should not consume a live slot or cause needless evict.
	_, err = admin.Exec(ctx, `UPDATE mamony_identity.opaque_sessions
		SET issued_at=clock_timestamp()-interval '40 minutes',
		    last_seen_at=clock_timestamp()-interval '31 minutes'
		WHERE token_digest=$1`, tokens[1])
	if err != nil {
		t.Fatal(err)
	}
	if got := live(); got != 4 {
		t.Fatalf("idle session incorrectly counted: %d", got)
	}
	_ = issue("after-idle")
	if got := live(); got != 5 || audit("SESSION_REVOKED") != 1 {
		t.Fatalf("idle was incorrectly evicted or sixth counted: live=%d revoked=%d", got, audit("SESSION_REVOKED"))
	}

	// Rollback must restore the previously-active victim, the unconsumed
	// proof, and both security outbox records as one unit.
	beforeCreated, beforeRevoked := audit("SESSION_CREATED"), audit("SESSION_REVOKED")
	rollbackGrant := stage("rollback")
	tx, err := sessionB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rollbackToken := token("rollback")
	var rollbackAllowed bool
	err = tx.QueryRow(ctx,
		"SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
		rollbackGrant, rollbackToken, randomSyntheticID(t, "evt_")).Scan(&rollbackAllowed)
	if err != nil || !rollbackAllowed {
		_ = tx.Rollback(context.Background())
		t.Fatalf("cap rollback setup unavailable: allowed=%t err=%v", rollbackAllowed, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if got := live(); got != 5 || audit("SESSION_CREATED") != beforeCreated || audit("SESSION_REVOKED") != beforeRevoked || resolve(rollbackToken) != "" {
		t.Fatalf("rolled-back cap changed persistent state: live=%d created=%d revoked=%d",
			got, audit("SESSION_CREATED"), audit("SESSION_REVOKED"))
	}
	var unconsumed bool
	if err := admin.QueryRow(ctx,
		"SELECT consumed_at IS NULL FROM mamony_identity.password_login_grants WHERE grant_digest=$1",
		rollbackGrant).Scan(&unconsumed); err != nil || !unconsumed {
		t.Fatalf("rollback consumed a grant: unconsumed=%t err=%v", unconsumed, err)
	}

	// Distinct simultaneous grants may both succeed, but every issuance
	// serializes against the same account row; no sixth active slot appears.
	proofA := stage("parallel-a")
	proofB := stage("parallel-b")
	var blockerPID, pidA, pidB int32
	if err := blocker.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&blockerPID); err != nil {
		t.Fatal(err)
	}
	if err := sessionA.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pidA); err != nil {
		t.Fatal(err)
	}
	if err := sessionB.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pidB); err != nil {
		t.Fatal(err)
	}
	block, err := blocker.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = block.Rollback(context.Background()) }()
	if _, err := block.Exec(ctx, "SELECT 1 FROM mamony_identity.subjects WHERE subject_id=$1 FOR UPDATE", subject); err != nil {
		t.Fatal(err)
	}
	type result struct {
		ok  bool
		err error
	}
	results := make(chan result, 2)
	evA := randomSyntheticID(t, "evt_")
	evB := randomSyntheticID(t, "evt_")
	go func() {
		var ok bool
		err := sessionA.QueryRow(ctx,
			"SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
			proofA, token("parallel-a"), evA).Scan(&ok)
		results <- result{ok, err}
	}()
	go func() {
		var ok bool
		err := sessionB.QueryRow(ctx,
			"SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
			proofB, token("parallel-b"), evB).Scan(&ok)
		results <- result{ok, err}
	}()
	// A waiting issuer can become the immediate pg_blocking_pids()
	// blocker of the next queued issuer. Require actual server-visible
	// Lock waits for BOTH backends, not a particular queue ordering.
	pollCtx, stopPoll := context.WithTimeout(ctx, 5*time.Second)
	defer stopPoll()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		err := admin.QueryRow(pollCtx, `SELECT count(*)
			FROM pg_catalog.pg_stat_activity a
			WHERE a.pid IN ($1,$2) AND a.wait_event_type='Lock'
			AND cardinality(pg_catalog.pg_blocking_pids(a.pid)) > 0`,
			pidA, pidB).Scan(&waiting)
		if err != nil {
			t.Fatalf("cannot observe concurrent login lock queue: %v", err)
		}
		if waiting == 2 {
			break
		}
		select {
		case <-pollCtx.Done():
			t.Fatalf("two issuing backends never both entered a PostgreSQL lock queue: %v", pollCtx.Err())
		case <-ticker.C:
		}
	}
	if err := block.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		got := <-results
		if got.err != nil || !got.ok {
			t.Fatalf("one of two concurrent grants denied incorrectly: %+v", got)
		}
	}
	if got := live(); got != 5 || audit("SESSION_CREATED") != beforeCreated+2 || audit("SESSION_REVOKED") != beforeRevoked+2 {
		t.Fatalf("concurrent issue overcommitted: live=%d created=%d revoked=%d",
			got, audit("SESSION_CREATED"), audit("SESSION_REVOKED"))
	}
	// Cross-account or epoch-freeze would need their own caller identity:
	// the cap is scoped to the grant's stored subject, never request input.
}
