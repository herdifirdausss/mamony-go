//go:build integration

package identity

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/credential"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityRH03RecoveredPasswordBinding is invoked only by the dedicated
// network-none disposable execution lab. Synthetic bearer values remain in
// this test process. The existing paused native recovery target is not used
// or activated. This case proves logical recovery enforcement, not native
// application execution or production access-policy restoration.
func TestIdentityRH03RecoveredPasswordBinding(t *testing.T) {
	if os.Getenv("MAMONY_RH03_EXECUTION_FIXTURE") != "true" {
		t.Skip("dedicated RH-03 execution fixture only")
	}
	const sourceDB = "mamony_test_rh03_exec_source"
	const targetDB = "mamony_test_rh03_exec_target"
	const adminRole = "mamony_test_admin"
	const adminPassword = "local-test-rh03-exec-admin"
	dsn := func(db, role, password string) string {
		u := url.URL{
			Scheme: "postgres", Host: "127.0.0.1:5432", Path: "/" + db,
			User: url.UserPassword(role, password), RawQuery: "sslmode=disable",
		}
		return u.String()
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" ||
		os.Getenv("TEST_DATABASE_ADMIN_URL") != dsn(sourceDB, adminRole, adminPassword) {
		t.Fatal("unexpected disposable execution fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	connectAdmin := func(db string) *pgx.Conn {
		t.Helper()
		c, err := pgx.Connect(ctx, dsn(db, adminRole, adminPassword))
		if err != nil {
			t.Fatal("fixture admin connection failed")
		}
		t.Cleanup(func() { _ = c.Close(context.Background()) })
		var correct bool
		if err := c.QueryRow(ctx, "SELECT current_database()=$1 AND current_user=$2", db, adminRole).Scan(&correct); err != nil || !correct {
			t.Fatal("unexpected fixture database or role")
		}
		return c
	}
	source := connectAdmin(sourceDB)
	var version int
	if err := source.QueryRow(ctx, "SELECT max(version) FROM mamony_identity.migration_fingerprints").Scan(&version); err != nil || version != 34 {
		t.Fatal("expected approved Identity migration fixture missing")
	}

	open := func(db, role, password string) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{
			DSN: dsn(db, role, password), RuntimeRole: role, MaxConns: 2,
		})
		if err != nil {
			t.Fatal("isolated fixture runtime pool refused")
		}
		t.Cleanup(p.Close)
		return p
	}
	kdf, err := credential.NewPasswordHasher(1)
	if err != nil {
		t.Fatal("fixture password hasher unavailable")
	}
	type authorities struct {
		flow    *usecase.PasswordLoginFlow
		login   *usecase.SessionLogin
		session *usecase.OpaqueSessionAuthority
		change  *usecase.AuthenticatedPasswordChange
	}
	build := func(db string) authorities {
		t.Helper()
		reg := open(db, "mamony_identity_runtime", "local-test-identity-runtime")
		auth := open(db, "mamony_identity_authenticator_runtime", "local-test-identity-authenticator")
		grant := open(db, "mamony_identity_auth_grant_runtime", "local-test-identity-auth-grant")
		session := open(db, "mamony_identity_session_runtime", "local-test-identity-session")
		change := open(db, "mamony_identity_password_change_runtime", "local-test-identity-password-change")

		reader, err := identitypg.NewAuthenticationStore(auth)
		if err != nil {
			t.Fatal("authentication store unavailable")
		}
		password, err := usecase.NewPasswordAuthentication(reader, kdf)
		if err != nil {
			t.Fatal("password authority unavailable")
		}
		admission, err := identitypg.NewAdmissionStore(reg)
		if err != nil {
			t.Fatal("admission store unavailable")
		}
		gate, err := usecase.NewLoginAdmission(admission, bytes.Repeat([]byte{0xe3}, 32))
		if err != nil {
			t.Fatal("admission authority unavailable")
		}
		flow, err := usecase.NewPasswordLoginFlow(gate, password)
		if err != nil {
			t.Fatal("password login flow unavailable")
		}
		grantStore, err := identitypg.NewAuthenticationGrantStore(grant)
		if err != nil {
			t.Fatal("grant store unavailable")
		}
		grants, err := usecase.NewAuthenticationGrantAuthority(grantStore)
		if err != nil {
			t.Fatal("grant authority unavailable")
		}
		sessionStore, err := identitypg.NewOpaqueSessionStore(session)
		if err != nil {
			t.Fatal("session store unavailable")
		}
		sessions, err := usecase.NewOpaqueSessionAuthority(sessionStore)
		if err != nil {
			t.Fatal("session authority unavailable")
		}
		login, err := usecase.NewSessionLogin(flow, grants, sessions)
		if err != nil {
			t.Fatal("session login unavailable")
		}
		rotation, err := identitypg.NewPasswordChangeStore(change)
		if err != nil {
			t.Fatal("password rotation store unavailable")
		}
		changer, err := usecase.NewAuthenticatedPasswordChange(sessions, flow, grants, kdf, rotation,
			domain.PasswordPolicy{IsBlocked: func(_, _ string) bool { return false }})
		if err != nil {
			t.Fatal("password change authority unavailable")
		}
		return authorities{flow: flow, login: login, session: sessions, change: changer}
	}

	// Identical synthetic subject provisioning pattern to the existing
	// authenticated password-change integration test; no production path.
	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "rh03_")) + "@example.test"
	oldPassword := "synthetic old password for rh03 fixture"
	newPassword := "synthetic new password for rh03 fixture"
	verifier, err := kdf.Hash(ctx, oldPassword)
	if err != nil {
		t.Fatal("fixture password hash failed")
	}
	if _, err := source.Exec(ctx, "INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES($1,clock_timestamp())", subject); err != nil {
		t.Fatal("fixture subject creation failed")
	}
	if _, err := source.Exec(ctx, "INSERT INTO mamony_identity.email_aliases(canonical_email,subject_id,verified_at,created_at) VALUES($1,$2,clock_timestamp(),clock_timestamp())", email, subject); err != nil {
		t.Fatal("fixture contact creation failed")
	}
	if _, err := source.Exec(ctx, "INSERT INTO mamony_identity.password_verifiers(subject_id,kdf_version,memory_kib,iterations,parallelism,salt,digest,created_at) VALUES($1,1,65536,3,1,$2,$3,clock_timestamp())", subject, verifier.Salt, verifier.Digest); err != nil {
		t.Fatal("fixture verifier creation failed")
	}
	if _, err := source.Exec(ctx, "UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
		t.Fatal("fixture subject activation failed")
	}

	src := build(sourceDB)
	peer := netip.MustParseAddr("198.51.100.192")
	old, err := src.login.Authenticate(ctx, email, oldPassword, peer)
	if err != nil {
		t.Fatal("source positive login failed")
	}
	if who, err := src.session.Resolve(ctx, old.ForTrustedTransport()); err != nil || who != subject {
		t.Fatal("source session positive control failed")
	}
	if err := src.change.Change(ctx, old.ForTrustedTransport(), email, oldPassword, newPassword, peer); err != nil {
		t.Fatal("source password change failed")
	}
	fresh, err := src.login.Authenticate(ctx, email, newPassword, peer)
	if err != nil {
		t.Fatal("source current credential positive control failed")
	}
	if who, err := src.session.Resolve(ctx, fresh.ForTrustedTransport()); err != nil || who != subject {
		t.Fatal("source current session positive control failed")
	}
	if who, err := src.session.Resolve(ctx, old.ForTrustedTransport()); who != "" || !errors.Is(err, usecase.ErrSessionInvalid) {
		t.Fatal("source prior session not invalidated")
	}

	// Compare complete selected durable rows, including original event IDs
	// and notice rows; do not export their contents or digest to logs.
	snapshot := func(c *pgx.Conn) string {
		t.Helper()
		var digest string
		err := c.QueryRow(ctx, `SELECT encode(sha256(convert_to(jsonb_build_array(
			(SELECT to_jsonb(s) FROM mamony_identity.subjects s WHERE s.subject_id=$1),
			(SELECT to_jsonb(p) FROM mamony_identity.password_verifiers p WHERE p.subject_id=$1),
			(SELECT to_jsonb(a) FROM mamony_identity.email_aliases a WHERE a.subject_id=$1),
			(SELECT jsonb_agg(to_jsonb(o) ORDER BY o.event_id) FROM mamony_identity.security_outbox o WHERE o.subject_id=$1),
			(SELECT jsonb_agg(to_jsonb(n) ORDER BY n.event_id) FROM mamony_identity.password_change_notices n WHERE n.subject_id=$1)
		)::text,'UTF8')),'hex')`, subject).Scan(&digest)
		if err != nil || len(digest) != 64 {
			t.Fatal("fixture durable snapshot failed")
		}
		return digest
	}
	var notices int
	if err := source.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.password_change_notices n
		JOIN mamony_identity.security_outbox o USING(event_id)
		WHERE n.subject_id=$1 AND o.subject_id=$1 AND n.recipient=$2
		AND o.event_type='PASSWORD_CHANGED' AND n.status='PENDING'`, subject, email).Scan(&notices); err != nil || notices != 1 {
		t.Fatal("source transition notice witness missing")
	}
	expected := snapshot(source)

	// The native tools execute only inside the dedicated network-none
	// container. Fixed database names; no shell, no credential artifacts
	// leave the test process/container. Preserve original fixture ACLs.
	archive := filepath.Join(t.TempDir(), "fixture.dump")
	run := func(name string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}
		if err := cmd.Run(); err != nil {
			t.Fatalf("isolated logical recovery command failed: %s", name)
		}
	}
	run("pg_dump", "--host=/var/run/postgresql", "--username="+adminRole, "--no-password",
		"--format=custom", "--file="+archive, "--dbname="+sourceDB)
	if _, err := source.Exec(ctx, "CREATE DATABASE mamony_test_rh03_exec_target"); err != nil {
		t.Fatal("distinct execution target creation failed")
	}
	run("pg_restore", "--host=/var/run/postgresql", "--username="+adminRole, "--no-password",
		"--single-transaction", "--exit-on-error", "--dbname="+targetDB, archive)
	target := connectAdmin(targetDB)
	if snapshot(source) != expected || snapshot(target) != expected {
		t.Fatal("recovered durable binding or original obligations differ")
	}

	dst := build(targetDB)
	// The existing admission limits are unchanged: three source admissions,
	// then two target admissions within the same recovered budget window.
	if who, err := dst.session.Resolve(ctx, fresh.ForTrustedTransport()); err != nil || who != subject {
		t.Fatal("recovered current session positive control failed")
	}
	if who, err := dst.session.Resolve(ctx, old.ForTrustedTransport()); who != "" || !errors.Is(err, usecase.ErrSessionInvalid) {
		t.Fatal("recovered prior session not rejected by application")
	}
	if who, err := dst.flow.Authenticate(ctx, email, oldPassword, peer); who != "" || !errors.Is(err, usecase.ErrPasswordAuthenticationDenied) {
		t.Fatal("recovered superseded password not rejected by application")
	}
	if who, err := dst.flow.Authenticate(ctx, email, newPassword, peer); err != nil || who != subject {
		t.Fatal("recovered current password positive control failed")
	}
	if snapshot(target) != expected || snapshot(source) != expected {
		t.Fatal("application assertions changed durable binding or original obligations")
	}
	t.Log("RH-03b3a PASS: separate synthetic logical recovery preserves durable binding and rejects prior session and superseded password with current positive controls")
}
