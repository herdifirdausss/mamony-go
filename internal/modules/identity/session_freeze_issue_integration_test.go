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

// waitForSessionLock is a deterministic synchronization barrier, not a sleep
// intended to guess when a concurrent statement has started. It observes
// PostgreSQL's actual Lock wait state using the disposable fixture superuser.
// NEVER run this helper against a real database or with application roles.
func waitForSessionLock(t *testing.T, ctx context.Context, observer *pgx.Conn, pid uint32) {
	t.Helper()
	pollCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waitType, waitEvent string
		err := observer.QueryRow(pollCtx, "SELECT coalesce(wait_event_type,''), coalesce(wait_event,'') FROM pg_catalog.pg_stat_activity WHERE pid=$1",
			int32(pid)).Scan(&waitType, &waitEvent)
		if err != nil {
			t.Fatalf("cannot observe disposable PostgreSQL lock waiter: %v", err)
		}
		if waitType == "Lock" {
			return
		}
		select {
		case <-pollCtx.Done():
			t.Fatalf("concurrent transaction never reached a genuine PostgreSQL lock wait (last wait=%s/%s): %v",
				waitType, waitEvent, pollCtx.Err())
		case <-ticker.C:
		}
	}
}

type sessionTxResult struct {
	allowed bool
	err     error
}

