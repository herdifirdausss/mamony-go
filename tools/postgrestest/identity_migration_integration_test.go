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
	if err != nil || out != "15:false" {
		t.Fatalf("identity migration should be at clean version 15: %q err=%v", out, err)
	}
	out, err = invokePSQL("TEST_IDENTITY_MIGRATION_URL", "-c",
		"SELECT count(*) FROM mamony_identity.migration_fingerprints")
	if err != nil || out != "15" {
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

// TestPostgresIdentityMaintenanceRole proves the v6 role is allowed to call
// only two bounded functions; even an SQL-injection in its one-shot process
// cannot read proof material or manually delete anonymous budget rows.
func TestPostgresIdentityMaintenanceRole(t *testing.T) {
	for _, sql := range []string{
		"SELECT * FROM mamony_identity.email_proof_deliveries",
		"SELECT * FROM mamony_identity.anonymous_admission_buckets",
		"SELECT * FROM mamony_identity.password_verifiers",
		"SELECT * FROM mamony_identity.email_verification_challenges",
		"DELETE FROM mamony_identity.anonymous_admission_buckets",
		"UPDATE mamony_identity.email_proof_deliveries SET sealed_token=NULL",
		"UPDATE mamony_identity.subjects SET status='ACTIVE'",
		"DELETE FROM mamony_identity.security_outbox",
		"SELECT * FROM mamony_identity.migration_fingerprints",
		"SET ROLE mamony_identity_migrator",
	} {
		output, err := invokePSQL("TEST_IDENTITY_MAINTENANCE_URL", "-c", sql)
		if err == nil || !strings.Contains(output, "42501") {
			t.Fatalf("maintenance role must reject arbitrary query %q: %v %s", sql, err, output)
		}
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL", "TEST_IDENTITY_VERIFIER_URL",
		"TEST_IDENTITY_DELIVERY_URL", "TEST_PLATFORM_RUNTIME_URL",
	} {
		for _, fun := range []string{
			"purge_expired_email_envelopes", "purge_expired_anonymous_buckets",
		} {
			out, err := invokePSQL(role, "-c", "SELECT mamony_identity."+fun+"()")
			if err == nil || !strings.Contains(out, "42501") {
				t.Fatalf("non-maintenance role used maintenance definer function: role=%s fn=%s err=%v output=%s",
					role, fun, err, out)
			}
		}
	}
	output, err := invokePSQL("TEST_IDENTITY_MAINTENANCE_URL", "-c", `SELECT
		(current_user='mamony_identity_maintenance_runtime'
		AND NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole
		AND NOT rolbypassrls)::text FROM pg_roles WHERE rolname=current_user`)
	if err != nil || output != "true" {
		t.Fatalf("maintenance runtime role must be unprivileged: %v %s", err, output)
	}
}

// TestPostgresIdentityZAuthenticatorRole runs after the dedicated Identity
// migration fixture and verifies password-read authority cannot be reused for
// registration, proof operations, outbox edits or privilege escalation.
func TestPostgresIdentityZAuthenticatorRole(t *testing.T) {
	for _, sql := range []string{
		"SELECT subject_id, status FROM mamony_identity.subjects LIMIT 1",
		"SELECT canonical_email, subject_id, verified_at FROM mamony_identity.email_aliases LIMIT 1",
		"SELECT subject_id, kdf_version, memory_kib, iterations, parallelism, salt, digest FROM mamony_identity.password_verifiers LIMIT 1",
	} {
		out, err := invokePSQL("TEST_IDENTITY_AUTHENTICATOR_URL", "-c", sql)
		if err != nil {
			t.Fatalf("read-only authentication role lost approved credential read: %v %s", err, out)
		}
	}
	for _, sql := range []string{
		"CREATE TABLE mamony_identity.illegal (id integer)",
		"SELECT * FROM mamony_identity.email_verification_challenges",
		"SELECT * FROM mamony_identity.email_proof_deliveries",
		"SELECT * FROM mamony_identity.anonymous_admission_buckets",
		"SELECT * FROM mamony_identity.security_outbox",
		"SELECT * FROM mamony_identity.password_verifiers",
		"SELECT created_at FROM mamony_identity.password_verifiers",
		"SELECT * FROM mamony_identity.migration_fingerprints",
		"SELECT * FROM mamony_identity.schema_migrations",
		"UPDATE mamony_identity.subjects SET status='ACTIVE'",
		"UPDATE mamony_identity.email_aliases SET verified_at=now()",
		"UPDATE mamony_identity.password_verifiers SET digest='\\x00'::bytea",
		"DELETE FROM mamony_identity.password_verifiers",
		"INSERT INTO mamony_identity.security_outbox (event_id) VALUES ('bad')",
		"SET ROLE mamony_identity_migrator",
	} {
		out, err := invokePSQL("TEST_IDENTITY_AUTHENTICATOR_URL", "-c", sql)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("authenticator role must reject %q: err=%v output=%s", sql, err, out)
		}
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL",
		"TEST_IDENTITY_VERIFIER_URL",
		"TEST_IDENTITY_DELIVERY_URL",
	} {
		out, err := invokePSQL(role, "-c",
			"SELECT digest FROM mamony_identity.password_verifiers LIMIT 1")
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("non-authenticator role read password digest: %s err=%v output=%s", role, err, out)
		}
	}
	out, err := invokePSQL("TEST_IDENTITY_AUTHENTICATOR_URL", "-c", `SELECT
		(NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole
		AND NOT rolreplication AND NOT rolbypassrls)::text
		FROM pg_roles WHERE rolname=current_user`)
	if err != nil || out != "true" {
		t.Fatalf("authenticator must not inherit elevated DB privilege: %v %s", err, out)
	}
}

