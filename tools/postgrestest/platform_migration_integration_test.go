//go:build integration

package postgrestest

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runPlatformMigration(t *testing.T, dir, dsn string, wantSuccess bool) string {
	t.Helper()
	cmd := exec.Command("go", "run", "../../cmd/mamony-migrate")
	cmd.Env = append(sanitizedPSQLEnv(os.Environ()),
		"MAMONY_MIGRATION_DSN="+dsn, "MAMONY_MIGRATIONS_DIR="+dir)
	output, err := cmd.CombinedOutput()
	if (err == nil) != wantSuccess {
		t.Fatalf("migration outcome unexpected (wantSuccess=%v): err=%v output=%s", wantSuccess, err, output)
	}
	return string(output)
}

func signMigration(t *testing.T, dir string, version int, body string) {
	t.Helper()
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
	sql := body + "\n-- mamony:integrity:v1\n" +
		fmt.Sprintf("INSERT INTO mamony_platform.migration_fingerprints (version, sha256) VALUES (%d, '%s');\n", version, sum)
	name := fmt.Sprintf("%06d_test.up.sql", version)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(sql), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresPlatformMigration(t *testing.T) {
	migratorURL := os.Getenv("TEST_PLATFORM_MIGRATION_URL")
	runtimeURL := os.Getenv("TEST_PLATFORM_RUNTIME_URL")
	if migratorURL == "" || runtimeURL == "" {
		t.Fatal("missing dedicated platform test role URLs")
	}

	dir, err := filepath.Abs("../../db/migrations/platform")
	if err != nil {
		t.Fatal(err)
	}

	// The runtime and bootstrap admin cannot silently become the migrator.
	runPlatformMigration(t, dir, runtimeURL, false)
	runPlatformMigration(t, dir, os.Getenv("TEST_DATABASE_ADMIN_URL"), false)

	// Empty database must apply one migration, then be idempotent.
	runPlatformMigration(t, dir, migratorURL, true)
	runPlatformMigration(t, dir, migratorURL, true)
	owner, err := invokePSQL("TEST_PLATFORM_MIGRATION_URL", "-c", "SELECT version || ':' || dirty FROM mamony_platform.schema_migrations")
	if err != nil || owner != "1:false" {
		t.Fatalf("migration version is not clean: %q err=%v", owner, err)
	}
	requireRuntimeDenied := func(sql, state string) {
		t.Helper()
		got, err := invokePSQL("TEST_PLATFORM_RUNTIME_URL", "-c", sql)
		if err == nil || !strings.Contains(got, state) {
			t.Fatalf("runtime operation must fail with SQLSTATE %s: err=%v output=%q", state, err, got)
		}
	}
	requireRuntimeDenied("CREATE TABLE mamony_platform.illegal(id int)", "42501")
	requireRuntimeDenied("SELECT * FROM mamony_platform.migration_fingerprints", "42501")
	requireRuntimeDenied("SELECT * FROM mamony_platform.schema_migrations", "42501")

	// Modified SQL with the original trailer must fail before DB writes.
	temp := t.TempDir()
	firstSQL, err := os.ReadFile(filepath.Join(dir, "000001_initialize.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temp, "000001_initialize.up.sql"), firstSQL, 0600); err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(firstSQL), "foundation.", "foundation changed.", 1)
	if err := os.WriteFile(filepath.Join(temp, "000001_initialize.up.sql"), []byte(mutated), 0600); err != nil {
		t.Fatal(err)
	}
	runPlatformMigration(t, temp, migratorURL, false)

	// Re-signing the local file must NOT override the committed history.
	body := strings.Split(string(firstSQL), "\n-- mamony:integrity:v1\n")[0]
	signMigration(t, temp, 1, "-- modified migration content\n"+body)
	// Rename to the exact applied version name; the test file and original
	// deliberately share a version and Load must reject this as duplicate.
	runPlatformMigration(t, temp, migratorURL, false)
	if err := os.Remove(filepath.Join(temp, "000001_initialize.up.sql")); err != nil {
		t.Fatal(err)
	}
	runPlatformMigration(t, temp, migratorURL, false)

	// Missing applied-history row is a hard failure, not an auto-repair.
	if out, err := invokePSQL("TEST_PLATFORM_MIGRATION_URL", "-c",
		"DELETE FROM mamony_platform.migration_fingerprints WHERE version=1"); err != nil {
		t.Fatalf("remove a synthetic history row: %s: %v", out, err)
	}
	runPlatformMigration(t, dir, migratorURL, false)
	if out, err := invokePSQL("TEST_PLATFORM_MIGRATION_URL", "-c",
		"INSERT INTO mamony_platform.migration_fingerprints (version, sha256) VALUES (1, '6a9be3034e4f5834951341ac30f4e1f6d2a6f2528b933f74817d5831cd1fb810')"); err != nil {
		t.Fatalf("restore fixture digest: %s: %v", out, err)
	}

	// Verify a valid upgrade using the same versioned migration runner.
	if err := os.WriteFile(filepath.Join(temp, "000001_initialize.up.sql"), firstSQL, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(temp, "000001_test.up.sql")); err != nil {
		t.Fatal(err)
	}
	signMigration(t, temp, 2, "CREATE TABLE mamony_platform.upgrade_canary (id int PRIMARY KEY);\n")
	runPlatformMigration(t, temp, migratorURL, true)
	owner, err = invokePSQL("TEST_PLATFORM_MIGRATION_URL", "-c",
		"SELECT version || ':' || dirty FROM mamony_platform.schema_migrations")
	if err != nil || owner != "2:false" {
		t.Fatalf("upgrade version is not clean: %q err=%v", owner, err)
	}

	// A bad SQL statement must rollback the DDL in that migration and
	// leave golang-migrate's dirty version as an explicit blocker.
	signMigration(t, temp, 3,
		"CREATE TABLE mamony_platform.must_rollback(id int);\nSELECT 1 / 0;\n")
	runPlatformMigration(t, temp, migratorURL, false)
	version, err := invokePSQL("TEST_PLATFORM_MIGRATION_URL", "-c",
		"SELECT version || ':' || dirty FROM mamony_platform.schema_migrations")
	if err != nil || version != "3:true" {
		t.Fatalf("failed migration did not leave dirty state: %q err=%v", version, err)
	}
	rolledBack, err := invokePSQL("TEST_PLATFORM_MIGRATION_URL", "-c",
		"SELECT to_regclass('mamony_platform.must_rollback') IS NULL")
	if err != nil || rolledBack != "t" {
		t.Fatalf("DDL from failed migration survived: %q err=%v", rolledBack, err)
	}
	runPlatformMigration(t, temp, migratorURL, false)
}
