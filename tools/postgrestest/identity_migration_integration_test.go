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
	if err != nil || out != "26:false" {
		t.Fatalf("identity migration should be at clean version 26: %q err=%v", out, err)
	}
	out, err = invokePSQL("TEST_IDENTITY_MIGRATION_URL", "-c",
		"SELECT count(*) FROM mamony_identity.migration_fingerprints")
	if err != nil || out != "26" {
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

func TestPostgresIdentityZPasswordChangeRole(t *testing.T) {
	const grantSQL = "SELECT mamony_identity.stage_password_change_grant('invalid',1,decode(repeat('11',32),'hex'))"
	const changeSQL = "SELECT mamony_identity.rotate_password_verifier(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),decode(repeat('33',16),'hex'),decode(repeat('44',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')"
	out, err := invokePSQL("TEST_IDENTITY_PASSWORD_CHANGE_URL", "-c", changeSQL)
	if err != nil || out != "f" {
		t.Fatalf("password change role denied approved bounded function: output=%q err=%v", out, err)
	}
	for _, sql := range []string{
		grantSQL,
		"SELECT * FROM mamony_identity.password_login_grants",
		"SELECT digest FROM mamony_identity.password_verifiers",
		"UPDATE mamony_identity.password_verifiers SET digest=decode(repeat('44',32),'hex')",
		"UPDATE mamony_identity.subjects SET session_epoch=42",
		"SELECT token_digest FROM mamony_identity.opaque_sessions",
		"SELECT mamony_identity.resolve_opaque_session(decode(repeat('11',32),'hex'))",
		"SELECT mamony_identity.issue_opaque_session_with_grant(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')",
		"INSERT INTO mamony_identity.security_outbox (event_id) VALUES ('bad')",
		"SELECT * FROM mamony_identity.migration_fingerprints",
		"SET ROLE mamony_identity_migrator",
	} {
		out, err := invokePSQL("TEST_IDENTITY_PASSWORD_CHANGE_URL", "-c", sql)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("password change role escaped EXECUTE-only boundary: sql=%q output=%s err=%v", sql, out, err)
		}
	}
	out, err = invokePSQL("TEST_IDENTITY_AUTH_GRANT_URL", "-c", grantSQL)
	if err != nil || out != "f" {
		t.Fatalf("grant role cannot stage new PASSWORD_CHANGE purpose: output=%q err=%v", out, err)
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL", "TEST_IDENTITY_VERIFIER_URL",
		"TEST_IDENTITY_AUTHENTICATOR_URL", "TEST_IDENTITY_SESSION_URL",
		"TEST_IDENTITY_DELIVERY_URL", "TEST_PLATFORM_RUNTIME_URL",
	} {
		for _, sql := range []string{grantSQL, changeSQL} {
			out, err := invokePSQL(role, "-c", sql)
			if err == nil || !strings.Contains(out, "42501") {
				t.Fatalf("role %s escaped password change privilege matrix: sql=%q out=%q err=%v",
					role, sql, out, err)
			}
		}
	}
	for _, role := range []string{"TEST_IDENTITY_AUTH_GRANT_URL", "TEST_IDENTITY_PASSWORD_CHANGE_URL"} {
		out, err := invokePSQL(role, "-c",
			"SELECT (NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolreplication AND NOT rolbypassrls)::text FROM pg_roles WHERE rolname=current_user")
		if err != nil || out != "true" {
			t.Fatalf("role %s inherited privileges: out=%q err=%v", role, out, err)
		}
	}
}

func TestPostgresIdentityZSecurityNoticeRole(t *testing.T) {
	const claim = "SELECT * FROM mamony_identity.claim_password_change_notice('lse_AAAAAAAAAAAAAAAAAAAAAA')"
	const finalize = "SELECT mamony_identity.finalize_password_change_notice_v2('evt_AAAAAAAAAAAAAAAAAAAAAA','lse_AAAAAAAAAAAAAAAAAAAAAA','ACCEPTED',decode(repeat('11',32),'hex'))"
	out, err := invokePSQL("TEST_IDENTITY_SECURITY_NOTICE_URL", "-c", claim)
	if err != nil || out != "" {
		t.Fatalf("notice worker could not execute safe empty claim: %q %v", out, err)
	}
	out, err = invokePSQL("TEST_IDENTITY_SECURITY_NOTICE_URL", "-c", finalize)
	if err != nil || out != "f" {
		t.Fatalf("notice worker cannot execute fenced finalize: %q %v", out, err)
	}
	for _, stmt := range []string{
		"CREATE TABLE mamony_identity.malicious_notice (id integer)",
		"SELECT * FROM mamony_identity.password_change_notices",
		"SELECT * FROM mamony_identity.security_outbox",
		"SELECT * FROM mamony_identity.password_verifiers",
		"SELECT canonical_email FROM mamony_identity.email_aliases",
		"SELECT * FROM mamony_identity.opaque_sessions",
		"UPDATE mamony_identity.password_change_notices SET status='ACCEPTED'",
		"DELETE FROM mamony_identity.password_change_notices",
		"INSERT INTO mamony_identity.security_outbox(event_id) VALUES ('bad')",
		"SELECT mamony_identity.stage_password_change_grant('sub_AAAAAAAAAAAAAAAAAAAAAA',1,decode(repeat('11',32),'hex'))",
		"SELECT mamony_identity.rotate_password_verifier(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),decode(repeat('33',16),'hex'),decode(repeat('44',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')",
		"SELECT mamony_identity.purge_old_password_change_notices()",
		"SET ROLE mamony_identity_migrator",
	} {
		out, err := invokePSQL("TEST_IDENTITY_SECURITY_NOTICE_URL", "-c", stmt)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("notice role escaped EXECUTE-only boundary: %q output=%q err=%v", stmt, out, err)
		}
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL", "TEST_IDENTITY_VERIFIER_URL",
		"TEST_IDENTITY_AUTHENTICATOR_URL", "TEST_IDENTITY_AUTH_GRANT_URL",
		"TEST_IDENTITY_SESSION_URL", "TEST_IDENTITY_PASSWORD_CHANGE_URL",
		"TEST_IDENTITY_DELIVERY_URL", "TEST_PLATFORM_RUNTIME_URL",
	} {
		for _, stmt := range []string{claim, finalize} {
			out, err := invokePSQL(role, "-c", stmt)
			if err == nil || !strings.Contains(out, "42501") {
				t.Fatalf("role %s could claim/finalize alerts: sql=%q out=%q err=%v", role, stmt, out, err)
			}
		}
	}
}

