//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	identitywa "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/webauthn"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Stale-backup negative control; all Docker operations are limited to the
// disposable GitHub Actions PostgreSQL service and a separate new container.
func stalePasskeyDocker(t *testing.T, ctx context.Context, input []byte, args ...string) []byte {
	t.Helper()
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("disposable Docker operation %s failed: %v", args[0], err)
	}
	return out
}

func captureStalePasskeyDump(t *testing.T, ctx context.Context, dsn string, admin *pgx.Conn) []byte {
	t.Helper()
	id := os.Getenv("MAMONY_TEST_PG_SERVICE_CONTAINER_ID")
	if os.Getenv("GITHUB_ACTIONS") != "true" ||
		dsn != "postgres://mamony_test_admin:local-test-admin@127.0.0.1:5432/mamony_test_ci?sslmode=disable" ||
		!regexp.MustCompile("^[a-f0-9]{12,64}$").MatchString(id) {
		t.Fatal("stale restore requires exact disposable CI source")
	}
	info := strings.TrimSpace(string(stalePasskeyDocker(t, ctx, nil, "inspect", "--format",
		"{{.Config.Image}}|{{.State.Running}}", id)))
	if info != "postgres:18.7|true" {
		t.Fatal("unexpected source container")
	}
	env := string(stalePasskeyDocker(t, ctx, nil, "inspect", "--format",
		"{{range .Config.Env}}{{println .}}{{end}}", id))
	if !strings.Contains("\n"+env, "\nPOSTGRES_DB=mamony_test_ci\n") ||
		!strings.Contains("\n"+env, "\nPOSTGRES_USER=mamony_test_admin\n") {
		t.Fatal("source database identity mismatch")
	}
	var systemID string
	if err := admin.QueryRow(ctx, "SELECT system_identifier::text FROM pg_control_system()").Scan(&systemID); err != nil {
		t.Fatal("cannot identify source cluster")
	}
	dockerID := strings.TrimSpace(string(stalePasskeyDocker(t, ctx, nil, "exec", id, "psql", "-XAt",
		"-U", "mamony_test_admin", "-d", "mamony_test_ci", "-c", "SELECT system_identifier::text FROM pg_control_system()")))
	if systemID != dockerID {
		t.Fatal("backup container differs from the actual integration source")
	}
	dump := stalePasskeyDocker(t, ctx, nil, "exec", id, "pg_dump", "-U", "mamony_test_admin",
		"-d", "mamony_test_ci", "--format=custom")
	if len(dump) == 0 {
		t.Fatal("empty pre-finish dump")
	}
	return dump
}