// TestPostgresIdentityZSessionRole verifies that the dedicated session runtime
// only executes grant-consume/resolve/revoke SECURITY DEFINER functions and no direct SQL.
func TestPostgresIdentityZSessionRole(t *testing.T) {
	for _, stmt := range []string{
		"SELECT mamony_identity.issue_opaque_session_with_grant(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')",
		"SELECT mamony_identity.resolve_opaque_session(decode(repeat('22',32),'hex')) IS NULL",
		"SELECT mamony_identity.revoke_opaque_session(decode(repeat('22',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')",
	} {
		out, err := invokePSQL("TEST_IDENTITY_SESSION_URL", "-c", stmt)
		if err != nil {
			t.Fatalf("approved session function denied: %q %s %v", stmt, out, err)
		}
	}
	for _, sql := range []string{
		"SELECT * FROM mamony_identity.password_login_grants",
		"UPDATE mamony_identity.opaque_sessions SET last_seen_at=clock_timestamp()",
		"SELECT mamony_identity.stage_password_auth_grant('sub_AAAAAAAAAAAAAAAAAAAAAA',1,decode(repeat('11',32),'hex'))",
		"SELECT * FROM mamony_identity.opaque_sessions",
		"SELECT token_digest FROM mamony_identity.opaque_sessions LIMIT 1",
		"INSERT INTO mamony_identity.opaque_sessions (token_digest) VALUES ('\\x00'::bytea)",
		"UPDATE mamony_identity.opaque_sessions SET revoked_at=now()",
		"DELETE FROM mamony_identity.opaque_sessions",
		"SELECT * FROM mamony_identity.subjects",
		"SELECT session_epoch FROM mamony_identity.subjects LIMIT 1",
		"UPDATE mamony_identity.subjects SET status='ACTIVE'",
		"SELECT * FROM mamony_identity.email_aliases",
		"SELECT digest FROM mamony_identity.password_verifiers LIMIT 1",
		"SELECT * FROM mamony_identity.security_outbox",
		"INSERT INTO mamony_identity.security_outbox (event_id) VALUES ('bad')",
		"SELECT * FROM mamony_identity.email_verification_challenges",
		"SELECT * FROM mamony_identity.anonymous_admission_buckets",
		"SELECT mamony_identity.purge_expired_opaque_sessions()",
		"SELECT * FROM mamony_identity.migration_fingerprints",
		"SET ROLE mamony_identity_migrator",
		"CREATE TABLE mamony_identity.session_escape(id integer)",
	} {
		out, err := invokePSQL("TEST_IDENTITY_SESSION_URL", "-c", sql)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("session role gained forbidden privilege: %q err=%v out=%s", sql, err, out)
		}
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL", "TEST_IDENTITY_AUTHENTICATOR_URL",
		"TEST_IDENTITY_VERIFIER_URL", "TEST_IDENTITY_DELIVERY_URL",
		"TEST_PLATFORM_RUNTIME_URL",
	} {
		out, err := invokePSQL(role, "-c",
			"SELECT mamony_identity.issue_opaque_session_with_grant(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')")
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("non-session role could mint: %s err=%v out=%s", role, err, out)
		}
	}
	out, err := invokePSQL("TEST_IDENTITY_SESSION_URL", "-c",
		"SELECT (NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolreplication AND NOT rolbypassrls)::text FROM pg_roles WHERE rolname=current_user")
	if err != nil || out != "true" {
		t.Fatalf("session runtime has elevated privileges: err=%v out=%s", err, out)
	}
}

