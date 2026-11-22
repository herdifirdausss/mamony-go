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
		if err != nil { t.Fatal(err) }
		t.Cleanup(func() { _ = conn.Close(context.Background()) })
		return conn
	}
	admin, grant, session := connect(adminDSN), connect(grantDSN), connect(sessionDSN)
	pool := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: 4})
		if err != nil { t.Fatal(err) }
		t.Cleanup(p.Close)
		return p
	}
	issuer, err := identitypg.NewRecoveryIssuerStore(pool(issuerDSN, "mamony_identity_recovery_issuer_runtime"))
	if err != nil { t.Fatal(err) }
	store, err := identitypg.NewRecoveryRedemptionStore(pool(redeemerDSN, "mamony_identity_recovery_redeemer_runtime"))
	if err != nil { t.Fatal(err) }
	hasher, err := credential.NewPasswordHasher(2)
	if err != nil { t.Fatal(err) }
	policy := domain.PasswordPolicy{IsBlocked: func(value, email string) bool {
		return value == "synthetic breached password not acceptable" || strings.Contains(value, email)
	}}
	redeemer, err := usecase.NewPasswordRecoveryRedemption(store, hasher, policy)
	if err != nil { t.Fatal(err) }

	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "reset_")) + "@example.test"
	old, err := hasher.Hash(ctx, "synthetic initial password for recovery")
	if err != nil { t.Fatal(err) }
	if _, err := admin.Exec(ctx, "INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES($1,clock_timestamp())", subject); err != nil { t.Fatal(err) }
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.email_aliases
	(canonical_email,subject_id,verified_at,created_at)
	VALUES($1,$2,clock_timestamp(),clock_timestamp())`, email, subject); err != nil { t.Fatal(err) }
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_verifiers
	(subject_id,kdf_version,memory_kib,iterations,parallelism,salt,digest,created_at)
	VALUES($1,1,65536,3,1,$2,$3,clock_timestamp())`, subject, old.Salt, old.Digest); err != nil { t.Fatal(err) }
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil { t.Fatal(err) }
	var epoch int64
	if err := admin.QueryRow(ctx, "SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", subject).Scan(&epoch); err != nil { t.Fatal(err) }
	proof, err := usecase.NewPasswordRecoveryProof()
	if err != nil { t.Fatal(err) }
	challenge := randomSyntheticID(t, "rcv_")
	issued, err := issuer.Issue(ctx, usecase.RecoveryIssue{
		CanonicalEmail: email, ChallengeID: challenge, TokenDigest: proof.Digest(),
		KeyID: "synthetic-recovery", SealedToken: bytes.Repeat([]byte{0x4c}, 71),
	})
	if err != nil || !issued { t.Fatalf("recovery challenge issue failed: %v %t", err, issued) }
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		conn, err := pgx.Connect(cleanupCtx, adminDSN)
		if err != nil { t.Errorf("cleanup: %v", err); return }
		defer func() { _ = conn.Close(context.Background()) }()
		if _, err := conn.Exec(cleanupCtx, `UPDATE mamony_identity.password_recovery_deliveries
		SET status='DEAD',lease_id=NULL,leased_until=NULL WHERE challenge_id=$1 AND status='PENDING'`,
			challenge); err != nil { t.Errorf("cleanup: %v", err) }
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