// openHeldStalePasskeyTarget owns a fresh disposable restore container.
// Recreated roles must be NOLOGIN; callers never pass production backups.
func openHeldStalePasskeyTarget(t *testing.T, ctx context.Context, sourceID string, roles, dump []byte) (string, string, *pgx.Conn) {
	t.Helper()
	target := strings.TrimSpace(string(stalePasskeyDocker(t, ctx, nil, "run", "--rm", "-d",
		"-e", "POSTGRES_DB=mamony_test_stale_restore", "-e", "POSTGRES_USER=mamony_test_admin",
		"-e", "POSTGRES_PASSWORD=local-test-stale-admin", "-p", "127.0.0.1::5432", "postgres:18.7")))
	if target == sourceID || !regexp.MustCompile("^[a-f0-9]{12,64}$").MatchString(target) {
		t.Fatal("restore target is not a distinct disposable container")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := exec.CommandContext(cleanup, "docker", "rm", "-f", target).Run(); err != nil {
			t.Error("could not remove disposable restore container")
		}
	})
	host, port, err := net.SplitHostPort(strings.TrimSpace(string(stalePasskeyDocker(t, ctx, nil, "port", target, "5432/tcp"))))
	if err != nil || host != "127.0.0.1" || port == "5432" {
		t.Fatal("restore must use a separate loopback port")
	}
	targetDSN := "postgres://mamony_test_admin:local-test-stale-admin@127.0.0.1:" + port +
		"/mamony_test_stale_restore?sslmode=disable"
	deadline := time.Now().Add(30 * time.Second)
	var admin *pgx.Conn
	for {
		attempt, cancel := context.WithTimeout(ctx, time.Second)
		admin, err = pgx.Connect(attempt, targetDSN)
		cancel()
		if err == nil {
			break
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			t.Fatal("restore database did not become reachable")
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	stalePasskeyDocker(t, ctx, roles, "exec", "-i", target, "psql", "-Xq", "-v", "ON_ERROR_STOP=1",
		"-U", "mamony_test_admin", "-d", "mamony_test_stale_restore")
	stalePasskeyDocker(t, ctx, dump, "exec", "-i", target, "pg_restore", "-U", "mamony_test_admin",
		"-d", "mamony_test_stale_restore", "--single-transaction", "--exit-on-error")
	return target, port, admin
}

func checkStalePasskeyRestore(t *testing.T, ctx context.Context, source *pgx.Conn, dump []byte,
	sessionDSN, readerDSN, finishDSN, bearer, challenge string, assertion []byte, account string, credential []byte) {
	t.Helper()
	// Source commit has been verified and its original assertion denied on
	// replay. Keep the source event identifier outside the restored database.
	var sourceEvent string
	if err := source.QueryRow(ctx, "SELECT possession_event_id FROM mamony_identity.passkey_credentials "+
		"WHERE credential_id=$1 AND state='POSSESSION_VERIFIED'", credential).Scan(&sourceEvent); err != nil {
		t.Fatal("source possession event absent")
	}
	sourceID := os.Getenv("MAMONY_TEST_PG_SERVICE_CONTAINER_ID")
	roles := stalePasskeyDocker(t, ctx, nil, "exec", sourceID, "psql", "-XAt", "-v", "ON_ERROR_STOP=1",
		"-U", "mamony_test_admin", "-d", "mamony_test_ci", "-c",
		"SELECT format('CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;', rolname) "+
			"FROM pg_catalog.pg_roles WHERE rolname LIKE 'mamony_%' AND rolname <> 'mamony_test_admin' ORDER BY rolname")
	// Preserve object ownership/ACLs. Set test-only passwords for the three
	// later negative-control roles but leave all restored principals NOLOGIN.
	roles = append(roles, []byte(
		"ALTER ROLE mamony_identity_session_runtime NOLOGIN PASSWORD 'local-test-identity-session';\n"+
			"ALTER ROLE mamony_identity_passkey_activation_reader_runtime NOLOGIN PASSWORD 'local-test-passkey-activation-reader';\n"+
			"ALTER ROLE mamony_identity_passkey_activation_finisher_runtime NOLOGIN PASSWORD 'local-test-passkey-activation-finisher';\n")...)
	target, port, admin := openHeldStalePasskeyTarget(t, ctx, sourceID, roles, dump)
	var state string
	var consumed, fresh bool
	var events, notices int
	checkState := func() {
		t.Helper()
		if err := admin.QueryRow(ctx,
			"SELECT c.state,a.consumed_at IS NOT NULL,clock_timestamp()<a.expires_at, "+
				"(SELECT count(*) FROM mamony_identity.security_outbox WHERE subject_id=$1 AND event_type='PASSKEY_POSSESSION_VERIFIED'), "+
				"(SELECT count(*) FROM mamony_identity.passkey_activation_notices WHERE subject_id=$1) "+
				"FROM mamony_identity.passkey_credentials c JOIN mamony_identity.passkey_activation_challenges a "+
				"ON a.subject_id=c.subject_id AND a.credential_id=c.credential_id WHERE c.subject_id=$1 AND c.credential_id=$2",
			account, credential).Scan(&state, &consumed, &fresh, &events, &notices); err != nil {
			t.Fatal("cannot inspect restored possession fixture")
		}
	}
	checkState()
	if state != "PENDING_ASSERTION" || consumed || events != 0 || notices != 0 {
		t.Fatal("pre-finish restore did not reproduce rolled-back state")
	}
	if !fresh {
		t.Fatal("stale restore negative control inconclusive: challenge expired")
	}
	// Actual restored Identity schema, not the marker-only fencing drill.
	// The source event is a surviving test oracle, not a production journal.
	var sourceEvidence, targetEvidence, held bool
	if err := source.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM mamony_identity.security_outbox o
		JOIN mamony_identity.passkey_activation_notices n USING(event_id)
		WHERE o.event_id=$1 AND o.subject_id=$2 AND n.subject_id=$2
		  AND o.event_type='PASSKEY_POSSESSION_VERIFIED')`, sourceEvent, account).Scan(&sourceEvidence); err != nil || !sourceEvidence {
		t.Fatal("surviving source event/notice evidence missing")
	}
	if err := admin.QueryRow(ctx, `SELECT
		EXISTS (SELECT 1 FROM mamony_identity.security_outbox WHERE event_id=$1)
		OR EXISTS (SELECT 1 FROM mamony_identity.passkey_activation_notices WHERE event_id=$1)`,
		sourceEvent).Scan(&targetEvidence); err != nil || targetEvidence {
		t.Fatal("expected exact source evidence absent from stale target")
	}
	if err := admin.QueryRow(ctx, `SELECT
		NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles
		 WHERE rolname LIKE 'mamony_%' AND rolname<>'mamony_test_admin' AND rolcanlogin)
		AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_stat_activity
		 WHERE datname=current_database() AND usename LIKE 'mamony_%' AND usename<>'mamony_test_admin')`).Scan(&held); err != nil || !held {
		t.Fatal("restored Identity fixture is not held")
	}
	targetRuntimeURL := func(raw, role string) string {
		t.Helper()
		u, err := url.Parse(raw)
		if err != nil || u.User == nil || u.User.Username() != role {
			t.Fatal("invalid runtime fixture")
		}
		u.Host = net.JoinHostPort("127.0.0.1", port)
		u.Path = "/mamony_test_stale_restore"
		return u.String()
	}
	for _, runtime := range []struct{ dsn, role string }{
		{sessionDSN, "mamony_identity_session_runtime"},
		{readerDSN, "mamony_identity_passkey_activation_reader_runtime"},
		{finishDSN, "mamony_identity_passkey_activation_finisher_runtime"},
	} {
		attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
		conn, err := pgx.Connect(attempt, targetRuntimeURL(runtime.dsn, runtime.role))
		cancel()
		if err == nil {
			_ = conn.Close(context.Background())
			t.Fatal("held restored runtime unexpectedly connected")
		}
		var denied *pgconn.PgError
		if !errors.As(err, &denied) || denied.Code != "28000" {
			t.Fatal("held runtime connection did not return authoritative authorization denial")
		}
	}
	checkState()
	if state != "PENDING_ASSERTION" || consumed || !fresh || events != 0 || notices != 0 {
		t.Fatal("held restore changed stale fixture or challenge expired")
	}
	t.Log("IRF-RESTORE HOLD MECHANISM PASS: actual stale Identity target denies three runtime logins; surviving event/notice absent; no release authorized")
	recordPath, firstBinding := checkHeldRestoreReviewRecord(t, ctx, source, admin, target, dump, sourceEvent, held, targetEvidence)

	// Deliberately remove the lab hold ONLY to run the existing exposure
	// negative control below. This is not a recovery release transition.
	if _, err := admin.Exec(ctx, "ALTER ROLE mamony_identity_session_runtime LOGIN"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "ALTER ROLE mamony_identity_passkey_activation_reader_runtime LOGIN"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "ALTER ROLE mamony_identity_passkey_activation_finisher_runtime LOGIN"); err != nil {
		t.Fatal(err)
	}
	open := func(raw, role string) *platformpg.Pool {
		t.Helper()
		u, err := url.Parse(raw)
		if err != nil || u.User == nil || u.User.Username() != role {
			t.Fatal("invalid runtime fixture")
		}
		u.Host = net.JoinHostPort("127.0.0.1", port)
		u.Path = "/mamony_test_stale_restore"
		p, err := platformpg.Open(ctx, platformpg.Config{DSN: u.String(), RuntimeRole: role, MaxConns: 1})
		if err != nil {
			t.Fatal("restored runtime privilege check failed")
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
	policy, err := usecase.NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	if err != nil {
		t.Fatal(err)
	}
	readerStore, err := identitypg.NewPasskeyActivationEvidenceStore(open(readerDSN, "mamony_identity_passkey_activation_reader_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := usecase.NewPasskeyActivationEvidenceReader(sessions, readerStore, policy)
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
	store, err := identitypg.NewPasskeyActivationFinishStore(open(finishDSN, "mamony_identity_passkey_activation_finisher_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	committer, err := usecase.NewPasskeyActivationCommitter(proof, store)
	if err != nil {
		t.Fatal(err)
	}
	// Reuse exactly the old signed assertion through real restricted roles.
	// Acceptance is an exposure demonstration, not a security-control pass.
	if err := committer.Finish(ctx, bearer, challenge, assertion); err != nil {
		t.Fatalf("expected stale-backup replay exposure not reproduced: %v", err)
	}
	checkState()
	var targetEvent, targetChallenge string
	if err := admin.QueryRow(ctx, "SELECT possession_event_id,possession_challenge_id FROM mamony_identity.passkey_credentials "+
		"WHERE credential_id=$1", credential).Scan(&targetEvent, &targetChallenge); err != nil ||
		targetEvent == sourceEvent || targetChallenge != challenge || state != "POSSESSION_VERIFIED" ||
		!consumed || events != 1 || notices != 1 {
		t.Fatal("stale replay did not atomically commit a distinct possession event")
	}
	t.Log("PKA-R01 NEGATIVE CONTROL: previously consumed signed assertion accepted on stale restore; original audit/notice absent, distinct event committed; recovery protection NOT IMPLEMENTED")

	// The second candidate uses the same captured backup and role DDL,
	// but never enables its runtime logins, even for the exposure control.
	repeat, repeatPort, repeatAdmin := openHeldStalePasskeyTarget(t, ctx, sourceID, roles, dump)
	if repeat == target || repeatPort == port {
		t.Fatal("repeated restore did not create a distinct target")
	}
	checkRepeatedRestoreReviewRecord(t, ctx, repeatAdmin, repeat, repeatPort, dump,
		recordPath, firstBinding, sessionDSN, readerDSN, finishDSN)
}
