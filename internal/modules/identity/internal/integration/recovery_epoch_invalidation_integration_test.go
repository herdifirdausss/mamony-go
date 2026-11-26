//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/integration/internal/fixture"
	"github.com/jackc/pgx/v5"
)

// TestIdentityZRecoveryEpochInvalidationMechanism tests the existing local
// epoch fence using a new synthetic subject in the disposable integration DB.
// It deliberately does not implement recovery, restore, or release authority.
func TestIdentityZRecoveryEpochInvalidationMechanism(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	adminDSN := fixture.GuardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	grantDSN := fixture.GuardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := fixture.GuardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	changeDSN := fixture.GuardedDB(t, "TEST_IDENTITY_PASSWORD_CHANGE_URL", "mamony_identity_password_change_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, dsn := range []string{grantDSN, sessionDSN, changeDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("epoch invalidation fixture requires one disposable database")
		}
	}
	connect := func(dsn string) *pgx.Conn {
		t.Helper()
		c, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Fatal("synthetic database connection failed")
		}
		t.Cleanup(func() { _ = c.Close(context.Background()) })
		return c
	}
	admin, grant, session, change := connect(adminDSN), connect(grantDSN), connect(sessionDSN), connect(changeDSN)
	subject := fixture.RandomID(t, "sub_")
	email := strings.ToLower(fixture.RandomID(t, "recovery_epoch_")) + "@example.test"
	if _, err := admin.Exec(ctx, "INSERT INTO mamony_identity.subjects (subject_id,created_at) VALUES ($1,clock_timestamp())", subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "INSERT INTO mamony_identity.email_aliases (canonical_email,subject_id,verified_at,created_at) VALUES ($1,$2,clock_timestamp(),clock_timestamp())", email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_verifiers
		(subject_id,kdf_version,memory_kib,iterations,parallelism,salt,digest,created_at)
		VALUES ($1,1,65536,3,1,decode(repeat('11',16),'hex'),decode(repeat('22',32),'hex'),clock_timestamp())`, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	epoch := func() int64 {
		t.Helper()
		var value int64
		if err := admin.QueryRow(ctx, "SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", subject).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	digest := func(label string) []byte {
		d := sha256.Sum256([]byte("synthetic:recovery-epoch:" + subject + ":" + label))
		return d[:]
	}
	stage := func(label string, expected int64) ([]byte, bool) {
		t.Helper()
		d := digest(label)
		var ok bool
		if err := grant.QueryRow(ctx, "SELECT mamony_identity.stage_password_auth_grant($1,$2,$3)", subject, expected, d).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return d, ok
	}
	mint := func(label string, proof []byte) ([]byte, bool) {
		t.Helper()
		d := digest(label)
		var ok bool
		if err := session.QueryRow(ctx, "SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)", proof, d, fixture.RandomID(t, "evt_")).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return d, ok
	}
	resolve := func(d []byte) string {
		t.Helper()
		var owner string
		if err := session.QueryRow(ctx, "SELECT coalesce(mamony_identity.resolve_opaque_session($1),'')", d).Scan(&owner); err != nil {
			t.Fatal(err)
		}
		return owner
	}
	initialEpoch := epoch()
	initialGrant, ok := stage("initial-grant", initialEpoch)
	if !ok {
		t.Fatal("baseline grant denied")
	}
	oldSession, ok := mint("initial-session", initialGrant)
	if !ok || resolve(oldSession) != subject {
		t.Fatal("baseline session denied")
	}
	oldGrant, ok := stage("outstanding-grant", initialEpoch)
	if !ok {
		t.Fatal("baseline outstanding grant denied")
	}

	// An administrator-only, subject-scoped fixture mutation, not a
	// production recovery operation. Rollback must retain the original epoch.
	tx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, "UPDATE mamony_identity.subjects SET session_epoch=session_epoch+1 WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if epoch() != initialEpoch || resolve(oldSession) != subject {
		t.Fatal("rolled-back epoch mutation invalidated valid session")
	}
	// Establish a separate usable grant after rollback as a positive control.
	rollbackGrant, ok := stage("rollback-control-grant", initialEpoch)
	if !ok {
		t.Fatal("rollback control grant denied")
	}
	rollbackSession, ok := mint("rollback-control-session", rollbackGrant)
	if !ok || resolve(rollbackSession) != subject {
		t.Fatal("rollback control session denied")
	}
	stageChange := func(label string, expected int64) ([]byte, bool) {
		t.Helper()
		d := digest(label)
		var allowed bool
		if err := grant.QueryRow(ctx, "SELECT mamony_identity.stage_password_change_grant($1,$2,$3)", subject, expected, d).Scan(&allowed); err != nil {
			t.Fatal(err)
		}
		return d, allowed
	}
	oldChange, ok := stageChange("old-change-grant", initialEpoch)
	if !ok {
		t.Fatal("baseline password-change grant denied")
	}
	tag, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET session_epoch=session_epoch+1 WHERE subject_id=$1", subject)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatal("scoped fixture epoch mutation failed")
	}
	if epoch() != initialEpoch+1 || resolve(oldSession) != "" || resolve(rollbackSession) != "" {
		t.Fatal("committed epoch mutation retained old session")
	}
	if _, ok := stage("stale-stage", initialEpoch); ok {
		t.Fatal("old epoch allowed new grant")
	}
	if _, ok := mint("stale-mint", oldGrant); ok {
		t.Fatal("old outstanding grant minted session")
	}
	var outstanding, consumed, sessions, events int
	if err := admin.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM mamony_identity.password_login_grants WHERE subject_id=$1 AND grant_digest=$2 AND consumed_at IS NULL),
		(SELECT count(*) FROM mamony_identity.password_login_grants WHERE subject_id=$1 AND consumed_at IS NOT NULL),
		(SELECT count(*) FROM mamony_identity.opaque_sessions WHERE subject_id=$1),
		(SELECT count(*) FROM mamony_identity.security_outbox WHERE subject_id=$1 AND event_type='SESSION_CREATED')`,
		subject, oldGrant).Scan(&outstanding, &consumed, &sessions, &events); err != nil {
		t.Fatal(err)
	}
	if outstanding != 1 || consumed != 2 || sessions != 2 || events != 2 {
		t.Fatal("denial altered proof consumption or session/audit history")
	}
	// Positive control: the synthetic authority is assumed current solely
	// for this mechanism test; no recovery release or password proof claimed.
	freshGrant, ok := stage("fresh-grant", epoch())
	if !ok {
		t.Fatal("fresh fixture grant denied")
	}
	freshSession, ok := mint("fresh-session", freshGrant)
	if !ok || resolve(freshSession) != subject || resolve(oldSession) != "" {
		t.Fatal("fresh fixture session or old-session denial failed")
	}
	// A fresh session isolates the stale change-grant epoch as the reason
	// for denial. Verify the old grant has not expired before the attempt.
	var usable bool
	if err := admin.QueryRow(ctx, `SELECT consumed_at IS NULL AND expires_at>clock_timestamp()
		AND purpose='PASSWORD_CHANGE' AND subject_epoch=$2
		FROM mamony_identity.password_login_grants WHERE grant_digest=$1`,
		oldChange, initialEpoch).Scan(&usable); err != nil || !usable {
		t.Fatal("old change-grant fixture is not live at the stale-epoch check")
	}
	if _, ok := stageChange("stale-change-stage", initialEpoch); ok {
		t.Fatal("old epoch staged a password-change grant")
	}
	newSalt := digest("new-salt")[:16]
	newVerifier := digest("new-verifier")
	var allowed bool
	if err := change.QueryRow(ctx, "SELECT mamony_identity.rotate_password_verifier($1,$2,$3,$4,$5)",
		freshSession, oldChange, newSalt, newVerifier, fixture.RandomID(t, "evt_")).Scan(&allowed); err != nil || allowed {
		t.Fatal("old change grant was not cleanly denied with fresh session")
	}
	// A fresh change grant succeeds inside a transaction; roll back so no
	// test notification remains queued for other fixtures.
	freshChange, ok := stageChange("fresh-change-grant", epoch())
	if !ok {
		t.Fatal("fresh change grant denied")
	}
	changeTx, err := change.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = changeTx.Rollback(context.Background()) }()
	if err := changeTx.QueryRow(ctx, "SELECT mamony_identity.rotate_password_verifier($1,$2,$3,$4,$5)",
		freshSession, freshChange, newSalt, newVerifier, fixture.RandomID(t, "evt_")).Scan(&allowed); err != nil || !allowed {
		t.Fatal("fresh change-grant positive control failed")
	}
	if err := changeTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var unchanged bool
	if err := admin.QueryRow(ctx, `SELECT
		(SELECT salt=decode(repeat('11',16),'hex') AND digest=decode(repeat('22',32),'hex')
		 FROM mamony_identity.password_verifiers WHERE subject_id=$1)
		AND (SELECT count(*)=2 FROM mamony_identity.password_login_grants
		 WHERE subject_id=$1 AND grant_digest IN ($2,$3) AND consumed_at IS NULL)
		AND NOT EXISTS (SELECT 1 FROM mamony_identity.security_outbox WHERE subject_id=$1 AND event_type='PASSWORD_CHANGED')
		AND NOT EXISTS (SELECT 1 FROM mamony_identity.password_change_notices WHERE subject_id=$1)`,
		subject, oldChange, freshChange).Scan(&unchanged); err != nil || !unchanged {
		t.Fatal("denial or rollback changed credential, grant consumption or notice history")
	}
	if epoch() != initialEpoch+1 || resolve(freshSession) != subject || resolve(oldSession) != "" {
		t.Fatal("change-grant check altered epoch or session authority")
	}
	t.Log("IRF-CHANGE MECHANISM PASS: live stale-epoch change grant denied with fresh session; fresh change positive control rolled back; verifier/audit/notice unchanged")
	t.Log("IRF-EPOCH MECHANISM PASS: rollback preserves session; committed local epoch denies old sessions/grant without fabricated consumption; fresh synthetic authority works; restore/currentness NOT RUN")
}