func TestPostgresIdentityZMailgunWebhookRole(t *testing.T) {
	const record = "SELECT mamony_identity.record_mailgun_security_notice_receipt(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),decode(repeat('33',32),'hex'),'DELIVERED',now())"
	for _, stmt := range []string{
		"CREATE TABLE mamony_identity.webhook_attack (id integer)",
		"SELECT * FROM mamony_identity.password_change_notices",
		"SELECT * FROM mamony_identity.security_notice_mailgun_receipts",
		"SELECT * FROM mamony_identity.security_outbox",
		"SELECT * FROM mamony_identity.email_aliases",
		"SELECT * FROM mamony_identity.password_verifiers",
		"SELECT * FROM mamony_identity.opaque_sessions",
		"UPDATE mamony_identity.password_change_notices SET provider_state='DELIVERED'",
		"DELETE FROM mamony_identity.security_notice_mailgun_receipts",
		"SELECT mamony_identity.security_notice_health()",
		"SELECT mamony_identity.purge_old_mailgun_security_receipts()",
		"SET ROLE mamony_identity_migrator",
	} {
		out, err := invokePSQL("TEST_IDENTITY_NOTICE_WEBHOOK_URL", "-c", stmt)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("webhook SQL role escaped append-only EXECUTE: %q out=%q err=%v", stmt, out, err)
		}
	}
	// A valid function call is permitted, but cannot claim receipt contents,
	// security audit or password/change/session permissions.
	out, err := invokePSQL("TEST_IDENTITY_NOTICE_WEBHOOK_URL", "-c", record)
	if err != nil || out != "t" {
		t.Fatalf("webhook receiver could not execute bounded SQL: %q %v", out, err)
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL", "TEST_IDENTITY_VERIFIER_URL",
		"TEST_IDENTITY_AUTHENTICATOR_URL", "TEST_IDENTITY_AUTH_GRANT_URL",
		"TEST_IDENTITY_SESSION_URL", "TEST_IDENTITY_PASSWORD_CHANGE_URL",
		"TEST_IDENTITY_SECURITY_NOTICE_URL", "TEST_IDENTITY_DELIVERY_URL",
		"TEST_PLATFORM_RUNTIME_URL",
	} {
		out, err := invokePSQL(role, "-c", record)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("role %s can spoof Mailgun receipt: out=%q err=%v", role, out, err)
		}
	}
	for _, role := range []string{"TEST_IDENTITY_NOTICE_WEBHOOK_URL",
		"TEST_IDENTITY_RUNTIME_URL", "TEST_IDENTITY_SECURITY_NOTICE_URL"} {
		out, err := invokePSQL(role, "-c",
			"SELECT mamony_identity.finalize_password_change_notice('evt_AAAAAAAAAAAAAAAAAAAAAA','lse_AAAAAAAAAAAAAAAAAAAAAA','ACCEPTED')")
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("role %s can use uncorrelated legacy finalize: %q %v", role, out, err)
		}
	}
	out, err = invokePSQL("TEST_IDENTITY_MAINTENANCE_URL", "-c",
		"SELECT * FROM mamony_identity.security_notice_health()")
	if err != nil || out == "" {
		t.Fatalf("maintenance monitoring role cannot access count-only health: %q %v", out, err)
	}
}