// TestIdentityZSessionFreezeIssueInterleavings proves both commit orders in
// the actual v10 SECURITY DEFINER issuer, with two distinct PostgreSQL
// connections and a superuser-only lock observer. No race-scheduler guesses,
// HTTP mocks, KDF shortcuts or production routes are involved.
func TestIdentityZSessionFreezeIssueInterleavings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	adminURL, _ := url.Parse(adminDSN)
	sessionURL, _ := url.Parse(sessionDSN)
	if adminURL.Host != sessionURL.Host || adminURL.Path != sessionURL.Path {
		t.Fatal("session concurrency tests must use one disposable loopback database")
	}

	observer, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = observer.Close(context.Background()) })
	freezer, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = freezer.Close(context.Background()) })
	sessionConn, err := pgx.Connect(ctx, sessionDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sessionConn.Close(context.Background()) })

	var sessionPID, freezerPID uint32
	if err := sessionConn.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&sessionPID); err != nil {
		t.Fatal(err)
	}
	if err := freezer.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&freezerPID); err != nil {
		t.Fatal(err)
	}
	if sessionPID == freezerPID {
		t.Fatal("session issue and freeze must use distinct database backends")
	}

	// The admin-only synthetic rows represent a verified ACTIVE account.
	// Tests for real proof activation and Argon2id are mandated separately
	// by PR #41. This test isolates transaction lock ordering only.
	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "freeze_")) + "@example.test"
	if _, err := observer.Exec(ctx,
		"INSERT INTO mamony_identity.subjects (subject_id,created_at) VALUES ($1,clock_timestamp())",
		subject); err != nil {
		t.Fatal(err)
	}
	if _, err := observer.Exec(ctx,
		"INSERT INTO mamony_identity.email_aliases (canonical_email,subject_id,verified_at,created_at) VALUES ($1,$2,clock_timestamp(),clock_timestamp())",
		email, subject); err != nil {
		t.Fatal(err)
	}
	if tag, err := observer.Exec(ctx,
		"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1",
		subject); err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("cannot activate isolated verified fixture: %v", err)
	}

	t.Run("freeze acquires lock first: waiting issuer is denied", func(t *testing.T) {
		freezeTx, err := freezer.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = freezeTx.Rollback(context.Background()) }()
		if tag, err := freezeTx.Exec(ctx,
			"UPDATE mamony_identity.subjects SET status='FROZEN' WHERE subject_id=$1",
			subject); err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("test freezer cannot hold subject lock: %v", err)
		}

		digest := sha256.Sum256([]byte(subject + ":freeze-first"))
		eventID := randomSyntheticID(t, "evt_")
		resultCh := make(chan sessionTxResult, 1)
		go func() {
			var allowed bool
			err := sessionConn.QueryRow(ctx,
				"SELECT mamony_identity.issue_opaque_session($1,$2,$3)",
				subject, digest[:], eventID).Scan(&allowed)
			resultCh <- sessionTxResult{allowed: allowed, err: err}
		}()
		waitForSessionLock(t, ctx, observer, sessionPID)
		select {
		case result := <-resultCh:
			t.Fatalf("issuer bypassed uncommitted freeze lock: allowed=%v err=%v", result.allowed, result.err)
		default:
		}
		if err := freezeTx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		select {
		case result := <-resultCh:
			if result.err != nil || result.allowed {
				t.Fatalf("issuer minted after freeze committed: allowed=%v err=%v", result.allowed, result.err)
			}
		case <-ctx.Done():
			t.Fatal("issuer did not finish after freeze committed")
		}

		var sessions, events int
		if err := observer.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM mamony_identity.opaque_sessions WHERE subject_id=$1),
			(SELECT count(*) FROM mamony_identity.security_outbox
			 WHERE subject_id=$1 AND event_type='SESSION_CREATED')`,
			subject).Scan(&sessions, &events); err != nil || sessions != 0 || events != 0 {
			t.Fatalf("failed issuance created session or audit: sessions=%d events=%d err=%v",
				sessions, events, err)
		}
		// Restore ACTIVE, but do not reuse a digest or event identifier.
		if tag, err := freezer.Exec(ctx,
			"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1",
			subject); err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("cannot restore synthetic account: %v", err)
		}
	})

	t.Run("issuer acquires lock first: later freeze invalidates its session", func(t *testing.T) {
		issueTx, err := sessionConn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = issueTx.Rollback(context.Background()) }()
		digest := sha256.Sum256([]byte(subject + ":issue-first"))
		eventID := randomSyntheticID(t, "evt_")
		var issued bool
		if err := issueTx.QueryRow(ctx,
			"SELECT mamony_identity.issue_opaque_session($1,$2,$3)",
			subject, digest[:], eventID).Scan(&issued); err != nil || !issued {
			t.Fatalf("cannot issue in uncommitted transaction: issued=%v err=%v", issued, err)
		}

		freezeCh := make(chan error, 1)
		go func() {
			_, err := freezer.Exec(ctx,
				"UPDATE mamony_identity.subjects SET status='FROZEN' WHERE subject_id=$1",
				subject)
			freezeCh <- err
		}()
		waitForSessionLock(t, ctx, observer, freezerPID)
		select {
		case err := <-freezeCh:
			t.Fatalf("freeze committed through uncommitted issuance lock: %v", err)
		default:
		}
		if err := issueTx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-freezeCh:
			if err != nil {
				t.Fatalf("freeze failed after issuer committed: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("freeze did not finish after issuance committed")
		}
		var oldEpoch, currentEpoch int64
		var status string
		if err := observer.QueryRow(ctx,
			`SELECT ss.subject_epoch,s.session_epoch,s.status
			FROM mamony_identity.opaque_sessions ss
			JOIN mamony_identity.subjects s ON s.subject_id=ss.subject_id
			WHERE ss.token_digest=$1`, digest[:]).Scan(&oldEpoch, &currentEpoch, &status); err != nil ||
			status != "FROZEN" || oldEpoch >= currentEpoch {
			t.Fatalf("freeze failed to invalidate issued digest: session_epoch=%d current_epoch=%d status=%s err=%v",
				oldEpoch, currentEpoch, status, err)
		}
		var resolved string
		if err := sessionConn.QueryRow(ctx,
			"SELECT coalesce(mamony_identity.resolve_opaque_session($1),'')",
			digest[:]).Scan(&resolved); err != nil || resolved != "" {
			t.Fatalf("session remained valid after later freeze: subject=%q err=%v", resolved, err)
		}
		if _, err := freezer.Exec(ctx,
			"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1",
			subject); err != nil {
			t.Fatal(err)
		}
		if err := sessionConn.QueryRow(ctx,
			"SELECT coalesce(mamony_identity.resolve_opaque_session($1),'')",
			digest[:]).Scan(&resolved); err != nil || resolved != "" {
			t.Fatal("old session was resurrected by thaw")
		}
		var sessions, events int
		if err := observer.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM mamony_identity.opaque_sessions WHERE subject_id=$1),
			(SELECT count(*) FROM mamony_identity.security_outbox WHERE subject_id=$1
			 AND event_type='SESSION_CREATED')`, subject).
			Scan(&sessions, &events); err != nil || sessions != 1 || events != 1 {
			t.Fatalf("issuer-first lost atomic digest+event: sessions=%d events=%d err=%v",
				sessions, events, err)
		}
	})

	t.Run("rollback of issuance must rollback both digest and event", func(t *testing.T) {
		issueTx, err := sessionConn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte(subject + ":rollback"))
		eventID := randomSyntheticID(t, "evt_")
		var issued bool
		if err := issueTx.QueryRow(ctx, "SELECT mamony_identity.issue_opaque_session($1,$2,$3)",
			subject, digest[:], eventID).Scan(&issued); err != nil || !issued {
			_ = issueTx.Rollback(context.Background())
			t.Fatalf("session fixture cannot issue before rollback: %v", err)
		}
		if err := issueTx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := observer.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM mamony_identity.opaque_sessions WHERE token_digest=$1)
			+
			(SELECT count(*) FROM mamony_identity.security_outbox WHERE event_id=$2)`,
			digest[:], eventID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rolled-back session or security event persisted: count=%d err=%v", count, err)
		}
	})

	// This deliberately does NOT claim to test financial transaction
	// authorization under a concurrent freeze, or represent a public login.
}
