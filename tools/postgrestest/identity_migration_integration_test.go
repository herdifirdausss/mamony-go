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
	if err != nil || out != "5:false" {
		t.Fatalf("identity migration should be at clean version 5: %q err=%v", out, err)
	}
	out, err = invokePSQL("TEST_IDENTITY_MIGRATION_URL", "-c",
		"SELECT count(*) FROM mamony_identity.migration_fingerprints")
	if err != nil || out != "5" {
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
		"SELECT * FROM mamony_identity.email_verification_challenges",
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
	if err == nil || !strings.Contains(output, "42501") {
		t.Fatalf("registration role must not INSERT the protected status column: %q %v", output, err)
	}
	output, err = invokePSQL("TEST_IDENTITY_RUNTIME_URL", "-c",
		"INSERT INTO mamony_identity.email_aliases (canonical_email, subject_id, verified_at, created_at) VALUES ('fake@example.test','"+activeID+"',now(),now())")
	if err == nil || !strings.Contains(output, "42501") {
		t.Fatalf("registration role must not INSERT the verified_at column: %q %v", output, err)
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

func TestPostgresIdentityVerifierRole(t *testing.T) {
	for _, stmt := range []string{
		"CREATE TABLE mamony_identity.invalid_table (id integer)",
		"SELECT * FROM mamony_identity.password_verifiers",
		"SELECT * FROM mamony_identity.migration_fingerprints",
		"SELECT * FROM mamony_identity.schema_migrations",
		"DELETE FROM mamony_identity.email_verification_challenges",
		"DELETE FROM mamony_identity.security_outbox",
		"UPDATE mamony_identity.email_verification_challenges SET expires_at=now()",
		"SET ROLE mamony_identity_migrator",
	} {
		output, err := invokePSQL("TEST_IDENTITY_VERIFIER_URL", "-c", stmt)
		if err == nil || !strings.Contains(output, "42501") {
			t.Fatalf("verifier runtime must deny %q: %q err=%v", stmt, output, err)
		}
	}
}

func TestPostgresIdentityDeliveryRole(t *testing.T) {
	for _, stmt := range []string{
		"CREATE TABLE mamony_identity.delivery_attack (id integer)",
		"SELECT * FROM mamony_identity.password_verifiers",
		"SELECT * FROM mamony_identity.migration_fingerprints",
		"SELECT * FROM mamony_identity.schema_migrations",
		"INSERT INTO mamony_identity.email_verification_challenges (challenge_id) VALUES ('bad')",
		"UPDATE mamony_identity.subjects SET status='ACTIVE'",
		"UPDATE mamony_identity.email_aliases SET verified_at=clock_timestamp()",
		"DELETE FROM mamony_identity.email_proof_deliveries",
		"UPDATE mamony_identity.email_proof_deliveries SET sealed_token='\\x00'::bytea",
		"SELECT token_digest FROM mamony_identity.email_verification_challenges",
		"SET ROLE mamony_identity_migrator",
	} {
		output, err := invokePSQL("TEST_IDENTITY_DELIVERY_URL", "-c", stmt)
		if err == nil || !strings.Contains(output, "42501") {
			t.Fatalf("delivery worker role must deny %q: %q err=%v", stmt, output, err)
		}
	}
}

// TestPostgresIdentityAdmissionRole verifies the v5 admission-only table does
// not allow the registration runtime to delete/rewrite identity key material,
// or broaden the roles used for verification and delivery.
func TestPostgresIdentityAdmissionRole(t *testing.T) {
	for _, tc := range []struct {
		env string
		sql string
	}{
		{"TEST_IDENTITY_RUNTIME_URL", "DELETE FROM mamony_identity.anonymous_admission_buckets"},
		{"TEST_IDENTITY_RUNTIME_URL", "UPDATE mamony_identity.anonymous_admission_buckets SET key_digest='\\x00'::bytea"},
		{"TEST_IDENTITY_VERIFIER_URL", "SELECT * FROM mamony_identity.anonymous_admission_buckets"},
		{"TEST_IDENTITY_DELIVERY_URL", "SELECT * FROM mamony_identity.anonymous_admission_buckets"},
		{"TEST_PLATFORM_RUNTIME_URL", "SELECT * FROM mamony_identity.anonymous_admission_buckets"},
	} {
		out, err := invokePSQL(tc.env, "-c", tc.sql)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("untrusted Identity role must refuse %q: %v %s", tc.sql, err, out)
		}
	}
}