func TestPostgresIdentityZNoticeMonitorRole(t *testing.T) {
	// A monitoring process needs ONLY the aggregate health query, not the
	// broader maintenance ability to delete data or inspect Identity tables.
	const health = "SELECT * FROM mamony_identity.security_notice_health()"
	if out, err := invokePSQL("TEST_IDENTITY_NOTICE_MONITOR_URL", "-c", health); err != nil || out == "" {
		t.Fatalf("read-only health credential cannot view aggregate counts: %q %v", out, err)
	}
	for _, stmt := range []string{
		"CREATE TABLE mamony_identity.notice_monitor_attack (x integer)",
		"SELECT * FROM mamony_identity.password_change_notices",
		"SELECT * FROM mamony_identity.security_notice_mailgun_receipts",
		"SELECT * FROM mamony_identity.security_outbox",
		"SELECT * FROM mamony_identity.email_aliases",
		"SELECT * FROM mamony_identity.opaque_sessions",
		"SELECT * FROM mamony_identity.password_verifiers",
		"UPDATE mamony_identity.password_change_notices SET status='DEAD'",
		"DELETE FROM mamony_identity.security_notice_mailgun_receipts",
		"SELECT mamony_identity.purge_old_mailgun_security_receipts()",
		"SELECT mamony_identity.purge_old_password_change_notices()",
		"SELECT mamony_identity.purge_expired_email_envelopes()",
		"SELECT mamony_identity.claim_password_change_notice('lse_AAAAAAAAAAAAAAAAAAAAAA')",
		"SELECT mamony_identity.finalize_password_change_notice_v2('evt_AAAAAAAAAAAAAAAAAAAAAA','lse_AAAAAAAAAAAAAAAAAAAAAA','ACCEPTED',decode(repeat('11',32),'hex'))",
		"SELECT mamony_identity.record_mailgun_security_notice_receipt(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),decode(repeat('33',32),'hex'),'DELIVERED',now())",
		"SET ROLE mamony_identity_migrator",
		"SET ROLE mamony_identity_maintenance_runtime",
	} {
		out, err := invokePSQL("TEST_IDENTITY_NOTICE_MONITOR_URL", "-c", stmt)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("health credential escalated beyond aggregate EXECUTE: %q output=%q err=%v", stmt, out, err)
		}
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL", "TEST_IDENTITY_VERIFIER_URL",
		"TEST_IDENTITY_AUTHENTICATOR_URL", "TEST_IDENTITY_AUTH_GRANT_URL",
		"TEST_IDENTITY_SESSION_URL", "TEST_IDENTITY_PASSWORD_CHANGE_URL",
		"TEST_IDENTITY_SECURITY_NOTICE_URL", "TEST_IDENTITY_NOTICE_WEBHOOK_URL",
		"TEST_PLATFORM_RUNTIME_URL",
	} {
		out, err := invokePSQL(role, "-c", health)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("role %s can read operator-only notice counts: %q %v", role, out, err)
		}
	}
	out, err := invokePSQL("TEST_IDENTITY_NOTICE_MONITOR_URL", "-c",
		"SELECT rolcanlogin::text || ':' || rolsuper::text || ':' || rolcreaterole::text || ':' || rolcreatedb::text || ':' || rolinherit::text || ':' || rolbypassrls::text FROM pg_roles WHERE rolname=current_user")
	if err != nil || out != "true:false:false:false:false:false" {
		t.Fatalf("notice monitoring role has elevated DB attributes: %q %v", out, err)
	}
}

// TestPostgresIdentityZRecoveryIssuerRole proves the v19 issuance credential
// cannot read recovery envelopes, inspect account data or mutate a verifier.
// Only the purpose-bound, owner-controlled issue function may be executed.
func TestPostgresIdentityZRecoveryIssuerRole(t *testing.T) {
	for _, sql := range []string{
		"SELECT mamony_identity.issue_password_recovery('missing@example.test','rcv_AAAAAAAAAAAAAAAAAAAAAA',decode(repeat('11',32),'hex'),'fixture-key',decode(repeat('22',72),'hex'))",
	} {
		out, err := invokePSQL("TEST_IDENTITY_RECOVERY_ISSUER_URL", "-c", sql)
		if err != nil || out != "f" {
			t.Fatalf("dedicated recovery issuer lost narrow EXECUTE permission: out=%s err=%v", out, err)
		}
	}
	for _, stmt := range []string{
		"CREATE TABLE mamony_identity.recovery_escape(id integer)",
		"SELECT * FROM mamony_identity.password_recovery_challenges",
		"SELECT * FROM mamony_identity.password_recovery_deliveries",
		"SELECT * FROM mamony_identity.email_aliases",
		"SELECT * FROM mamony_identity.subjects",
		"SELECT * FROM mamony_identity.password_verifiers",
		"SELECT * FROM mamony_identity.email_verification_challenges",
		"SELECT * FROM mamony_identity.opaque_sessions",
		"SELECT * FROM mamony_identity.password_login_grants",
		"SELECT * FROM mamony_identity.anonymous_admission_buckets",
		"SELECT * FROM mamony_identity.security_outbox",
		"SELECT * FROM mamony_identity.migration_fingerprints",
		"UPDATE mamony_identity.password_verifiers SET digest='\\x00'::bytea",
		"DELETE FROM mamony_identity.password_recovery_challenges",
		"SELECT mamony_identity.rotate_password_verifier(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),decode(repeat('33',16),'hex'),decode(repeat('44',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')",
		"SET ROLE mamony_identity_migrator",
	} {
		out, err := invokePSQL("TEST_IDENTITY_RECOVERY_ISSUER_URL", "-c", stmt)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("recovery issuer gained forbidden SQL: %q err=%v out=%s", stmt, err, out)
		}
	}
	for _, env := range []string{
		"TEST_IDENTITY_RUNTIME_URL", "TEST_IDENTITY_VERIFIER_URL",
		"TEST_IDENTITY_PASSWORD_CHANGE_URL", "TEST_IDENTITY_AUTHENTICATOR_URL",
		"TEST_IDENTITY_DELIVERY_URL", "TEST_PLATFORM_RUNTIME_URL",
	} {
		out, err := invokePSQL(env, "-c",
			"SELECT mamony_identity.issue_password_recovery('missing@example.test','rcv_AAAAAAAAAAAAAAAAAAAAAA',decode(repeat('11',32),'hex'),'fixture-key',decode(repeat('22',72),'hex'))")
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("other role gained recovery issuance permission: role=%s err=%v out=%s", env, err, out)
		}
	}
}

