//go:build integration

package identity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	identityhandler "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/handler"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityMailgunMockRecoveryNoticeWebhookReconciliation verifies the
// purpose-separated v23 webhook end-to-end with a real PostgreSQL 18 fixture.
// No public route or live Mailgun is mounted; the signer is synthetic.
func TestIdentityMailgunMockRecoveryNoticeWebhookReconciliation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	sendDSN := guardedDB(t, "TEST_IDENTITY_RECOVERY_NOTICE_URL", "mamony_identity_recovery_notice_runtime")
	webhookDSN := guardedDB(t, "TEST_IDENTITY_RECOVERY_NOTICE_WEBHOOK_URL", "mamony_identity_recovery_notice_webhook_runtime")
	monitorDSN := guardedDB(t, "TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL", "mamony_identity_recovery_notice_monitor_runtime")
	maintenanceDSN := guardedDB(t, "TEST_IDENTITY_MAINTENANCE_URL", "mamony_identity_maintenance_runtime")
	original, _ := url.Parse(adminDSN)
	for _, raw := range []string{sendDSN, webhookDSN, monitorDSN, maintenanceDSN} {
		u, _ := url.Parse(raw)
		if u.Host != original.Host || u.Path != original.Path {
			t.Fatal("recovery webhook test must use the same disposable database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: 8})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	sender, err := identitypg.NewRecoverySecurityNoticeStore(open(sendDSN, "mamony_identity_recovery_notice_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := identitypg.NewRecoveryNoticeWebhookStore(open(webhookDSN, "mamony_identity_recovery_notice_webhook_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("synthetic-recovery-notice-webhook-signing-key")
	webhook, err := identityhandler.NewMailgunRecoveryNoticeWebhook(
		[][]byte{key}, "mg.example.test", recorder)
	if err != nil {
		t.Fatal(err)
	}
	// This is an ADMIN-ONLY synthetic database fixture. Genuine v21
	// PASSWORD_RECOVERED provenance is independently tested by the v21
	// redemption test. A test administrator must not imply a public writer.
	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "receipt_")) + "@example.test"
	recoveryEvent := randomSyntheticID(t, "evt_")
	oldEvent := randomSyntheticID(t, "evt_")
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.subjects
		(subject_id,created_at) VALUES($1,clock_timestamp())`, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.email_aliases
		(canonical_email,subject_id,verified_at,created_at)
		VALUES($1,$2,clock_timestamp(),clock_timestamp())`, email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.subjects
		SET status='ACTIVE' WHERE subject_id=$1`, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.security_outbox
		(event_id,subject_id,event_type,occurred_at)
		VALUES($1,$3,'ENROLLMENT_PENDING',clock_timestamp()),
		      ($2,$3,'ENROLLMENT_PENDING',clock_timestamp())`,
		recoveryEvent, oldEvent, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_recovery_notices
		(event_id,subject_id,recipient,occurred_at)
		VALUES($1,$2,$3,clock_timestamp())`,
		recoveryEvent, subject, email); err != nil {
		t.Fatal(err)
	}
	messageID := "2026.recovery-notice@mailgun.test"
	messageDigest := sha256.Sum256([]byte(messageID))
	// Deliberately reuse one synthetic provider digest across different
	// queues to prove the RECOVERY webhook cannot mutate the OLD notice.
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_change_notices
		(event_id,subject_id,recipient,occurred_at,status,attempts,
		 accepted_at,provider_message_digest,provider_state)
		VALUES($1,$2,$3,clock_timestamp(),'ACCEPTED',1,
		       clock_timestamp(),$4,'SUBMITTED')`,
		oldEvent, subject, email, messageDigest[:]); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	send := func(body []byte) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "https://hook.example.test/isolated", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		webhook.ServeHTTP(rec, req)
		return rec.Code
	}
	token1 := strings.Repeat("a", 50)
	early := signedMockSecurityWebhook(t, key, now, token1,
		"recovery-delivered-1", "<"+messageID+">", "delivered", "")
	if code := send(early); code != http.StatusOK {
		t.Fatalf("valid recovery delivery callback rejected: %d", code)
	}
	var receipts int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.password_recovery_notice_mailgun_receipts
		WHERE message_digest=$1`, messageDigest[:]).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatal("early recovery callback did not durably store PII-free receipt")
	}
	var legacyState string
	if err := admin.QueryRow(ctx, `SELECT provider_state FROM mamony_identity.password_change_notices
		WHERE event_id=$1`, oldEvent).Scan(&legacyState); err != nil || legacyState != "SUBMITTED" {
		t.Fatal("recovery callback mutated legacy PASSWORD_CHANGED notice")
	}
	lease := randomSyntheticID(t, "lse_")
	job, ok, err := sender.Claim(ctx, lease)
	if err != nil || !ok || job.EventID != recoveryEvent || job.Recipient != email {
		t.Fatalf("valid synthetic recovery alert could not be claimed: ok=%v err=%v", ok, err)
	}
	if err := sender.Finalize(ctx, recoveryEvent, lease, usecase.DeliveryAccepted,
		"<"+messageID+">"); err != nil {
		t.Fatal(err)
	}
	state := func() string {
		t.Helper()
		var provider string
		if err := admin.QueryRow(ctx, `SELECT provider_state FROM mamony_identity.password_recovery_notices
			WHERE event_id=$1`, recoveryEvent).Scan(&provider); err != nil {
			t.Fatal(err)
		}
		return provider
	}
	if state() != "DELIVERED" {
		t.Fatal("early webhook receipt did not reconcile with late Mailgun 200")
	}
	health, err := ReadRecoverySecurityNoticeHealth(ctx, monitorDSN)
	if err != nil || health.Delivered < 1 || health.SubmittedUnconfirmed != 0 {
		t.Fatalf("count-only delivery status did not distinguish provider acceptance: %+v %v", health, err)
	}
	// Parallel replay is acknowledged but creates no duplicate receipt.
	const parallel = 6
	codes := make(chan int, parallel)
	var wg sync.WaitGroup
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "https://hook.example.test/isolated", bytes.NewReader(early))
			req.Header.Set("Content-Type", "application/json")
			result := httptest.NewRecorder()
			webhook.ServeHTTP(result, req)
			codes <- result.Code
		}()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != http.StatusOK {
			t.Fatalf("signed duplicate callback failed safe acknowledgement: %d", code)
		}
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.password_recovery_notice_mailgun_receipts
		WHERE message_digest=$1`, messageDigest[:]).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatal("concurrent recovery callback replay bypassed unique token and event digests")
	}
	// A permanent bounce outranks delivered, and a late delivered event
	// must not lower a high-severity failure.
	permanent := signedMockSecurityWebhook(t, key, now, strings.Repeat("b", 50),
		"recovery-bounce-2", messageID, "failed", "permanent")
	if code := send(permanent); code != http.StatusOK || state() != "PERMANENT_FAILURE" {
		t.Fatal("permanent recovery bounce did not override delivered")
	}
	late := signedMockSecurityWebhook(t, key, now, strings.Repeat("c", 50),
		"recovery-late-delivered-3", messageID, "delivered", "")
	if code := send(late); code != http.StatusOK || state() != "PERMANENT_FAILURE" {
		t.Fatal("out-of-order delivered event masked permanent bounce")
	}
	complaint := signedMockSecurityWebhook(t, key, now, strings.Repeat("d", 50),
		"recovery-complaint-4", messageID, "complained", "")
	if code := send(complaint); code != http.StatusOK || state() != "COMPLAINED" {
		t.Fatal("signed recovery complaint did not escalate notice")
	}
	unsub := signedMockSecurityWebhook(t, key, now, strings.Repeat("e", 50),
		"recovery-unsubscribed-5", messageID, "unsubscribed", "")
	if code := send(unsub); code != http.StatusOK || state() != "UNSUBSCRIBED" {
		t.Fatal("signed unsubscribe did not override complaint")
	}
	unknown := signedMockSecurityWebhook(t, key, now, strings.Repeat("f", 50),
		"recovery-unknown-6", "unknown-recovery@mailgun.test", "failed", "permanent")
	if code := send(unknown); code != http.StatusOK {
		t.Fatalf("unknown but signed provider receipt was not recorded: %d", code)
	}
	health, err = ReadRecoverySecurityNoticeHealth(ctx, monitorDSN)
	if err != nil || health.ProviderFailures < 1 || health.UnmatchedEvents < 1 ||
		health.Delivered != 0 || health.SubmittedUnconfirmed != 0 {
		t.Fatalf("recovery monitor incorrectly reported complaint as delivered: %+v %v", health, err)
	}
	if err := admin.QueryRow(ctx, `SELECT provider_state FROM mamony_identity.password_change_notices
		WHERE event_id=$1`, oldEvent).Scan(&legacyState); err != nil || legacyState != "SUBMITTED" {
		t.Fatal("recovery webhook reconciled unrelated PASSWORD_CHANGED notice")
	}

	// Signer rotation is explicit and independent of the Mailgun sending
	// API key. Malformed requests must be denied before database writes.
	wrongKey := []byte("totally-separate-invalid-signer-credential")
	forged := signedMockSecurityWebhook(t, wrongKey, now, strings.Repeat("1", 50),
		"forged-recovery-7", messageID, "delivered", "")
	if got := send(forged); got != http.StatusNotAcceptable {
		t.Fatalf("untrusted recovery signer accepted: %d", got)
	}
	wrongDomain := signedMockSecurityWebhook(t, key, now, strings.Repeat("2", 50),
		"wrong-domain-8", messageID, "delivered", "")
	// Change domain in the signed JSON envelope: Mailgun's timestamp/token
	// signature still verifies, but domain binding must reject the payload.
	wrongDomain = bytes.Replace(wrongDomain,
		[]byte("mg.example.test"), []byte("evil.example.test"), 1)
	if got := send(wrongDomain); got != http.StatusNotAcceptable {
		t.Fatalf("wrong provider domain accepted: %d", got)
	}
	var countBefore int
	unknownDigest := sha256.Sum256([]byte("unknown-recovery@mailgun.test"))
	if err := admin.QueryRow(ctx, `SELECT count(*)
		FROM mamony_identity.password_recovery_notice_mailgun_receipts
		WHERE message_digest=$1 OR message_digest=$2`,
		messageDigest[:], unknownDigest[:]).
		Scan(&countBefore); err != nil {
		t.Fatal(err)
	}
	// A forged bounce must not downgrade state or create a receipt.
	if countBefore != 6 || state() != "UNSUBSCRIBED" {
		t.Fatal("forged callbacks changed recovery delivery state")
	}
	maintenance, err := pgx.Connect(ctx, maintenanceDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = maintenance.Close(context.Background()) }()
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_recovery_notice_mailgun_receipts
		SET recorded_at=clock_timestamp()-interval '8 days'
		WHERE message_digest=$1`, messageDigest[:]); err != nil {
		t.Fatal(err)
	}
	var purged int
	if err := maintenance.QueryRow(ctx,
		"SELECT mamony_identity.purge_old_recovery_notice_mailgun_receipts()").
		Scan(&purged); err != nil || purged < 5 {
		t.Fatalf("bounded digest-only receipt cleanup failed: count=%d err=%v", purged, err)
	}
	var auditCount int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM mamony_identity.security_outbox WHERE event_id=$1",
		recoveryEvent).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatal("receipt erasure unexpectedly destroyed immutable audit trail")
	}
	// Provider callback correlation is reversible only through the
	// external Mailgun message ID, not a persisted recipient or raw body.
}
