//go:build integration

package postgrestest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPostgresIdentityMigration runs on an explicitly guarded disposable
// test database. The two-run check proves idempotence, not production rollout.
func TestPostgresIdentityMigration(t *testing.T) {
	dir, err := filepath.Abs("../../db/migrations/identity")
	if err != nil {
		t.Fatal(err)
	}
	run := func(dsn string, success bool) {
		t.Helper()
		cmd := exec.Command("go", "run", "../../cmd/mamony-identity-migrate")
		cmd.Env = append(sanitizedPSQLEnv(os.Environ()),
			"MAMONY_IDENTITY_MIGRATION_DSN="+dsn, "MAMONY_IDENTITY_MIGRATIONS_DIR="+dir)
		out, err := cmd.CombinedOutput()
		if (err == nil) != success {
			t.Fatalf("identity migration result: wantSuccess=%v err=%v output=%q", success, err, out)
		}
	}
	run(os.Getenv("TEST_IDENTITY_RUNTIME_URL"), false)
	run(os.Getenv("TEST_PLATFORM_MIGRATION_URL"), false)
	run(os.Getenv("TEST_DATABASE_ADMIN_URL"), false)
	run(os.Getenv("TEST_IDENTITY_MIGRATION_URL"), true)
	run(os.Getenv("TEST_IDENTITY_MIGRATION_URL"), true)

	out, err := invokePSQL("TEST_IDENTITY_MIGRATION_URL", "-c",
		"SELECT version || ':' || dirty FROM mamony_identity.schema_migrations")
	if err != nil || out != "1:false" {
		t.Fatalf("identity migration should be at clean version 1: %q err=%v", out, err)
	}
	out, err = invokePSQL("TEST_IDENTITY_MIGRATION_URL", "-c",
		"SELECT count(*) FROM mamony_identity.migration_fingerprints")
	if err != nil || out != "1" {
		t.Fatalf("identity fingerprint missing: %q err=%v", out, err)
	}

	for _, stmt := range []string{
		"CREATE TABLE mamony_identity.illegal (id integer)",
		"SELECT * FROM mamony_identity.password_verifiers",
		"UPDATE mamony_identity.subjects SET status='ACTIVE'",
		"DELETE FROM mamony_identity.security_outbox",
		"SELECT * FROM mamony_identity.migration_fingerprints",
		"SELECT * FROM mamony_identity.schema_migrations",
		"SET ROLE mamony_identity_migrator",
	} {
		output, err := invokePSQL("TEST_IDENTITY_RUNTIME_URL", "-c", stmt)
		if err == nil || !strings.Contains(output, "42501") {
			t.Fatalf("identity runtime role must fail SQLSTATE 42501: query=%q err=%v output=%q", stmt, err, output)
		}
	}
	// Even an INSERT-capable runtime cannot manufacture an already active or
	// verified account before the separate verification migration is accepted.
	activeID := "sub_AAAAAAAAAAAAAAAAAAAAAA"
	output, err := invokePSQL("TEST_IDENTITY_RUNTIME_URL", "-c",
		"INSERT INTO mamony_identity.subjects (subject_id, status, created_at) VALUES ('"+activeID+"','ACTIVE',now())")
	if err == nil || !strings.Contains(output, "23514") {
		t.Fatalf("direct ACTIVE insert must fail the migration constraint: %q %v", output, err)
	}
	for _, stmt := range []string{
		"SELECT * FROM mamony_identity.subjects",
		"SELECT * FROM mamony_identity.email_aliases",
		"SELECT * FROM mamony_identity.security_outbox",
	} {
		output, err := invokePSQL("TEST_PLATFORM_RUNTIME_URL", "-c", stmt)
		if err == nil || !strings.Contains(output, "42501") {
			t.Fatalf("cross-schema access must fail: query=%q err=%v output=%q", stmt, err, output)
		}
	}
}