func TestPostgresIdentityZAuthenticationGrantRole(t *testing.T) {
	legacy, legacyErr := invokePSQL("TEST_IDENTITY_MIGRATION_URL", "-c",
		"SELECT to_regprocedure('mamony_identity.stage_password_auth_grant(text,bytea)') IS NULL")
	if legacyErr != nil || legacy != "t" {
		t.Fatalf("old unversioned authentication grant function still exists: %v %q", legacyErr, legacy)
	}
	out, err := invokePSQL("TEST_IDENTITY_AUTH_GRANT_URL", "-c",
		"SELECT mamony_identity.stage_password_auth_grant('invalid',1,decode(repeat('11',32),'hex'))")
	if err != nil || out != "f" {
		t.Fatalf("dedicated grant role cannot stage approved typed function: %v %s", err, out)
	}
	for _, sql := range []string{
		"SELECT mamony_identity.issue_opaque_session_with_grant(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')",
		"SELECT mamony_identity.resolve_opaque_session(decode(repeat('22',32),'hex'))",
		"SELECT mamony_identity.revoke_opaque_session(decode(repeat('22',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')",
		"SELECT * FROM mamony_identity.password_login_grants",
		"INSERT INTO mamony_identity.password_login_grants (grant_digest) VALUES ('\\x00'::bytea)",
		"UPDATE mamony_identity.password_login_grants SET consumed_at=now()",
		"SELECT digest FROM mamony_identity.password_verifiers",
		"SELECT * FROM mamony_identity.opaque_sessions",
		"SELECT * FROM mamony_identity.subjects",
		"SELECT mamony_identity.purge_expired_password_login_grants()",
		"SET ROLE mamony_identity_migrator",
	} {
		out, err := invokePSQL("TEST_IDENTITY_AUTH_GRANT_URL", "-c", sql)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("grant-stage role gained forbidden capability %q: err=%v out=%s", sql, err, out)
		}
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL", "TEST_IDENTITY_VERIFIER_URL",
		"TEST_IDENTITY_AUTHENTICATOR_URL", "TEST_IDENTITY_SESSION_URL",
		"TEST_IDENTITY_DELIVERY_URL", "TEST_PLATFORM_RUNTIME_URL",
	} {
		out, err := invokePSQL(role, "-c",
			"SELECT mamony_identity.stage_password_auth_grant('invalid',1,decode(repeat('11',32),'hex'))")
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("non-grant role staged authentication: %s err=%v out=%s", role, err, out)
		}
	}
	for _, role := range []string{"TEST_IDENTITY_SESSION_URL", "TEST_IDENTITY_AUTH_GRANT_URL"} {
		out, err := invokePSQL(role, "-c",
			"SELECT mamony_identity.issue_opaque_session('sub_AAAAAAAAAAAAAAAAAAAAAA',decode(repeat('22',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')")
		if err == nil || !strings.Contains(out, "42883") {
			t.Fatalf("removed subject-only function survived: %s err=%v out=%s", role, err, out)
		}
	}
}