// TestPostgresIdentityZRecoveryDeliveryRole exercises the isolated v20
// SECURITY DEFINER surface: only lease claim/finalize, no direct table access.
func TestPostgresIdentityZRecoveryDeliveryRole(t *testing.T) {
	lease := "lse_AAAAAAAAAAAAAAAAAAAAAA"
	challenge := "rcv_AAAAAAAAAAAAAAAAAAAAAA"
	out, err := invokePSQL("TEST_IDENTITY_RECOVERY_DELIVERY_URL", "-c",
		"SELECT mamony_identity.finalize_password_recovery_delivery('"+challenge+"','"+lease+"','TEMPORARY')")
	if err != nil || out != "f" {
		t.Fatalf("recovery delivery must EXECUTE only approved function: %q %v", out, err)
	}
	if _, err := invokePSQL("TEST_IDENTITY_RECOVERY_DELIVERY_URL", "-c",
		"SELECT * FROM mamony_identity.claim_password_recovery_delivery('"+lease+"')"); err != nil {
		t.Fatalf("recovery delivery claim EXECUTE was denied: %v", err)
	}
	for _, sql := range []string{
		"CREATE TABLE mamony_identity.delivery_escape(id integer)",
		"SELECT * FROM mamony_identity.password_recovery_challenges",
		"SELECT * FROM mamony_identity.password_recovery_deliveries",
		"SELECT * FROM mamony_identity.email_aliases",
		"SELECT * FROM mamony_identity.subjects",
		"SELECT * FROM mamony_identity.password_verifiers",
		"SELECT * FROM mamony_identity.email_verification_challenges",
		"SELECT * FROM mamony_identity.email_proof_deliveries",
		"SELECT * FROM mamony_identity.opaque_sessions",
		"SELECT * FROM mamony_identity.password_login_grants",
		"SELECT * FROM mamony_identity.anonymous_admission_buckets",
		"SELECT * FROM mamony_identity.security_outbox",
		"SELECT * FROM mamony_identity.migration_fingerprints",
		"SELECT * FROM mamony_identity.schema_migrations",
		"UPDATE mamony_identity.password_recovery_deliveries SET status='ACCEPTED'",
		"UPDATE mamony_identity.password_verifiers SET digest=decode(repeat('11',32),'hex')",
		"SELECT mamony_identity.issue_password_recovery('missing@example.test','" + challenge + "',decode(repeat('11',32),'hex'),'fixture-key',decode(repeat('22',72),'hex'))",
		"SET ROLE mamony_identity_migrator",
	} {
		out, err := invokePSQL("TEST_IDENTITY_RECOVERY_DELIVERY_URL", "-c", sql)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("recovery delivery role escalated on %q: out=%q err=%v", sql, out, err)
		}
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL", "TEST_IDENTITY_RECOVERY_ISSUER_URL",
		"TEST_IDENTITY_VERIFIER_URL", "TEST_IDENTITY_DELIVERY_URL",
		"TEST_IDENTITY_PASSWORD_CHANGE_URL", "TEST_PLATFORM_RUNTIME_URL",
	} {
		out, err := invokePSQL(role, "-c",
			"SELECT * FROM mamony_identity.claim_password_recovery_delivery('"+lease+"')")
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("non-worker role called recovery delivery: role=%s out=%q err=%v", role, out, err)
		}
	}
}

// TestPostgresIdentityZRecoveryRedeemerRole proves the v21 credential can
// only EXECUTE the owner-controlled recovery transaction. It has no table
// access, no proof/assurance grant issuance and no session minting.
func TestPostgresIdentityZRecoveryRedeemerRole(t *testing.T) {
	const redeem = "SELECT mamony_identity.redeem_password_recovery(" +
		"decode(repeat('11',32),'hex'),'missing@example.test'," +
		"decode(repeat('22',16),'hex'),decode(repeat('33',32),'hex')," +
		"'evt_AAAAAAAAAAAAAAAAAAAAAA')"
	out, err := invokePSQL("TEST_IDENTITY_RECOVERY_REDEEMER_URL", "-c", redeem)
	if err != nil || out != "f" {
		t.Fatalf("recovery redemption function unavailable: out=%s err=%v", out, err)
	}
	for _, stmt := range []string{
		"CREATE TABLE mamony_identity.recovery_redemption_escape(id integer)",
		"SELECT * FROM mamony_identity.password_recovery_challenges",
		"SELECT * FROM mamony_identity.password_recovery_deliveries",
		"SELECT * FROM mamony_identity.password_recovery_assurance_grants",
		"SELECT * FROM mamony_identity.password_recovery_notices",
		"SELECT * FROM mamony_identity.email_aliases",
		"SELECT * FROM mamony_identity.subjects",
		"SELECT * FROM mamony_identity.password_verifiers",
		"SELECT * FROM mamony_identity.opaque_sessions",
		"SELECT * FROM mamony_identity.password_login_grants",
		"SELECT * FROM mamony_identity.security_outbox",
		"SELECT * FROM mamony_identity.password_change_notices",
		"SELECT * FROM mamony_identity.migration_fingerprints",
		"UPDATE mamony_identity.password_verifiers SET digest=decode(repeat('11',32),'hex')",
		"DELETE FROM mamony_identity.password_recovery_challenges",
		"INSERT INTO mamony_identity.password_recovery_assurance_grants(subject_id,subject_epoch,approved_at,expires_at) VALUES ('sub_AAAAAAAAAAAAAAAAAAAAAA',1,clock_timestamp(),clock_timestamp()+interval '5 minutes')",
		"SELECT mamony_identity.issue_password_recovery('missing@example.test','rcv_AAAAAAAAAAAAAAAAAAAAAA',decode(repeat('11',32),'hex'),'test-key',decode(repeat('22',71),'hex'))",
		"SELECT * FROM mamony_identity.claim_password_recovery_delivery('lse_AAAAAAAAAAAAAAAAAAAAAA')",
		"SET ROLE mamony_identity_migrator",
	} {
		out, err := invokePSQL("TEST_IDENTITY_RECOVERY_REDEEMER_URL", "-c", stmt)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("recovery redeemer exceeded EXECUTE-only boundary: %s out=%s err=%v", stmt, out, err)
		}
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL", "TEST_IDENTITY_VERIFIER_URL",
		"TEST_IDENTITY_PASSWORD_CHANGE_URL",
		"TEST_IDENTITY_RECOVERY_ISSUER_URL",
		"TEST_IDENTITY_RECOVERY_DELIVERY_URL",
		"TEST_IDENTITY_AUTHENTICATOR_URL", "TEST_IDENTITY_SESSION_URL",
		"TEST_PLATFORM_RUNTIME_URL",
	} {
		out, err := invokePSQL(role, "-c", redeem)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("unapproved role gained recovery redemption: %s out=%s err=%v", role, out, err)
		}
	}
}

