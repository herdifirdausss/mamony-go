//go:build integration

package identity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/credential"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

func TestIdentityZPasswordRecoveryRedemption(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	issuerDSN := guardedDB(t, "TEST_IDENTITY_RECOVERY_ISSUER_URL", "mamony_identity_recovery_issuer_runtime")
	redeemerDSN := guardedDB(t, "TEST_IDENTITY_RECOVERY_REDEEMER_URL", "mamony_identity_recovery_redeemer_runtime")
	grantDSN := guardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	root, _ := url.Parse(adminDSN)
	for _, raw := range []string{issuerDSN, redeemerDSN, grantDSN, sessionDSN} {
		parsed, _ := url.Parse(raw)
		if parsed.Host != root.Host || parsed.Path != root.Path {
			t.Fatal("recovery redemption requires one disposable DB")
		}
	}
	connect := func(dsn string) *pgx.Conn {
		t.Helper()
		conn, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close(context.Background()) })
		return conn
	}
	admin, grant, session := connect(adminDSN), connect(grantDSN), connect(sessionDSN)
	pool := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: 4})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	issuer, err := identitypg.NewRecoveryIssuerStore(pool(issuerDSN, "mamony_identity_recovery_issuer_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := identitypg.NewRecoveryRedemptionStore(pool(redeemerDSN, "mamony_identity_recovery_redeemer_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	hasher, err := credential.NewPasswordHasher(2)
	if err != nil {
		t.Fatal(err)
	}
	policy := domain.PasswordPolicy{IsBlocked: func(value, email string) bool {
		return value == "synthetic breached password not acceptable" || strings.Contains(value, email)
	}}
	redeemer, err := usecase.NewPasswordRecoveryRedemption(store, hasher, policy)
	if err != nil {
		t.Fatal(err)
	}

	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "reset_")) + "@example.test"
	old, err := hasher.Hash(ctx, "synthetic initial password for recovery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES($1,clock_timestamp())", subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.email_aliases
	(canonical_email,subject_id,verified_at,created_at)
	VALUES($1,$2,clock_timestamp(),clock_timestamp())`, email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_verifiers
	(subject_id,kdf_version,memory_kib,iterations,parallelism,salt,digest,created_at)
	VALUES($1,1,65536,3,1,$2,$3,clock_timestamp())`, subject, old.Salt, old.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	var epoch int64
	if err := admin.QueryRow(ctx, "SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", subject).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	proof, err := usecase.NewPasswordRecoveryProof()
	if err != nil {
		t.Fatal(err)
	}
	challenge := randomSyntheticID(t, "rcv_")
	issued, err := issuer.Issue(ctx, usecase.RecoveryIssue{
		CanonicalEmail: email, ChallengeID: challenge, TokenDigest: proof.Digest(),
		KeyID: "synthetic-recovery", SealedToken: bytes.Repeat([]byte{0x4c}, 71),
	})
	if err != nil || !issued {
		t.Fatalf("recovery challenge issue failed: %v %t", err, issued)
	}
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		conn, err := pgx.Connect(cleanupCtx, adminDSN)
		if err != nil {
			t.Errorf("cleanup: %v", err)
			return
		}
		defer func() { _ = conn.Close(context.Background()) }()
		if _, err := conn.Exec(cleanupCtx, `UPDATE mamony_identity.password_recovery_deliveries
		SET status='DEAD',lease_id=NULL,leased_until=NULL WHERE challenge_id=$1 AND status='PENDING'`,
			challenge); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	next := "synthetic new recovered password credential"
	if err := redeemer.Redeem(ctx, proof.ForTrustedDelivery(), email, next); !errors.Is(err, usecase.ErrPasswordRecoveryDenied) {
		t.Fatal("recovery bearer bypassed missing independent assurance")
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_recovery_assurance_grants
	(subject_id,subject_epoch,approved_at,expires_at)
	VALUES($1,$2,clock_timestamp(),clock_timestamp()+interval '9 minutes')`, subject, epoch); err != nil {
		t.Fatal(err)
	}
	if err := redeemer.Redeem(ctx, proof.ForTrustedDelivery(), "another@example.test", next); !errors.Is(err, usecase.ErrPasswordRecoveryDenied) {
		t.Fatal("recovery policy alias not checked against token-bound account")
	}
	if err := redeemer.Redeem(ctx, proof.ForTrustedDelivery(), email, "synthetic breached password not acceptable"); !errors.Is(err, usecase.ErrPasswordRecoveryDenied) {
		t.Fatal("blocked password passed policy")
	}
	if err := redeemer.Redeem(ctx, "bad-token", email, next); !errors.Is(err, usecase.ErrPasswordRecoveryDenied) {
		t.Fatal("invalid bearer passed proof decoder")
	}

	sessionProof := sha256.Sum256([]byte("synthetic recovery login grant " + subject))
	sessionDigest := sha256.Sum256([]byte("synthetic recovery opaque session " + subject))
	var allowed bool
	if err := grant.QueryRow(ctx, "SELECT mamony_identity.stage_password_auth_grant($1,$2,$3)",
		subject, epoch, sessionProof[:]).Scan(&allowed); err != nil || !allowed {
		t.Fatalf("old-epoch login grant not issued: %v %t", err, allowed)
	}
	if err := session.QueryRow(ctx, "SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
		sessionProof[:], sessionDigest[:], randomSyntheticID(t, "evt_")).Scan(&allowed); err != nil || !allowed {
		t.Fatalf("old-epoch session not issued: %v %t", err, allowed)
	}
	var resolved string
	if err := session.QueryRow(ctx, "SELECT COALESCE(mamony_identity.resolve_opaque_session($1),'')",
		sessionDigest[:]).Scan(&resolved); err != nil || resolved != subject {
		t.Fatal("synthetic old session was not valid before reset")
	}
	previous := func() (int64, bool, bool, int) {
		t.Helper()
		var current int64
		var tokenUsed, grantUsed bool
		var notices int
		err := admin.QueryRow(ctx, `SELECT s.session_epoch,
			c.consumed_at IS NOT NULL,g.consumed_at IS NOT NULL,
			(SELECT count(*) FROM mamony_identity.password_recovery_notices n
			 WHERE n.subject_id=$1)
			FROM mamony_identity.subjects s
			JOIN mamony_identity.password_recovery_challenges c ON c.subject_id=s.subject_id
			JOIN mamony_identity.password_recovery_assurance_grants g ON g.subject_id=s.subject_id
			WHERE s.subject_id=$1 AND c.challenge_id=$2`, subject, challenge).
			Scan(&current, &tokenUsed, &grantUsed, &notices)
		if err != nil {
			t.Fatal(err)
		}
		return current, tokenUsed, grantUsed, notices
	}
	newVerifier, err := hasher.Hash(ctx, next)
	if err != nil {
		t.Fatal(err)
	}
	collisionEvent := randomSyntheticID(t, "evt_")
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.security_outbox
	(event_id,subject_id,event_type,occurred_at)
	VALUES($1,$2,'SESSION_CREATED',clock_timestamp())`, collisionEvent, subject); err != nil {
		t.Fatal(err)
	}
	if accepted, err := store.Redeem(ctx, proof.Digest(), email, newVerifier, collisionEvent); err == nil || accepted {
		t.Fatal("audit ID collision incorrectly committed a password reset")
	}
	at, tokenUsed, grantUsed, notices := previous()
	if at != epoch || tokenUsed || grantUsed || notices != 0 {
		t.Fatal("late event insert failure failed to roll back entire reset")
	}
	if err := session.QueryRow(ctx, "SELECT COALESCE(mamony_identity.resolve_opaque_session($1),'')",
		sessionDigest[:]).Scan(&resolved); err != nil || resolved != subject {
		t.Fatal("failed reset incorrectly revoked old session")
	}
	if err := redeemer.Redeem(ctx, proof.ForTrustedDelivery(), email, next); err != nil {
		t.Fatalf("eligible one-shot recovery failed: %v", err)
	}
	at, tokenUsed, grantUsed, notices = previous()
	if at != epoch+1 || !tokenUsed || !grantUsed || notices != 1 {
		t.Fatal("password rotation did not atomically consume both proofs, bump epoch and enqueue notice")
	}
	if err := session.QueryRow(ctx, "SELECT COALESCE(mamony_identity.resolve_opaque_session($1),'')",
		sessionDigest[:]).Scan(&resolved); err != nil || resolved != "" {
		t.Fatal("old session still authorized after account recovery")
	}
	var oldMailStatus string
	if err := admin.QueryRow(ctx, "SELECT status FROM mamony_identity.password_recovery_deliveries WHERE challenge_id=$1",
		challenge).Scan(&oldMailStatus); err != nil || oldMailStatus != "DEAD" {
		t.Fatal("recovery request ciphertext remained pending after reset")
	}
	var auditCount int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.security_outbox
		WHERE subject_id=$1 AND event_type='PASSWORD_RECOVERED'`,
		subject).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatal("missing dedicated PASSWORD_RECOVERED audit event")
	}
	var recipient string
	if err := admin.QueryRow(ctx, "SELECT recipient FROM mamony_identity.password_recovery_notices WHERE subject_id=$1",
		subject).Scan(&recipient); err != nil || recipient != email {
		t.Fatal("recovery notice recipient did not match verified contact")
	}
	var persisted domain.PasswordVerifier
	if err := admin.QueryRow(ctx, `SELECT kdf_version,memory_kib,iterations,parallelism,salt,digest
		FROM mamony_identity.password_verifiers WHERE subject_id=$1`, subject).
		Scan(&persisted.Version, &persisted.MemoryKiB, &persisted.Iterations,
			&persisted.Parallelism, &persisted.Salt, &persisted.Digest); err != nil {
		t.Fatal(err)
	}
	if valid, err := hasher.Verify(ctx, next, persisted); err != nil || !valid {
		t.Fatal("new Argon2id password was not valid after reset")
	}
	if valid, err := hasher.Verify(ctx, "synthetic initial password for recovery", persisted); err != nil || valid {
		t.Fatal("old password still verified after recovery")
	}
	if err := redeemer.Redeem(ctx, proof.ForTrustedDelivery(), email, next); !errors.Is(err, usecase.ErrPasswordRecoveryDenied) {
		t.Fatal("replayed token performed another recovery")
	}
	at, _, _, notices = previous()
	if at != epoch+1 || notices != 1 {
		t.Fatal("replay changed epoch or audit")
	}

	// A second independently verified account exercises simultaneous
	// redemption on two SQL connections, not an in-memory mutex.
	newFixture := func(t *testing.T, seed byte) (string, string, string, [32]byte, int64) {
		t.Helper()
		s, alias := randomSyntheticID(t, "sub_"),
			strings.ToLower(randomSyntheticID(t, "race_")) + "@example.test"
		initial, err := hasher.Hash(ctx, "synthetic baseline for recovery concurrency")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx,
			"INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES($1,clock_timestamp())", s); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.email_aliases
			(canonical_email,subject_id,verified_at,created_at)
			VALUES($1,$2,clock_timestamp(),clock_timestamp())`, alias, s); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_verifiers
			(subject_id,kdf_version,memory_kib,iterations,parallelism,salt,digest,created_at)
			VALUES($1,1,65536,3,1,$2,$3,clock_timestamp())`,
			s, initial.Salt, initial.Digest); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx,
			"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", s); err != nil {
			t.Fatal(err)
		}
		var current int64
		if err := admin.QueryRow(ctx,
			"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", s).Scan(&current); err != nil {
			t.Fatal(err)
		}
		fresh, err := usecase.NewPasswordRecoveryProof()
		if err != nil {
			t.Fatal(err)
		}
		id := randomSyntheticID(t, "rcv_")
		authorized, err := issuer.Issue(ctx, usecase.RecoveryIssue{
			CanonicalEmail: alias, ChallengeID: id, TokenDigest: fresh.Digest(),
			KeyID: "synthetic-recovery", SealedToken: bytes.Repeat([]byte{seed}, 71),
		})
		if err != nil || !authorized {
			t.Fatal("fixture recovery issuance failed")
		}
		t.Cleanup(func() {
			cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			conn, err := pgx.Connect(cleanupCtx, adminDSN)
			if err != nil {
				t.Errorf("recovery cleanup connection: %v", err)
				return
			}
			defer func() { _ = conn.Close(context.Background()) }()
			if _, err := conn.Exec(cleanupCtx, `UPDATE mamony_identity.password_recovery_deliveries
				SET status='DEAD',lease_id=NULL,leased_until=NULL
				WHERE challenge_id=$1 AND status='PENDING'`, id); err != nil {
				t.Errorf("recovery cleanup: %v", err)
			}
		})
		if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_recovery_assurance_grants
			(subject_id,subject_epoch,approved_at,expires_at)
			VALUES($1,$2,clock_timestamp(),clock_timestamp()+interval '9 minutes')`, s, current); err != nil {
			t.Fatal(err)
		}
		return s, alias, fresh.ForTrustedDelivery(), fresh.Digest(), current
	}
	t.Run("concurrent single winner", func(t *testing.T) {
		s, alias, _, digest, before := newFixture(t, 0x78)
		candidate, err := hasher.Hash(ctx, "synthetic concurrent recovery new password")
		if err != nil {
			t.Fatal(err)
		}
		eventA, eventB := randomSyntheticID(t, "evt_"), randomSyntheticID(t, "evt_")
		results := make(chan bool, 2)
		var wg sync.WaitGroup
		for _, ev := range []string{eventA, eventB} {
			wg.Add(1)
			go func(event string) {
				defer wg.Done()
				ok, err := store.Redeem(ctx, digest, alias, candidate, event)
				results <- err == nil && ok
			}(ev)
		}
		wg.Wait()
		close(results)
		winners := 0
		for success := range results {
			if success {
				winners++
			}
		}
		var current int64
		var eventCount int
		if err := admin.QueryRow(ctx, `SELECT session_epoch FROM mamony_identity.subjects
			WHERE subject_id=$1`, s).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.security_outbox
			WHERE subject_id=$1 AND event_type='PASSWORD_RECOVERED'`, s).Scan(&eventCount); err != nil {
			t.Fatal(err)
		}
		if winners != 1 || eventCount != 1 || current != before+1 {
			t.Fatal("simultaneous recovery transactions produced duplicate authority")
		}
	})
	t.Run("contact epoch change rejects previously granted proof", func(t *testing.T) {
		s, alias, bearer, _, before := newFixture(t, 0x79)
		if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_aliases
			SET verified_at=clock_timestamp()+interval '1 second'
			WHERE subject_id=$1`, s); err != nil {
			t.Fatal(err)
		}
		if err := redeemer.Redeem(ctx, bearer, alias, next); !errors.Is(err, usecase.ErrPasswordRecoveryDenied) {
			t.Fatal("stale contact epoch still authorized account reset")
		}
		var current int64
		if err := admin.QueryRow(ctx,
			"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", s).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if current != before+1 {
			t.Fatal("contact update did not advance subject epoch")
		}
	})
	t.Run("DB expiry during redemption denies proof", func(t *testing.T) {
		s, alias, bearer, digest, before := newFixture(t, 0x7a)
		if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_recovery_challenges
			SET requested_at=clock_timestamp()-interval '16 minutes',
			    expires_at=clock_timestamp()-interval '1 minute'
			WHERE token_digest=$1`, digest[:]); err != nil {
			t.Fatal(err)
		}
		if err := redeemer.Redeem(ctx, bearer, alias, next); !errors.Is(err, usecase.ErrPasswordRecoveryDenied) {
			t.Fatal("expired recovery proof was accepted")
		}
		var current int64
		if err := admin.QueryRow(ctx,
			"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", s).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if current != before {
			t.Fatal("expired token unexpectedly changed account epoch")
		}
	})
}
