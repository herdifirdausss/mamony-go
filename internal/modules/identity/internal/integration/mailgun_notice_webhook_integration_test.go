//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/handler/mailgunwebhook"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/integration/internal/fixture"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

func signedMockSecurityWebhook(t *testing.T, secret []byte, when time.Time,
	token, webhookID, messageID, kind, severity string) []byte {
	t.Helper()
	stamp := strconv.FormatInt(when.Unix(), 10)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(stamp + token))
	data := map[string]any{
		"signature": map[string]string{
			"timestamp": stamp, "token": token,
			"signature": hex.EncodeToString(mac.Sum(nil)),
		},
		"event-data": map[string]any{
			"event": kind, "id": webhookID, "timestamp": float64(when.Unix()),
			"severity": severity, "domain": map[string]string{"name": "mg.example.test"},
			"message": map[string]any{"headers": map[string]string{
				"message-id": messageID,
			}},
			"recipient": "sensitive-fixture@example.test",
		},
	}
	result, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestIdentityMailgunMockWebhookReconciliation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	adminDSN := fixture.GuardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	noticeDSN := fixture.GuardedDB(t, "TEST_IDENTITY_SECURITY_NOTICE_URL", "mamony_identity_security_notice_runtime")
	webhookDSN := fixture.GuardedDB(t, "TEST_IDENTITY_NOTICE_WEBHOOK_URL", "mamony_identity_notice_webhook_runtime")
	maintenanceDSN := fixture.GuardedDB(t, "TEST_IDENTITY_MAINTENANCE_URL", "mamony_identity_maintenance_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, raw := range []string{noticeDSN, webhookDSN, maintenanceDSN} {
		u, _ := url.Parse(raw)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("real webhook test may use only the disposable database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{
			DSN: dsn, RuntimeRole: role, MaxConns: 8,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	noticeStore, err := identitypg.NewSecurityNoticeStore(open(noticeDSN,
		"mamony_identity_security_notice_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := identitypg.NewSecurityNoticeWebhookStore(open(webhookDSN,
		"mamony_identity_notice_webhook_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("synthetic-webhook-signing-key-not-sending-api-key")
	webhook, err := mailgunwebhook.NewSecurityNotice(
		[][]byte{secret}, "mg.example.test", recorder)
	if err != nil {
		t.Fatal(err)
	}
	// Admin-only test fixture: a synthetic PENDING notice linked by FK to
	// a generic event. Production notices are created *only* by the trusted
	// v15/v16 PASSWORD_CHANGED path, independently verified in the required
	// TestIdentityZAuthenticatedPasswordChange and Mailgun sender E2E.
	subject := fixture.RandomID(t, "sub_")
	email := strings.ToLower(fixture.RandomID(t, "notice_")) + "@example.test"
	eventID := fixture.RandomID(t, "evt_")
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.subjects
		(subject_id,created_at) VALUES ($1,clock_timestamp())`, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.email_aliases
		(canonical_email,subject_id,verified_at,created_at)
		VALUES ($1,$2,clock_timestamp(),clock_timestamp())`, email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.subjects
		SET status='ACTIVE' WHERE subject_id=$1`, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.security_outbox
		(event_id,subject_id,event_type,occurred_at)
		VALUES ($1,$2,'ENROLLMENT_PENDING',clock_timestamp())`, eventID, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_change_notices
		(event_id,subject_id,recipient,occurred_at)
		VALUES ($1,$2,$3,clock_timestamp())`, eventID, subject, email); err != nil {
		t.Fatal(err)
	}
	// Non-sensitive correlation is SHA-256 of normalized Mailgun Message-ID.
	messageID := "20261122.example@mailgun.test"
	messageDigest := sha256.Sum256([]byte(messageID))
	when := time.Now().UTC()
	send := func(body []byte) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "https://hook.example.test/internal", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		webhook.ServeHTTP(out, req)
		return out.Code
	}
	token1 := strings.Repeat("a", 50)
	beforeAck := signedMockSecurityWebhook(t, secret, when, token1,
		"evt-delivered-1", "<"+messageID+">", "delivered", "")
	// Provider may POST a signed delivery webhook before the sender can
	// transactionally finalize its 200 response. Preserve the receipt first.
	if got := send(beforeAck); got != http.StatusOK {
		t.Fatalf("early provider webhook failed: %d", got)
	}
	var receiptCount int
	if err := admin.QueryRow(ctx, `SELECT count(*)
		FROM mamony_identity.security_notice_mailgun_receipts
		WHERE message_digest=$1`, messageDigest[:]).Scan(&receiptCount); err != nil ||
		receiptCount != 1 {
		t.Fatal("signed webhook did not commit durable, PII-free receipt")
	}
	leaseID := fixture.RandomID(t, "lse_")
	job, ok, err := noticeStore.Claim(ctx, leaseID)
	if err != nil || !ok || job.EventID != eventID {
		t.Fatalf("valid fixture job not claimed under restricted role: %+v %v", job, err)
	}
	if err := noticeStore.Finalize(ctx, eventID, leaseID, usecase.DeliveryAccepted,
		"<"+messageID+">"); err != nil {
		t.Fatal(err)
	}
	var status, state string
	var storedDigest []byte
	if err := admin.QueryRow(ctx, `SELECT status,provider_state,provider_message_digest
		FROM mamony_identity.password_change_notices WHERE event_id=$1`, eventID).
		Scan(&status, &state, &storedDigest); err != nil ||
		status != "ACCEPTED" || state != "DELIVERED" ||
		!bytes.Equal(storedDigest, messageDigest[:]) {
		t.Fatalf("early webhook did not reconcile at provider acceptance: %v %s %s",
			err, status, state)
	}
	// Replay and concurrent duplicate POSTs are idempotently acknowledged,
	// never inserted multiple times or exposed to other SQL roles.
	const parallel = 6
	var wg sync.WaitGroup
	codes := make(chan int, parallel)
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "https://hook.example.test/internal", bytes.NewReader(beforeAck))
			req.Header.Set("Content-Type", "application/json")
			out := httptest.NewRecorder()
			webhook.ServeHTTP(out, req)
			codes <- out.Code
		}()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != http.StatusOK {
			t.Fatalf("replayed valid event rejected instead of deduped: %d", code)
		}
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM
		mamony_identity.security_notice_mailgun_receipts
		WHERE message_digest=$1`, messageDigest[:]).Scan(&receiptCount); err != nil ||
		receiptCount != 1 {
		t.Fatal("parallel webhook replays bypassed unique digest")
	}
	// A later permanent provider failure (bounce) must override delivered.
	token2 := strings.Repeat("b", 50)
	failed := signedMockSecurityWebhook(t, secret, when, token2,
		"evt-failed-2", messageID, "failed", "permanent")
	if code := send(failed); code != http.StatusOK {
		t.Fatalf("valid permanent bounce rejected: %d", code)
	}
	if err := admin.QueryRow(ctx, `SELECT provider_state
		FROM mamony_identity.password_change_notices WHERE event_id=$1`,
		eventID).Scan(&state); err != nil || state != "PERMANENT_FAILURE" {
		t.Fatal("permanent provider failure was not reconciled")
	}
	// A delayed delivered event must never hide a permanent bounce.
	token3 := strings.Repeat("c", 50)
	lateDelivered := signedMockSecurityWebhook(t, secret, when, token3,
		"evt-late-delivered-3", messageID, "delivered", "")
	if code := send(lateDelivered); code != http.StatusOK {
		t.Fatalf("late valid provider event rejected: %d", code)
	}
	if err := admin.QueryRow(ctx, `SELECT provider_state
		FROM mamony_identity.password_change_notices WHERE event_id=$1`,
		eventID).Scan(&state); err != nil || state != "PERMANENT_FAILURE" {
		t.Fatal("late delivery wrongly masked provider bounce")
	}
	// Complaint escalates above bounce; no automatic resend is started.
	token4 := strings.Repeat("d", 50)
	complaint := signedMockSecurityWebhook(t, secret, when, token4,
		"evt-complaint-4", messageID, "complained", "")
	if code := send(complaint); code != http.StatusOK {
		t.Fatalf("signed complaint rejected: %d", code)
	}
	if err := admin.QueryRow(ctx, `SELECT provider_state
		FROM mamony_identity.password_change_notices WHERE event_id=$1`,
		eventID).Scan(&state); err != nil || state != "COMPLAINED" {
		t.Fatal("complaint did not override previous provider failures")
	}
	// Unknown message IDs are stored only as digests and counted as unmatched.
	token5 := strings.Repeat("e", 50)
	unknown := signedMockSecurityWebhook(t, secret, when, token5,
		"evt-unmatched-5", "unrecognized@mailgun.test", "failed", "permanent")
	if code := send(unknown); code != http.StatusOK {
		t.Fatalf("unknown signed event should become monitored unmatched receipt: %d", code)
	}
	health, err := identity.ReadSecurityNoticeHealth(ctx, maintenanceDSN)
	if err != nil || health.ProviderFailures < 1 || health.UnmatchedEvents < 1 {
		t.Fatalf("count-only operator health missing failure/unmatched signal: %+v %v", health, err)
	}
	badBody := signedMockSecurityWebhook(t, []byte("wrong-signer-key-value-with-minimum-length"),
		when, strings.Repeat("f", 50), "evt-forged-6", messageID, "delivered", "")
	if code := send(badBody); code != http.StatusNotAcceptable {
		t.Fatalf("unsigned forged status was accepted: %d", code)
	}
	// Retention is bounded; no raw recipient/message IDs have been stored
	// in the receipts table and the original security outbox row survives.
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.security_notice_mailgun_receipts
		SET recorded_at=clock_timestamp()-interval '8 days'
		WHERE message_digest=$1`, messageDigest[:]); err != nil {
		t.Fatal(err)
	}
	maintenance, err := pgx.Connect(ctx, maintenanceDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = maintenance.Close(context.Background()) }()
	var purged int
	if err := maintenance.QueryRow(ctx,
		"SELECT mamony_identity.purge_old_mailgun_security_receipts()").
		Scan(&purged); err != nil || purged < 4 {
		t.Fatalf("bounded receipt purge failed: %v %d", err, purged)
	}
	var audit int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM mamony_identity.security_outbox WHERE event_id=$1",
		eventID).Scan(&audit); err != nil || audit != 1 {
		t.Fatal("provider receipt retention erased identifier-only security outbox")
	}
	// No live Mailgun, no real recipients, and no public webhook route were used.
}