// TestPostgresIdentityZRecoverySecurityNoticeRoles enforces v22's independent
// sender and count-only monitor credentials, without direct table authority.
func TestPostgresIdentityZRecoverySecurityNoticeRoles(t *testing.T) {
	const lease = "lse_AAAAAAAAAAAAAAAAAAAAAA"
	const event = "evt_AAAAAAAAAAAAAAAAAAAAAA"
	const claim = "SELECT * FROM mamony_identity.claim_password_recovery_notice('" + lease + "')"
	const finalize = "SELECT mamony_identity.finalize_password_recovery_notice('" +
		event + "','" + lease + "','TEMPORARY',NULL)"
	out, err := invokePSQL("TEST_IDENTITY_RECOVERY_NOTICE_URL", "-c", finalize)
	if err != nil || out != "f" {
		t.Fatalf("recovery notice sender cannot call finalize: %s %v", out, err)
	}
	out, err = invokePSQL("TEST_IDENTITY_RECOVERY_NOTICE_URL", "-c", claim)
	if err != nil {
		t.Fatalf("recovery notice sender cannot claim queue: %s %v", out, err)
	}
	out, err = invokePSQL("TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL", "-c",
		"SELECT * FROM mamony_identity.password_recovery_notice_health()")
	if err != nil {
		t.Fatalf("count-only recovery monitor denied health: %s %v", out, err)
	}
	for _, role := range []string{
		"TEST_IDENTITY_RECOVERY_NOTICE_URL",
		"TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL",
	} {
		for _, sql := range []string{
			"CREATE TABLE mamony_identity.recovery_notice_escape(id integer)",
			"SELECT * FROM mamony_identity.password_recovery_notices",
			"SELECT * FROM mamony_identity.password_change_notices",
			"SELECT * FROM mamony_identity.password_recovery_challenges",
			"SELECT * FROM mamony_identity.password_recovery_assurance_grants",
			"SELECT * FROM mamony_identity.password_verifiers",
			"SELECT * FROM mamony_identity.email_aliases",
			"SELECT * FROM mamony_identity.subjects",
			"SELECT * FROM mamony_identity.opaque_sessions",
			"SELECT * FROM mamony_identity.security_outbox",
			"SELECT * FROM mamony_identity.schema_migrations",
			"SELECT * FROM mamony_identity.migration_fingerprints",
			"UPDATE mamony_identity.password_recovery_notices SET status='ACCEPTED'",
			"SELECT * FROM mamony_identity.claim_password_change_notice('" + lease + "')",
			"SELECT * FROM mamony_identity.claim_password_recovery_delivery('" + lease + "')",
			"SELECT mamony_identity.redeem_password_recovery(decode(repeat('11',32),'hex'),'owner@example.test',decode(repeat('22',16),'hex'),decode(repeat('33',32),'hex'),'" + event + "')",
			"SET ROLE mamony_identity_migrator",
		} {
			out, err := invokePSQL(role, "-c", sql)
			if err == nil || !strings.Contains(out, "42501") {
				t.Fatalf("v22 notice role exceeded EXECUTE-only boundary: role=%s sql=%s out=%s err=%v", role, sql, out, err)
			}
		}
	}
	for _, tc := range []struct {
		role string
		sql  string
	}{
		{"TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL", claim},
		{"TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL", finalize},
		{"TEST_IDENTITY_RECOVERY_NOTICE_URL", "SELECT * FROM mamony_identity.password_recovery_notice_health()"},
		{"TEST_IDENTITY_SECURITY_NOTICE_URL", claim},
		{"TEST_IDENTITY_SECURITY_NOTICE_URL", finalize},
		{"TEST_IDENTITY_RECOVERY_ISSUER_URL", claim},
		{"TEST_IDENTITY_RECOVERY_REDEEMER_URL", finalize},
		{"TEST_IDENTITY_RUNTIME_URL", claim},
		{"TEST_PLATFORM_RUNTIME_URL", "SELECT * FROM mamony_identity.password_recovery_notice_health()"},
		{"TEST_IDENTITY_RECOVERY_NOTICE_URL", "SELECT mamony_identity.purge_old_password_recovery_notices()"},
		{"TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL", "SELECT mamony_identity.purge_old_password_recovery_notices()"},
	} {
		out, err := invokePSQL(tc.role, "-c", tc.sql)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("cross-purpose v22 EXECUTE succeeded: role=%s sql=%s out=%s err=%v",
				tc.role, tc.sql, out, err)
		}
	}
}

