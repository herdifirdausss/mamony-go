//go:build integration

package identity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestIdentityZSecurityNoticeOutbox verifies transactional enqueue, forged
// event rejection, immutable recipient, bounded claims, retry and retention
// using real role-separated disposable PostgreSQL (no live email send).
func TestIdentityZSecurityNoticeOutbox(t *testing.T) {
	ctx, stop := context.WithTimeout(context.Background(), 100*time.Second)
	defer stop()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	verifierDSN := guardedDB(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	grantDSN := guardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	changeDSN := guardedDB(t, "TEST_IDENTITY_PASSWORD_CHANGE_URL", "mamony_identity_password_change_runtime")
	noticeDSN := guardedDB(t, "TEST_IDENTITY_SECURITY_NOTICE_URL", "mamony_identity_security_notice_runtime")
	maintDSN := guardedDB(t, "TEST_IDENTITY_MAINTENANCE_URL", "mamony_identity_maintenance_runtime")
	base, _ := url.Parse(adminDSN)
	for _, raw := range []string{verifierDSN, grantDSN, sessionDSN, changeDSN, noticeDSN, maintDSN} {
		u, _ := url.Parse(raw)
		if u.Host != base.Host || u.Path != base.Path {
			t.Fatal("notice integration requires one disposable database")
		}
	}
	connect := func(raw string) *pgx.Conn {
		t.Helper()
		c, err := pgx.Connect(ctx, raw)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close(context.Background()) })
		return c
	}
	admin := connect(adminDSN)
	verifier := connect(verifierDSN)
	granter := connect(grantDSN)
	session := connect(sessionDSN)
	changer := connect(changeDSN)
	maint := connect(maintDSN)
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: noticeDSN, RuntimeRole: "mamony_identity_security_notice_runtime", MaxConns: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	notices, err := identitypg.NewSecurityNoticeStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	// Each fixture uses syntactically valid digest values but NO plaintext
	// passwords; the real password verifier+Argon2id path is separately
	// mandatory in TestIdentityZAuthenticatedPasswordChange.
	makeChanged := func(label string) (string, string, string) {
		t.Helper()
		subject := randomSyntheticID(t, "sub_")
		email := strings.ToLower(randomSyntheticID(t, "notice_")) + "@example.test"
		if _, err := admin.Exec(ctx,
			"INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES ($1,clock_timestamp())",
			subject); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.email_aliases
			(canonical_email,subject_id,verified_at,created_at)
			VALUES ($1,$2,clock_timestamp(),clock_timestamp())`, email, subject); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_verifiers
			(subject_id,kdf_version,memory_kib,iterations,parallelism,salt,digest,created_at)
			VALUES ($1,1,65536,3,1,decode(repeat('11',16),'hex'),
				decode(repeat('22',32),'hex'),clock_timestamp())`, subject); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx,
			"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
			t.Fatal(err)
		}
		var epoch int64
		if err := admin.QueryRow(ctx,
			"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
			subject).Scan(&epoch); err != nil {
			t.Fatal(err)
		}
		// Distinct domain-separated synthetic digests are fixtures, not
		// actual bearer/password secrets. The subject and label ensure that
		// unrelated test cases cannot accidentally share a primary key.
		loginHash := sha256.Sum256([]byte("mamony:test:notice:login:" + subject + ":" + label))
		changeHash := sha256.Sum256([]byte("mamony:test:notice:change:" + subject + ":" + label))
		sessionHash := sha256.Sum256([]byte("mamony:test:notice:session:" + subject + ":" + label))
		loginDigest := loginHash[:]
		changeDigest := changeHash[:]
		sessionDigest := sessionHash[:]
		var allowed bool
		if err := granter.QueryRow(ctx,
			"SELECT mamony_identity.stage_password_auth_grant($1,$2,$3)",
			subject, epoch, loginDigest).Scan(&allowed); err != nil || !allowed {
			t.Fatalf("cannot stage synthetic login proof: %v %v", err, allowed)
		}
		if err := session.QueryRow(ctx,
			"SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
			loginDigest, sessionDigest, randomSyntheticID(t, "evt_")).Scan(&allowed); err != nil || !allowed {
			t.Fatalf("cannot mint synthetic valid session: %v %v", err, allowed)
		}
		if err := granter.QueryRow(ctx,
			"SELECT mamony_identity.stage_password_change_grant($1,$2,$3)",
			subject, epoch, changeDigest).Scan(&allowed); err != nil || !allowed {
			t.Fatalf("cannot stage synthetic change proof: %v %v", err, allowed)
		}
		event := randomSyntheticID(t, "evt_")
		if err := changer.QueryRow(ctx,
			"SELECT mamony_identity.rotate_password_verifier($1,$2,$3,$4,$5)",
			sessionDigest, changeDigest, bytes.Repeat([]byte{0x54}, 16),
			bytes.Repeat([]byte{0x65}, 32), event).Scan(&allowed); err != nil || !allowed {
			t.Fatalf("synthetic rotation missing trusted event: %v %v", err, allowed)
		}
		var events, queued int
		if err := admin.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM mamony_identity.security_outbox WHERE event_id=$1
				AND subject_id=$2 AND event_type='PASSWORD_CHANGED'),
			(SELECT count(*) FROM mamony_identity.password_change_notices
			 WHERE event_id=$1 AND subject_id=$2 AND recipient=$3 AND status='PENDING')`,
			event, subject, email).Scan(&events, &queued); err != nil || events != 1 || queued != 1 {
			t.Fatalf("outbox + verified recipient snapshot were not atomic: %v %d %d", err, events, queued)
		}
		return subject, email, event
	}
	subject, recipient, event := makeChanged("first")
	// The verifier had broad INSERT on audit in earlier migrations. This must
	// not let it forge a PASSWORD_CHANGED event and mail a user.
	for _, conn := range []*pgx.Conn{verifier, admin} {
		_, err := conn.Exec(ctx, `INSERT INTO mamony_identity.security_outbox
			(event_id,subject_id,event_type,occurred_at)
			VALUES ($1,$2,'PASSWORD_CHANGED',clock_timestamp())`,
			randomSyntheticID(t, "evt_"), subject)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("privileged but non-change event origin forged notice: %v", err)
		}
	}
	first, ok, err := notices.Claim(ctx, randomSyntheticID(t, "lse_"))
	if err != nil || !ok || first.EventID != event || first.Recipient != recipient {
		t.Fatalf("claim did not return committed password event: %+v %v", first, err)
	}
	if _, ok, err := notices.Claim(ctx, randomSyntheticID(t, "lse_")); err != nil || ok {
		t.Fatal("leased notice was reclaimed before expiry")
	}
	if err := notices.Finalize(ctx, first.EventID, randomSyntheticID(t, "lse_"), usecase.DeliveryAccepted); err == nil {
		t.Fatal("wrong lease acknowledged notice")
	}
	if err := notices.Finalize(ctx, first.EventID, first.LeaseID, usecase.DeliveryTemporary); err != nil {
		t.Fatal(err)
	}
	var state string
	var attempts int
	if err := admin.QueryRow(ctx, `SELECT status,attempts FROM mamony_identity.password_change_notices
		WHERE event_id=$1`, event).Scan(&state, &attempts); err != nil || state != "PENDING" || attempts != 1 {
		t.Fatal("temporary failure did not persist first bounded retry")
	}
	if _, ok, err := notices.Claim(ctx, randomSyntheticID(t, "lse_")); err != nil || ok {
		t.Fatal("backoff bypassed before due time")
	}
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_change_notices
		SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE event_id=$1`, event); err != nil {
		t.Fatal(err)
	}
	second, ok, err := notices.Claim(ctx, randomSyntheticID(t, "lse_"))
	if err != nil || !ok || second.LeaseID == first.LeaseID || second.EventID != event {
		t.Fatal("a fresh lease did not recover retryable notice")
	}
	if err := notices.Finalize(ctx, first.EventID, first.LeaseID, usecase.DeliveryAccepted); err == nil {
		t.Fatal("stale lease completed current worker claim")
	}
	if err := notices.Finalize(ctx, second.EventID, second.LeaseID, usecase.DeliveryAccepted); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT status,attempts FROM mamony_identity.password_change_notices
		WHERE event_id=$1`, event).Scan(&state, &attempts); err != nil || state != "ACCEPTED" || attempts != 2 {
		t.Fatal("accepted provider submission did not finalize once")
	}
	if _, ok, err := notices.Claim(ctx, randomSyntheticID(t, "lse_")); err != nil || ok {
		t.Fatal("accepted notification was redelivered")
	}
	// If the account contact changed before submission, never send an old
	// contact-address notice; quarantine it as DEAD on the next claim.
	staleSubject, oldRecipient, staleEvent := makeChanged("second")
	newRecipient := strings.ToLower(randomSyntheticID(t, "changed_")) + "@example.test"
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_aliases
		SET canonical_email=$1 WHERE subject_id=$2`, newRecipient, staleSubject); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := notices.Claim(ctx, randomSyntheticID(t, "lse_")); err != nil || ok {
		t.Fatal("stale-contact notification escaped recipient fence")
	}
	if err := admin.QueryRow(ctx, `SELECT status,recipient FROM mamony_identity.password_change_notices
		WHERE event_id=$1`, staleEvent).Scan(&state, &oldRecipient); err != nil || state != "DEAD" {
		t.Fatal("rebound email was not quarantined")
	}
	// A third pending notice fails permanently after the fifth bounded send.
	_, _, retryEvent := makeChanged("third")
	for i := 0; i < 5; i++ {
		if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_change_notices
			SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE event_id=$1`, retryEvent); err != nil {
			t.Fatal(err)
		}
		job, ok, err := notices.Claim(ctx, randomSyntheticID(t, "lse_"))
		if err != nil || !ok || job.EventID != retryEvent {
			t.Fatalf("retry %d not claimable: %+v %v", i, job, err)
		}
		if err := notices.Finalize(ctx, job.EventID, job.LeaseID, usecase.DeliveryTemporary); err != nil {
			t.Fatal(err)
		}
	}
	if err := admin.QueryRow(ctx, `SELECT status,attempts FROM mamony_identity.password_change_notices
		WHERE event_id=$1`, retryEvent).Scan(&state, &attempts); err != nil || state != "DEAD" || attempts != 5 {
		t.Fatal("bounded fifth send did not enter terminal status")
	}
	if _, ok, err := notices.Claim(ctx, randomSyntheticID(t, "lse_")); err != nil || ok {
		t.Fatal("terminal notice was claimed again")
	}
	// Seven-day recipient PII purge is role-restricted, bounded and leaves
	// identifier-only PASSWORD_CHANGED history intact.
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_change_notices
		SET occurred_at=clock_timestamp()-interval '8 days'
		WHERE event_id=$1`, event); err != nil {
		t.Fatal(err)
	}
	var purged int
	if err := maint.QueryRow(ctx, `SELECT mamony_identity.purge_old_password_change_notices()`).Scan(&purged); err != nil || purged < 1 {
		t.Fatalf("bounded PII retention failed: %v %d", err, purged)
	}
	var retained int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.security_outbox
		WHERE event_id=$1 AND event_type='PASSWORD_CHANGED'`, event).Scan(&retained); err != nil || retained != 1 {
		t.Fatal("recipient purge deleted mandatory event obligation")
	}
}
