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

// TestIdentityZAuthMaterialEpochFence exercises actual v14 DDL and v11/v13
// restricted grant/session functions. Synthetic KDF rows and privileged
// changes prove transactional epoch behavior, NOT a public reset flow.
func TestIdentityZAuthMaterialEpochFence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 95*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	grantDSN := guardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, raw := range []string{grantDSN, sessionDSN} {
		u, _ := url.Parse(raw)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("epoch fence requires one disposable database")
		}
	}
	open := func(dsn string) *pgx.Conn {
		t.Helper()
		conn, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close(context.Background()) })
		return conn
	}
	admin, changing, grant, session := open(adminDSN), open(adminDSN), open(grantDSN), open(sessionDSN)
	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "epoch_")) + "@example.test"
	if _, err := admin.Exec(ctx, "INSERT INTO mamony_identity.subjects (subject_id,created_at) VALUES ($1,clock_timestamp())", subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.email_aliases (canonical_email,subject_id,verified_at,created_at) VALUES ($1,$2,clock_timestamp(),clock_timestamp())",
		email, subject); err != nil {
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
	mintGrant := func(label string, expected int64) ([]byte, bool) {
		t.Helper()
		digest := sha256.Sum256([]byte("synthetic:epoch:grant:" + subject + ":" + label))
		var allowed bool
		err := grant.QueryRow(ctx,
			"SELECT mamony_identity.stage_password_auth_grant($1,$2,$3)",
			subject, expected, digest[:]).Scan(&allowed)
		if err != nil {
			t.Fatalf("grant stage %s unavailable: %v", label, err)
		}
		return digest[:], allowed
	}
	mintSession := func(label string, proof []byte) ([]byte, bool) {
		t.Helper()
		digest := sha256.Sum256([]byte("synthetic:epoch:session:" + subject + ":" + label))
		var allowed bool
		err := session.QueryRow(ctx,
			"SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
			proof, digest[:], randomSyntheticID(t, "evt_")).Scan(&allowed)
		if err != nil {
			t.Fatalf("session issue %s unavailable: %v", label, err)
		}
		return digest[:], allowed
	}
	issue := func(label string) []byte {
		t.Helper()
		proof, allowed := mintGrant(label, epoch())
		if !allowed {
			t.Fatalf("cannot stage eligible proof %s", label)
		}
		digest, issued := mintSession(label, proof)
		if !issued {
			t.Fatalf("cannot mint eligible session %s", label)
		}
		return digest
	}
	resolve := func(digest []byte) string {
		t.Helper()
		var value string
		if err := session.QueryRow(ctx,
			"SELECT coalesce(mamony_identity.resolve_opaque_session($1),'')",
			digest).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	count := func() int {
		t.Helper()
		var value int
		if err := admin.QueryRow(ctx,
			"SELECT count(*) FROM mamony_identity.security_outbox WHERE subject_id=$1 AND event_type='SESSION_CREATED'",
			subject).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	initial := issue("initial")
	firstEpoch := epoch()
	if resolve(initial) != subject {
		t.Fatal("baseline active session not valid")
	}
	// A no-op UPDATE of verifier bytes must not log every session out.
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.password_verifiers SET digest=digest WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	if epoch() != firstEpoch || resolve(initial) != subject {
		t.Fatal("no-op verifier UPDATE incorrectly revoked a session")
	}
	// Database trigger and credential mutation must roll back together.
	tx, err := changing.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE mamony_identity.password_verifiers
		SET digest=decode(repeat('33',32),'hex') WHERE subject_id=$1`, subject); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	var inTx int64
	if err := tx.QueryRow(ctx, "SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", subject).Scan(&inTx); err != nil || inTx != firstEpoch+1 {
		_ = tx.Rollback(context.Background())
		t.Fatalf("password mutation did not update epoch in same transaction: %d %v", inTx, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if epoch() != firstEpoch || resolve(initial) != subject {
		t.Fatal("rollback permanently revoked a valid bearer")
	}
	// A proof staged under the old epoch becomes invalid after commit.
	oldProof, ok := mintGrant("pre-rotation", firstEpoch)
	if !ok {
		t.Fatal("pre-rotation proof was denied")
	}
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_verifiers
		SET digest=decode(repeat('44',32),'hex') WHERE subject_id=$1`, subject); err != nil {
		t.Fatal(err)
	}
	rotatedEpoch := epoch()
	if rotatedEpoch != firstEpoch+1 || resolve(initial) != "" {
		t.Fatal("password change did not invalidate old session")
	}
	if _, granted := mintGrant("stale-verified-password", firstEpoch); granted {
		t.Fatal("stale Argon2id snapshot minted new-epoch grant")
	}
	if _, issued := mintSession("old-proof", oldProof); issued {
		t.Fatal("pre-rotation grant resurrected after password update")
	}
	newBearer := issue("new-password")
	if resolve(newBearer) != subject {
		t.Fatal("new epoch could not mint after credential rotation")
	}

	// A contact that is no longer verified immediately invalidates sessions;
	// restoring verification never revives an older epoch.
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.email_aliases SET verified_at=NULL WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	if epoch() != rotatedEpoch+1 || resolve(newBearer) != "" {
		t.Fatal("email verification removal retained old session")
	}
	if _, allowed := mintGrant("unverified-contact", epoch()); allowed {
		t.Fatal("unverified contact issued grant")
	}
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_aliases
		SET verified_at=clock_timestamp() WHERE subject_id=$1`, subject); err != nil {
		t.Fatal(err)
	}
	if resolve(newBearer) != "" {
		t.Fatal("reverified contact resurrected earlier session")
	}
	restored := issue("restored-contact")
	newEmail := strings.ToLower(randomSyntheticID(t, "changed_")) + "@example.test"
	beforeContactEpoch := epoch()
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.email_aliases SET canonical_email=$1 WHERE subject_id=$2", newEmail, subject); err != nil {
		t.Fatal(err)
	}
	if epoch() != beforeContactEpoch+1 || resolve(restored) != "" {
		t.Fatal("canonical login alias change retained old session")
	}
	afterAlias := issue("new-contact")
	if resolve(afterAlias) != subject {
		t.Fatal("new contact cannot have new-epoch session")
	}

	// Deterministic TOCTOU proof: a password update holds the subject lock.
	// Grant stage using an already-compared old epoch must wait, then DENY
	// after password mutation commits (not silently use the new epoch).
	verifiedEpoch := epoch()
	var changePID, grantPID int32
	if err := changing.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&changePID); err != nil {
		t.Fatal(err)
	}
	if err := grant.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&grantPID); err != nil {
		t.Fatal(err)
	}
	tx, err = changing.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx,
		"SELECT 1 FROM mamony_identity.subjects WHERE subject_id=$1 FOR UPDATE", subject); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE mamony_identity.password_verifiers
		SET salt=decode(repeat('55',16),'hex') WHERE subject_id=$1`, subject); err != nil {
		t.Fatal(err)
	}
	type result struct { allowed bool; err error }
	stageResult := make(chan result, 1)
	staleDigest := sha256.Sum256([]byte("synthetic:epoch:staged-after-lock:" + subject))
	go func() {
		var allowed bool
		err := grant.QueryRow(ctx,
			"SELECT mamony_identity.stage_password_auth_grant($1,$2,$3)",
			subject, verifiedEpoch, staleDigest[:]).Scan(&allowed)
		stageResult <- result{allowed, err}
	}()
	waitForSessionLock(t, ctx, admin, grantPID, changePID)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-stageResult:
		if r.err != nil || r.allowed {
			t.Fatalf("stale proof won epoch race after commit: %+v", r)
		}
	case <-ctx.Done():
		t.Fatal("grant stage stuck after password rotation")
	}
	if epoch() != verifiedEpoch+1 || resolve(afterAlias) != "" {
		t.Fatal("concurrent password update did not revoke old bearer")
	}
	if count() != 4 {
		t.Fatalf("wrong mandatory session creation audit count after denied race: %d", count())
	}

	// Removing the credential row also invalidates all previous sessions.
	latest := issue("before-verifier-delete")
	beforeDelete := epoch()
	if _, err := admin.Exec(ctx,
		"DELETE FROM mamony_identity.password_verifiers WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	if epoch() != beforeDelete+1 || resolve(latest) != "" {
		t.Fatal("deleted password verifier retained earlier bearer")
	}
	// Deleting the email alias is also an epoch-changing operation, even
	// without a password or active session; it never resurrects past epochs.
	beforeDelete = epoch()
	if _, err := admin.Exec(ctx,
		"DELETE FROM mamony_identity.email_aliases WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	if epoch() != beforeDelete+1 {
		t.Fatal("deleted login alias did not advance epoch")
	}
}