// TestPostgresIdentityZRecoveryNoticeWebhookRole proves that signed recovery
// receipts have a separate, EXECUTE-only SQL principal. Password-change
// sender/webhook and v22 recovery sender/monitor must never inherit it.
func TestPostgresIdentityZRecoveryNoticeWebhookRole(t *testing.T) {
	const valid = "SELECT mamony_identity.record_mailgun_recovery_notice_receipt(" +
		"decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex')," +
		"decode(repeat('33',32),'hex'),'DELIVERED',clock_timestamp())"
	out, err := invokePSQL("TEST_IDENTITY_RECOVERY_NOTICE_WEBHOOK_URL", "-c", valid)
	if err != nil || out != "t" {
		t.Fatalf("purpose-specific digest-only receipt recording denied: %s %v", out, err)
	}
	for _, stmt := range []string{
		"CREATE TABLE mamony_identity.recovery_webhook_escape (id integer)",
		"SELECT * FROM mamony_identity.password_recovery_notices",
		"SELECT * FROM mamony_identity.password_recovery_notice_mailgun_receipts",
		"SELECT * FROM mamony_identity.security_notice_mailgun_receipts",
		"SELECT * FROM mamony_identity.password_change_notices",
		"SELECT * FROM mamony_identity.password_verifiers",
		"SELECT * FROM mamony_identity.password_recovery_challenges",
		"SELECT * FROM mamony_identity.password_recovery_assurance_grants",
		"SELECT * FROM mamony_identity.email_aliases",
		"SELECT * FROM mamony_identity.subjects",
		"SELECT * FROM mamony_identity.opaque_sessions",
		"SELECT * FROM mamony_identity.security_outbox",
		"UPDATE mamony_identity.password_recovery_notices SET provider_state='DELIVERED'",
		"DELETE FROM mamony_identity.password_recovery_notice_mailgun_receipts",
		"SELECT * FROM mamony_identity.password_recovery_notice_health()",
		"SELECT * FROM mamony_identity.password_recovery_notice_delivery_health()",
		"SELECT mamony_identity.purge_old_recovery_notice_mailgun_receipts()",
		"SELECT mamony_identity.record_mailgun_security_notice_receipt(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),decode(repeat('33',32),'hex'),'DELIVERED',clock_timestamp())",
		"SELECT * FROM mamony_identity.claim_password_recovery_notice('lse_AAAAAAAAAAAAAAAAAAAAAA')",
		"SET ROLE mamony_identity_migrator",
	} {
		out, err := invokePSQL("TEST_IDENTITY_RECOVERY_NOTICE_WEBHOOK_URL", "-c", stmt)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("recovery receipt role escalated: query=%s out=%s err=%v", stmt, out, err)
		}
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL",
		"TEST_IDENTITY_NOTICE_WEBHOOK_URL",
		"TEST_IDENTITY_SECURITY_NOTICE_URL",
		"TEST_IDENTITY_RECOVERY_NOTICE_URL",
		"TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL",
		"TEST_IDENTITY_RECOVERY_ISSUER_URL",
		"TEST_IDENTITY_RECOVERY_REDEEMER_URL",
		"TEST_IDENTITY_RECOVERY_DELIVERY_URL",
		"TEST_PLATFORM_RUNTIME_URL",
	} {
		out, err := invokePSQL(role, "-c", valid)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("cross-purpose role recorded recovery webhook: role=%s out=%s err=%v", role, out, err)
		}
	}
	for _, sql := range []string{
		"SELECT * FROM mamony_identity.password_recovery_notice_health()",
		"SELECT * FROM mamony_identity.password_recovery_notice_delivery_health()",
	} {
		out, err := invokePSQL("TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL", "-c", sql)
		if err != nil || out == "" {
			t.Fatalf("read-only recovery monitor lost approved aggregate: sql=%s err=%v", sql, err)
		}
	}
}

// TestPostgresIdentityZRecoveryAssuranceRole proves v24's separate offline-
// proof verifier can EXECUTE only the challenge-bound approval transaction.
func TestPostgresIdentityZRecoveryAssuranceRole(t *testing.T) {
	const approve = "SELECT mamony_identity.approve_password_recovery_offline_code(" +
		"decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex')," +
		"'evt_AAAAAAAAAAAAAAAAAAAAAA')"
	out, err := invokePSQL("TEST_IDENTITY_RECOVERY_ASSURANCE_URL", "-c", approve)
	if err != nil || out != "f" {
		t.Fatalf("isolated assurance SQL principal cannot deny unknown proofs: out=%s err=%v", out, err)
	}
	for _, sql := range []string{
		"CREATE TABLE mamony_identity.recovery_assurance_escape(id integer)",
		"SELECT * FROM mamony_identity.password_recovery_offline_codes",
		"SELECT * FROM mamony_identity.password_recovery_assurance_grants",
		"SELECT * FROM mamony_identity.password_recovery_challenges",
		"SELECT * FROM mamony_identity.password_recovery_deliveries",
		"SELECT * FROM mamony_identity.password_recovery_notices",
		"SELECT * FROM mamony_identity.email_aliases",
		"SELECT * FROM mamony_identity.subjects",
		"SELECT * FROM mamony_identity.password_verifiers",
		"SELECT * FROM mamony_identity.opaque_sessions",
		"SELECT * FROM mamony_identity.security_outbox",
		"SELECT * FROM mamony_identity.migration_fingerprints",
		"DELETE FROM mamony_identity.password_recovery_offline_codes",
		"UPDATE mamony_identity.password_recovery_assurance_grants SET consumed_at=NULL",
		"INSERT INTO mamony_identity.password_recovery_offline_codes(code_digest,subject_id,enrolled_epoch,contact_version,enrolled_at) VALUES(decode(repeat('11',32),'hex'),'sub_AAAAAAAAAAAAAAAAAAAAAA',1,1,clock_timestamp())",
		"SELECT mamony_identity.redeem_password_recovery(decode(repeat('11',32),'hex'),'missing@example.test',decode(repeat('22',16),'hex'),decode(repeat('33',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')",
		"SELECT mamony_identity.issue_password_recovery('missing@example.test','rcv_AAAAAAAAAAAAAAAAAAAAAA',decode(repeat('11',32),'hex'),'testkey',decode(repeat('22',71),'hex'))",
		"SELECT * FROM mamony_identity.password_recovery_notice_health()",
		"SET ROLE mamony_identity_migrator",
	} {
		out, err := invokePSQL("TEST_IDENTITY_RECOVERY_ASSURANCE_URL", "-c", sql)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("offline recovery assurance role gained SQL authority: sql=%s out=%s err=%v", sql, out, err)
		}
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL",
		"TEST_IDENTITY_AUTHENTICATOR_URL",
		"TEST_IDENTITY_PASSWORD_CHANGE_URL",
		"TEST_IDENTITY_RECOVERY_ISSUER_URL",
		"TEST_IDENTITY_RECOVERY_DELIVERY_URL",
		"TEST_IDENTITY_RECOVERY_REDEEMER_URL",
		"TEST_IDENTITY_RECOVERY_NOTICE_URL",
		"TEST_IDENTITY_RECOVERY_NOTICE_WEBHOOK_URL",
		"TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL",
		"TEST_IDENTITY_SESSION_URL",
		"TEST_PLATFORM_RUNTIME_URL",
	} {
		out, err := invokePSQL(role, "-c", approve)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("unauthorized role approved recovery assurance: role=%s out=%s err=%v", role, out, err)
		}
	}
}

