//go:build integration

// Package postgrestest verifies PostgreSQL privilege and RLS primitives on a
// disposable, loopback-only database. It does not exercise production SQL.
package postgrestest

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if err := safeFixture(); err != nil {
		fmt.Fprintln(os.Stderr, "PostgreSQL fixture denied:", err)
		os.Exit(1)
	}
	if output, err := invokePSQL("TEST_DATABASE_ADMIN_URL", "-f", filepath.Join("testdata", "bootstrap.sql")); err != nil {
		fmt.Fprintf(os.Stderr, "PostgreSQL fixture setup failed: %v; output: %s\n", err, output)
		os.Exit(1)
	}
	if output, err := invokePSQL("TEST_DATABASE_ADMIN_URL", "-f", filepath.Join("testdata", "platform-bootstrap.sql")); err != nil {
		fmt.Fprintf(os.Stderr, "Platform fixture setup failed: %v; output: %s\n", err, output)
		os.Exit(1)
	}
	if output, err := invokePSQL("TEST_DATABASE_ADMIN_URL", "-f", filepath.Join("testdata", "identity-bootstrap.sql")); err != nil {
		fmt.Fprintf(os.Stderr, "Identity fixture setup failed: %v; output: %s\n", err, output)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// Recheck these restrictions inside the suite: direct 'go test -tags=integration'
// must not bypass the safety checks in tools/testgate.
func safeFixture() error {
	var dbPath, host, port string
	for _, pair := range []struct{ key, user string }{
		{"TEST_DATABASE_ADMIN_URL", "mamony_test_admin"},
		{"TEST_DATABASE_URL", "mamony_test_runtime"},
		{"TEST_PLATFORM_MIGRATION_URL", "mamony_platform_migrator"},
		{"TEST_PLATFORM_RUNTIME_URL", "mamony_platform_runtime"},
		{"TEST_IDENTITY_MIGRATION_URL", "mamony_identity_migrator"},
		{"TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime"},
		{"TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime"},
		{"TEST_IDENTITY_PASSKEY_REGISTRATION_ISSUER_URL", "mamony_identity_passkey_registration_issuer_runtime"},
		{"TEST_IDENTITY_PASSKEY_REGISTRATION_READER_URL", "mamony_identity_passkey_registration_reader_runtime"},
		{"TEST_IDENTITY_DELIVERY_URL", "mamony_identity_delivery_runtime"},
		{"TEST_IDENTITY_MAINTENANCE_URL", "mamony_identity_maintenance_runtime"},
	} {
		raw := os.Getenv(pair.key)
		u, err := url.Parse(raw)
		if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") ||
			(u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") ||
			u.User == nil || u.User.Username() != pair.user ||
			!strings.HasPrefix(u.Path, "/mamony_test_") ||
			strings.Contains(strings.TrimPrefix(u.Path, "/"), "/") ||
			len(u.Query()) != 1 || u.Query().Get("sslmode") != "disable" ||
			len(u.Query()["sslmode"]) != 1 || u.Fragment != "" {
			return fmt.Errorf("%s must be a loopback-only synthetic mamony_test_ database with expected role", pair.key)
		}
		if _, ok := u.User.Password(); !ok {
			return fmt.Errorf("%s lacks fixture password", pair.key)
		}
		n, err := strconv.Atoi(u.Port())
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("%s needs explicit valid port", pair.key)
		}
		if dbPath != "" && (dbPath != u.Path || host != u.Hostname() || port != u.Port()) {
			return fmt.Errorf("runtime and admin fixtures must use the same database")
		}
		dbPath, host, port = u.Path, u.Hostname(), u.Port()
	}
	return nil
}

func invokePSQL(urlEnv string, args ...string) (string, error) {
	// -X ignores ~/.psqlrc, -w avoids interactive prompts, ON_ERROR_STOP
	// rejects erroring SQL; verbose output exposes SQLSTATE for negative tests.
	base := []string{"-X", "-w", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-v", "VERBOSITY=verbose", "-d", os.Getenv(urlEnv)}
	cmd := exec.Command("psql", append(base, args...)...)
	// libpq environment variables (notably PGHOSTADDR and PGSERVICE) can
	// override/redirect a URL target. Ignore all inherited PG* values.
	cmd.Env = sanitizedPSQLEnv(os.Environ())
	b, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(b)), err
}

func query(runtimeSQL string) (string, error) {
	return invokePSQL("TEST_DATABASE_URL", "-c", runtimeSQL)
}

func requireSQL(t *testing.T, sql, want string) {
	t.Helper()
	got, err := query(sql)
	if err != nil {
		t.Fatalf("runtime SQL failed: %v; output: %s", err, got)
	}
	if got != want {
		t.Fatalf("unexpected query result: got %q, want %q", got, want)
	}
}

func denySQL(t *testing.T, sql, sqlstate string) {
	t.Helper()
	out, err := query(sql)
	if err == nil || !strings.Contains(out, sqlstate) {
		t.Fatalf("expected SQLSTATE %s, got err=%v, output=%q", sqlstate, err, out)
	}
}

func TestPostgresRoleSeparation(t *testing.T) {
	requireSQL(t, `SELECT (current_user = 'mamony_test_runtime'
	AND NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole
	AND NOT rolbypassrls AND NOT rolreplication
	AND has_schema_privilege(current_user, 'mamony_test_fixture', 'USAGE')
	AND NOT has_schema_privilege(current_user, 'mamony_test_fixture', 'CREATE')
	AND NOT pg_has_role(current_user, 'mamony_test_owner', 'MEMBER'))::text
	FROM pg_roles WHERE rolname = current_user`, "true")
	denySQL(t, "CREATE TABLE mamony_test_fixture.unexpected(id int)", "42501")
	requireSQL(t, "SELECT COUNT(*) FROM pg_tables WHERE schemaname = 'mamony_test_fixture' AND tablename = 'unexpected'", "0")
}

func TestPostgresTenantIsolation(t *testing.T) {
	requireSQL(t, "SELECT COUNT(*) FROM mamony_test_fixture.records", "0")
	requireSQL(t, "BEGIN; SET LOCAL app.tenant_id = 'tenant_a'; SELECT COUNT(*) FROM mamony_test_fixture.records; COMMIT;", "1")
	requireSQL(t, "BEGIN; SET LOCAL app.tenant_id = 'tenant_a'; SELECT COUNT(*) FROM mamony_test_fixture.records WHERE tenant_id = 'tenant_b'; COMMIT;", "0")
	requireSQL(t, "BEGIN; SET LOCAL app.tenant_id = 'tenant_a'; UPDATE mamony_test_fixture.records SET note = 'attacked' WHERE tenant_id = 'tenant_b' RETURNING id; COMMIT;", "")
	requireSQL(t, "BEGIN; SET LOCAL app.tenant_id = 'tenant_a'; DELETE FROM mamony_test_fixture.records WHERE tenant_id = 'tenant_b' RETURNING id; COMMIT;", "")
	denySQL(t, "BEGIN; SET LOCAL app.tenant_id = 'tenant_a'; INSERT INTO mamony_test_fixture.records VALUES (4, 'tenant_b', 'forbidden'); COMMIT;", "42501")
	denySQL(t, "BEGIN; SET LOCAL app.tenant_id = 'tenant_a'; UPDATE mamony_test_fixture.records SET tenant_id = 'tenant_b' WHERE id = 1; COMMIT;", "42501")
	requireSQL(t, "BEGIN; SET LOCAL app.tenant_id = 'tenant_a'; INSERT INTO mamony_test_fixture.records VALUES (3, 'tenant_a', 'allowed') RETURNING id; COMMIT;", "3")
	admin, err := invokePSQL("TEST_DATABASE_ADMIN_URL", "-c", "SELECT note FROM mamony_test_fixture.records WHERE id = 2")
	if err != nil || admin != "fixture-b" {
		t.Fatalf("tenant B record was changed: %q; err=%v", admin, err)
	}
}

func TestPostgresTenantContext(t *testing.T) {
	// One psql backend connection, two sequential transaction contexts.
	// This is NOT yet a test of a Go connection pool, which needs a real driver.
	requireSQL(t, "BEGIN; SET LOCAL app.tenant_id = 'tenant_a'; SELECT COUNT(*) FROM mamony_test_fixture.records WHERE id = 1; COMMIT; SELECT COUNT(*) FROM mamony_test_fixture.records WHERE id = 1;", "1\n0")
	requireSQL(t, "BEGIN; SET LOCAL app.tenant_id = 'tenant_b'; SELECT COUNT(*) FROM mamony_test_fixture.records WHERE id = 2; ROLLBACK; SELECT COUNT(*) FROM mamony_test_fixture.records WHERE id = 2;", "1\n0")
	owner, err := invokePSQL("TEST_DATABASE_ADMIN_URL", "-c",
		"SET ROLE mamony_test_owner; SELECT COUNT(*) FROM mamony_test_fixture.records; RESET ROLE;")
	if err != nil || owner != "0" {
		t.Fatalf("FORCE RLS should apply to table owner: owner=%q err=%v", owner, err)
	}
}

func sanitizedPSQLEnv(env []string) []string {
	out := make([]string, 0, len(env)+2)
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "PG") {
			continue
		}
		out = append(out, entry)
	}
	return append(out, "PGCONNECT_TIMEOUT=5", "PGOPTIONS=-c statement_timeout=5000 -c lock_timeout=1000")
}

func TestPostgresPSQLEnvIsolation(t *testing.T) {
	env := sanitizedPSQLEnv([]string{
		"PATH=/usr/bin", "PGHOSTADDR=10.1.2.3", "PGSERVICE=production",
		"PGDATABASE=payments", "PGPASSWORD=real-secret",
	})
	for _, entry := range env {
		if strings.HasPrefix(entry, "PGHOSTADDR=") || strings.HasPrefix(entry, "PGSERVICE=") ||
			strings.HasPrefix(entry, "PGDATABASE=") || strings.HasPrefix(entry, "PGPASSWORD=") {
			t.Fatalf("libpq override leaked into fixture: %q", entry)
		}
	}
}