// TestPostgresIdentityZRecoveryCodeManagerRole enforces v25's EXECUTE-only
// enrollment/rotation principal and distinct one-use reauthentication purpose.
func TestPostgresIdentityZRecoveryCodeManagerRole(t *testing.T) {
	const issue = "SELECT mamony_identity.stage_recovery_code_rotation_grant(" +
		"'sub_AAAAAAAAAAAAAAAAAAAAAA',1,decode(repeat('11',32),'hex'))"
	const replace = "SELECT mamony_identity.replace_password_recovery_offline_codes(" +
		"decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex')," +
		"decode(repeat('33',32),'hex')," +
		"ARRAY[decode(repeat('44',32),'hex'),decode(repeat('55',32),'hex')," +
		"decode(repeat('66',32),'hex'),decode(repeat('77',32),'hex')," +
		"decode(repeat('88',32),'hex')],'evt_AAAAAAAAAAAAAAAAAAAAAA')"
	out, err := invokePSQL("TEST_IDENTITY_AUTH_GRANT_URL", "-c", issue)
	if err != nil || out != "f" {
		t.Fatalf("auth grant runtime cannot safely reject unknown recovery subject: %s %v", out, err)
	}
	out, err = invokePSQL("TEST_IDENTITY_RECOVERY_CODE_MANAGER_URL", "-c", replace)
	if err != nil || out != "f" {
		t.Fatalf("recovery manager cannot safely reject unknown session: %s %v", out, err)
	}
	for _, sql := range []string{
		"CREATE TABLE mamony_identity.offline_manager_escape(id integer)",
		"SELECT * FROM mamony_identity.password_recovery_offline_codes",
		"SELECT * FROM mamony_identity.password_recovery_code_enrollment_approvals",
		"SELECT * FROM mamony_identity.password_recovery_code_notices",
		"SELECT * FROM mamony_identity.password_login_grants",
		"SELECT * FROM mamony_identity.password_verifiers",
		"SELECT * FROM mamony_identity.password_recovery_assurance_grants",
		"SELECT * FROM mamony_identity.password_recovery_challenges",
		"SELECT * FROM mamony_identity.password_recovery_deliveries",
		"SELECT * FROM mamony_identity.subjects",
		"SELECT * FROM mamony_identity.email_aliases",
		"SELECT * FROM mamony_identity.opaque_sessions",
		"SELECT * FROM mamony_identity.security_outbox",
		"SELECT * FROM mamony_identity.migration_fingerprints",
		"UPDATE mamony_identity.password_recovery_offline_codes SET consumed_at=NULL",
		"DELETE FROM mamony_identity.password_recovery_code_notices",
		"INSERT INTO mamony_identity.password_recovery_code_enrollment_approvals(subject_id,subject_epoch,proof_digest,approved_at,expires_at) VALUES('sub_AAAAAAAAAAAAAAAAAAAAAA',1,decode(repeat('11',32),'hex'),clock_timestamp(),clock_timestamp()+interval '5 minutes')",
		issue,
		"SELECT mamony_identity.approve_password_recovery_offline_code(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')",
		"SELECT mamony_identity.stage_password_change_grant('sub_AAAAAAAAAAAAAAAAAAAAAA',1,decode(repeat('11',32),'hex'))",
		"SELECT mamony_identity.rotate_password_verifier(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),decode(repeat('33',16),'hex'),decode(repeat('44',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')",
		"SET ROLE mamony_identity_migrator",
	} {
		out, err := invokePSQL("TEST_IDENTITY_RECOVERY_CODE_MANAGER_URL", "-c", sql)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("recovery code manager exceeded EXECUTE-only role: sql=%s out=%s err=%v", sql, out, err)
		}
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL",
		"TEST_IDENTITY_AUTHENTICATOR_URL",
		"TEST_IDENTITY_PASSWORD_CHANGE_URL",
		"TEST_IDENTITY_RECOVERY_ISSUER_URL",
		"TEST_IDENTITY_RECOVERY_DELIVERY_URL",
		"TEST_IDENTITY_RECOVERY_REDEEMER_URL",
		"TEST_IDENTITY_RECOVERY_ASSURANCE_URL",
		"TEST_IDENTITY_RECOVERY_NOTICE_URL",
		"TEST_IDENTITY_RECOVERY_NOTICE_WEBHOOK_URL",
		"TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL",
		"TEST_IDENTITY_SESSION_URL",
		"TEST_IDENTITY_AUTH_GRANT_URL",
		"TEST_PLATFORM_RUNTIME_URL",
	} {
		out, err := invokePSQL(role, "-c", replace)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("different authority replaced offline code set: role=%s out=%s err=%v",
				role, out, err)
		}
	}
}

// TestPostgresIdentityZRecoveryCodeSecurityNoticeRoles proves the worker and
// count-only observer have independent EXECUTE grants, no row-level PII,
// and no authority to issue/reset password or offline recovery material.
func TestPostgresIdentityZRecoveryCodeSecurityNoticeRoles(t *testing.T) {
	const claim = "SELECT * FROM mamony_identity.claim_recovery_code_security_notice('lse_AAAAAAAAAAAAAAAAAAAAAA')"
	const monitor = "SELECT * FROM mamony_identity.recovery_code_security_notice_health()"
	out, err := invokePSQL("TEST_IDENTITY_RECOVERY_CODE_NOTICE_URL", "-c", claim)
	if err != nil {
		t.Fatalf("recovery-code sender cannot claim: %s %v", out, err)
	}
	out, err = invokePSQL("TEST_IDENTITY_RECOVERY_CODE_NOTICE_MONITOR_URL", "-c", monitor)
	if err != nil || out == "" {
		t.Fatalf("count-only recovery-code monitor denied: %s %v", out, err)
	}
	denied := []string{
		"SELECT * FROM mamony_identity.password_recovery_code_notices",
		"SELECT * FROM mamony_identity.password_recovery_offline_codes",
		"SELECT * FROM mamony_identity.password_recovery_code_enrollment_approvals",
		"SELECT * FROM mamony_identity.password_recovery_assurance_grants",
		"SELECT * FROM mamony_identity.password_recovery_challenges",
		"SELECT * FROM mamony_identity.password_recovery_notices",
		"SELECT * FROM mamony_identity.password_change_notices",
		"SELECT * FROM mamony_identity.security_outbox",
		"SELECT * FROM mamony_identity.password_verifiers",
		"SELECT * FROM mamony_identity.email_aliases",
		"SELECT * FROM mamony_identity.subjects",
		"SELECT * FROM mamony_identity.opaque_sessions",
		"CREATE TABLE mamony_identity.recovery_code_notice_escape(id integer)",
		"DELETE FROM mamony_identity.password_recovery_code_notices",
		"UPDATE mamony_identity.password_recovery_code_notices SET status='ACCEPTED'",
		"SELECT mamony_identity.replace_password_recovery_offline_codes(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),decode(repeat('33',32),'hex'),ARRAY[decode(repeat('44',32),'hex')],'evt_AAAAAAAAAAAAAAAAAAAAAA')",
		"SELECT mamony_identity.approve_password_recovery_offline_code(decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),'evt_AAAAAAAAAAAAAAAAAAAAAA')",
		"SELECT mamony_identity.claim_password_recovery_notice('lse_AAAAAAAAAAAAAAAAAAAAAA')",
		"SELECT mamony_identity.claim_password_change_notice('lse_AAAAAAAAAAAAAAAAAAAAAA')",
		"SELECT mamony_identity.recovery_code_security_notice_health()",
		"SELECT mamony_identity.purge_old_recovery_code_security_notices()",
		"SET ROLE mamony_identity_migrator",
	}
	for _, sql := range denied {
		out, err := invokePSQL("TEST_IDENTITY_RECOVERY_CODE_NOTICE_URL", "-c", sql)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("sender overprivileged: sql=%s out=%s err=%v", sql, out, err)
		}
	}
	for _, sql := range []string{
		claim,
		"SELECT mamony_identity.finalize_recovery_code_security_notice('evt_AAAAAAAAAAAAAAAAAAAAAA','lse_AAAAAAAAAAAAAAAAAAAAAA','TEMPORARY',NULL)",
		"SELECT * FROM mamony_identity.password_recovery_code_notices",
		"SELECT * FROM mamony_identity.security_outbox",
		"SELECT * FROM mamony_identity.subjects",
		"SELECT mamony_identity.password_recovery_notice_health()",
		"SELECT mamony_identity.purge_old_recovery_code_security_notices()",
		"SET ROLE mamony_identity_migrator",
	} {
		out, err := invokePSQL("TEST_IDENTITY_RECOVERY_CODE_NOTICE_MONITOR_URL", "-c", sql)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("recovery code observer overprivileged: sql=%s out=%s err=%v", sql, out, err)
		}
	}
	for _, role := range []string{
		"TEST_IDENTITY_RUNTIME_URL",
		"TEST_IDENTITY_AUTHENTICATOR_URL",
		"TEST_IDENTITY_AUTH_GRANT_URL",
		"TEST_IDENTITY_PASSWORD_CHANGE_URL",
		"TEST_IDENTITY_RECOVERY_ISSUER_URL",
		"TEST_IDENTITY_RECOVERY_DELIVERY_URL",
		"TEST_IDENTITY_RECOVERY_REDEEMER_URL",
		"TEST_IDENTITY_RECOVERY_ASSURANCE_URL",
		"TEST_IDENTITY_RECOVERY_CODE_MANAGER_URL",
		"TEST_IDENTITY_RECOVERY_NOTICE_URL",
		"TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL",
		"TEST_IDENTITY_RECOVERY_NOTICE_WEBHOOK_URL",
		"TEST_IDENTITY_SECURITY_NOTICE_URL",
		"TEST_IDENTITY_SESSION_URL",
		"TEST_PLATFORM_RUNTIME_URL",
	} {
		out, err := invokePSQL(role, "-c", claim)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("unrelated role could claim code-change alert: role=%s out=%s err=%v", role, out, err)
		}
		out, err = invokePSQL(role, "-c", monitor)
		if err == nil || !strings.Contains(out, "42501") {
			t.Fatalf("unrelated role could monitor code-change alerts: role=%s out=%s err=%v", role, out, err)
		}
	}
}
