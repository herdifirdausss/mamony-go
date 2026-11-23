//go:build integration

package identity

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	identityhandler "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/handler"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailgun"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityMailgunMockRecoveryCodeSecurityNotice exercises v25 actual
// enrollment+rotation audit/notice provenance, v26 sender/monitor authority,
// and local HTTP-only Mailgun mocks. No live provider or public route exists.
func TestIdentityMailgunMockRecoveryCodeNoticeWebhookReconciliation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 125*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	managerDSN := guardedDB(t, "TEST_IDENTITY_RECOVERY_CODE_MANAGER_URL", "mamony_identity_recovery_code_manager_runtime")
	noticeDSN := guardedDB(t, "TEST_IDENTITY_RECOVERY_CODE_NOTICE_URL", "mamony_identity_recovery_code_notice_runtime")
	monitorDSN := guardedDB(t, "TEST_IDENTITY_RECOVERY_CODE_NOTICE_MONITOR_URL", "mamony_identity_recovery_code_notice_monitor_runtime")
	webhookDSN := guardedDB(t, "TEST_IDENTITY_RECOVERY_CODE_NOTICE_WEBHOOK_URL", "mamony_identity_recovery_code_notice_webhook_runtime")
	root, _ := url.Parse(adminDSN)
	for _, raw := range []string{managerDSN, noticeDSN, monitorDSN, webhookDSN} {
		u, _ := url.Parse(raw)
		if u.Host != root.Host || u.Path != root.Path {
			t.Fatal("code-change worker must use one disposable database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: 3})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	manager, err := identitypg.NewRecoveryCodeManagerStore(open(managerDSN, "mamony_identity_recovery_code_manager_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := identitypg.NewRecoveryCodeSecurityNoticeStore(open(noticeDSN, "mamony_identity_recovery_code_notice_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := identitypg.NewRecoveryCodeNoticeWebhookStore(
		open(webhookDSN, "mamony_identity_recovery_code_notice_webhook_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	signingKey := []byte("synthetic-recovery-code-webhook-signing-key")
	webhook, err := identityhandler.NewMailgunRecoveryCodeNoticeWebhook(
		[][]byte{signingKey}, "mg.example.test", recorder)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now().UTC()
	deliverWebhook := func(payload []byte) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost,
			"https://hook.example.test/internal-recovery-code", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		webhook.ServeHTTP(response, req)
		return response.Code
	}

	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "notice_")) + "@example.test"
	if _, err := admin.Exec(ctx, "INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES($1,clock_timestamp())", subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.email_aliases
		(canonical_email,subject_id,verified_at,created_at)
		VALUES($1,$2,clock_timestamp(),clock_timestamp())`, email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	var epoch int64
	if err := admin.QueryRow(ctx,
		"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
		subject).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	sessionDigest := sha256.Sum256([]byte("synthetic-code-notice-session:" + subject))
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.opaque_sessions
		(token_digest,subject_id,subject_epoch,issued_at,expires_at)
		VALUES($1,$2,$3,clock_timestamp()-interval '1 second',
		       clock_timestamp()+interval '12 hours')`,
		sessionDigest[:], subject, epoch); err != nil {
		t.Fatal(err)
	}
	freshGrant := func(suffix string) [32]byte {
		t.Helper()
		d := sha256.Sum256([]byte("synthetic-code-notice-reauth:" + suffix + subject))
		if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_login_grants
			(grant_digest,subject_id,subject_epoch,purpose,issued_at,expires_at)
			VALUES($1,$2,$3,'RECOVERY_CODE_ROTATION',clock_timestamp(),
			       clock_timestamp()+interval '30 seconds')`,
			d[:], subject, epoch); err != nil {
			t.Fatal(err)
		}
		return d
	}
	batch := func(tag string) [5][32]byte {
		var v [5][32]byte
		for i := range v {
			v[i] = sha256.Sum256([]byte(tag + string(rune('a'+i)) + subject))
		}
		return v
	}
	approval := sha256.Sum256([]byte("synthetic-code-notice-independent:" + subject))
	// Disposable ADMIN fixture simulates a previously trusted factor binding;
	// no production runtime or API may register a signing key.
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.recovery_code_factor_keys
		(subject_id,public_key,bound_epoch,bound_contact_version,bound_at)
		VALUES($1,$2,$3,1,clock_timestamp())`,
		subject, []byte(publicKey), epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_recovery_code_enrollment_approvals
		(subject_id,subject_epoch,proof_digest,approved_at,expires_at,factor_public_key)
		VALUES($1,$2,$3,clock_timestamp(),clock_timestamp()+interval '4 minutes',$4)`,
		subject, epoch, approval[:], []byte(publicKey)); err != nil {
		t.Fatal(err)
	}
	first := batch("first-notice-")
	createdEvent := randomSyntheticID(t, "evt_")
	grant := freshGrant("enrolled")
	allowed, err := manager.Replace(ctx, sessionDigest, grant, approval, first, createdEvent)
	if err != nil || !allowed {
		t.Fatalf("v25 genuine code enrollment failed: %v %t", err, allowed)
	}
	second := batch("second-notice-")
	rotatedEvent := randomSyntheticID(t, "evt_")
	grant = freshGrant("rotated")
	allowed, err = manager.Replace(ctx, sessionDigest, grant, first[0], second, rotatedEvent)
	if err != nil || !allowed {
		t.Fatalf("v25 genuine code rotation failed: %v %t", err, allowed)
	}
	t.Cleanup(func() {
		limited, stop := context.WithTimeout(context.Background(), 4*time.Second)
		defer stop()
		conn, err := pgx.Connect(limited, adminDSN)
		if err != nil {
			t.Errorf("cleanup connection: %v", err)
			return
		}
		defer func() { _ = conn.Close(context.Background()) }()
		if _, err := conn.Exec(limited, `UPDATE mamony_identity.password_recovery_code_notices
			SET status='DEAD',lease_id=NULL,leased_until=NULL,accepted_at=NULL,provider_message_digest=NULL
			WHERE subject_id=$1 AND status='PENDING'`, subject); err != nil {
			t.Errorf("cleanup synthetic code-change notice: %v", err)
		}
	})
	var pending int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.password_recovery_code_notices
		WHERE subject_id=$1 AND recipient=$2 AND status='PENDING'`, subject, email).
		Scan(&pending); err != nil || pending != 2 {
		t.Fatal("v25 did not transactionally enqueue both code-change notices")
	}
	var mu sync.Mutex
	requests := map[string]int{}
	senderIDs := map[string]string{
		"RECOVERY_CODES_ENROLLED": "created-code-webhook-notice@mailgun.test",
		"RECOVERY_CODES_ROTATED":  "rotated-code-webhook-notice@mailgun.test",
	}
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		user, key, authenticated := req.BasicAuth()
		if !authenticated || user != "api" || key != "synthetic-code-notice-key" ||
			req.Method != http.MethodPost || req.URL.Path != "/v3/mg.example.test/messages" {
			t.Error("wrong Mailgun API endpoint or authentication")
		}
		req.Body = http.MaxBytesReader(w, req.Body, 12<<10)
		reader, err := req.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		fields := map[string]string{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Error(err)
				return
			}
			value, readErr := io.ReadAll(io.LimitReader(part, 4097))
			_ = part.Close()
			if readErr != nil || len(value) > 4096 {
				t.Error("unbounded provider field")
				return
			}
			fields[part.FormName()] = string(value)
		}
		kind := ""
		switch fields["subject"] {
		case "Mamony security alert: recovery codes created":
			kind = "RECOVERY_CODES_ENROLLED"
		case "Mamony security alert: recovery codes replaced":
			kind = "RECOVERY_CODES_ROTATED"
		default:
			t.Error("unknown code-change email template")
			return
		}
		requests[kind]++
		message := fields["text"]
		if fields["to"] != email ||
			fields["o:tracking"] != "no" || fields["o:tracking-clicks"] != "no" ||
			fields["o:tracking-opens"] != "no" || fields["o:require-tls"] != "yes" ||
			fields["o:deliver-within"] != "5m" ||
			strings.Contains(message, subject) || strings.Contains(message, createdEvent) ||
			strings.Contains(message, rotatedEvent) ||
			strings.Contains(message, "http://") || strings.Contains(message, "https://") ||
			strings.Contains(message, "256-bit") ||
			(kind == "RECOVERY_CODES_ENROLLED" && !strings.Contains(message, "created")) ||
			(kind == "RECOVERY_CODES_ROTATED" && !strings.Contains(message, "replaced")) {
			t.Error("code-change notice leaked sensitive material or missing privacy controls")
		}
		w.Header().Set("Content-Type", "application/json")
		if kind == "RECOVERY_CODES_ENROLLED" && requests[kind] == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"message":"transient synthetic throttle"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"id":"<%s>"}`, senderIDs[kind])
	}))
	defer mock.Close()
	origin, err := url.Parse(mock.URL)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := mailgun.NewRecoveryCodeSecurityNoticeSender(mailgun.SecurityNoticeConfig{
		Region: mailgun.RegionUS, Domain: "mg.example.test",
		From:   "Mamony Security <security@mg.example.test>",
		APIKey: "synthetic-code-notice-key",
	}, &http.Client{Timeout: 2 * time.Second, Transport: mockMailgunTransport{origin: origin}})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := usecase.NewRecoveryCodeSecurityNoticeWorker(store, sender)
	if err != nil {
		t.Fatal(err)
	}
	// Provider webhook arrives BEFORE an API ACK: persist a separate
	// purpose-specific digest-only receipt, reconcile when ACK commits.
	early := signedMockSecurityWebhook(t, signingKey, clock, strings.Repeat("a", 50),
		"recovery-code-early-delivered", "<created-code-webhook-notice@mailgun.test>", "delivered", "")
	if status := deliverWebhook(early); status != http.StatusOK {
		t.Fatalf("signed early code-notice callback rejected: %d", status)
	}
	var earlyReceipts int
	createdDigest := sha256.Sum256([]byte(senderIDs["RECOVERY_CODES_ENROLLED"]))
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.recovery_code_notice_mailgun_receipts
		WHERE message_digest=$1`, createdDigest[:]).Scan(&earlyReceipts); err != nil || earlyReceipts != 1 {
		t.Fatal("early code-change callback was not recorded independent of provider ACK")
	}
	for i := 0; i < 2; i++ {
		if processed, err := worker.DispatchOnce(ctx); !processed || err != nil {
			t.Fatalf("real recovery-code notice worker dispatch %d: %t %v", i, processed, err)
		}
	}
	health, err := ReadRecoveryCodeSecurityNoticeHealth(ctx, monitorDSN)
	if err != nil || health.Retrying < 1 || health.SubmittedUnconfirmed < 1 {
		t.Fatalf("count-only monitor did not distinguish retry from submission: %+v %v", health, err)
	}
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_recovery_code_notices
		SET next_attempt_at=clock_timestamp()-interval '1 second'
		WHERE event_id=$1 AND status='PENDING'`, createdEvent); err != nil {
		t.Fatal(err)
	}
	if processed, err := worker.DispatchOnce(ctx); !processed || err != nil {
		t.Fatal("429 recovery-code alert did not retry with the lease")
	}
	rotatedDigest := sha256.Sum256([]byte(senderIDs["RECOVERY_CODES_ROTATED"]))
	for _, item := range []struct {
		id       string
		provider string
		attempts int
	}{{createdEvent, senderIDs["RECOVERY_CODES_ENROLLED"], 2},
		{rotatedEvent, senderIDs["RECOVERY_CODES_ROTATED"], 1}} {
		var state string
		var attempts int
		var digest []byte
		if err := admin.QueryRow(ctx, `SELECT status,attempts,provider_message_digest
			FROM mamony_identity.password_recovery_code_notices WHERE event_id=$1`,
			item.id).Scan(&state, &attempts, &digest); err != nil {
			t.Fatal(err)
		}
		expected := sha256.Sum256([]byte(item.provider))
		if state != "ACCEPTED" || attempts != item.attempts ||
			!bytes.Equal(digest, expected[:]) {
			t.Fatalf("provider ACK not correlated to code-notice event: %s %d", state, attempts)
		}
	}
	health, err = ReadRecoveryCodeSecurityNoticeHealth(ctx, monitorDSN)
	if err != nil || health.Delivered < 1 || health.SubmittedUnconfirmed < 1 {
		t.Fatal("Mailgun callback and provider-acceptance counts were confused")
	}
	// Parallel replay must acknowledge but MUST NOT create extra rows.
	const replayCount = 6
	answers := make(chan int, replayCount)
	var replies sync.WaitGroup
	for i := 0; i < replayCount; i++ {
		replies.Add(1)
		go func() {
			defer replies.Done()
			req := httptest.NewRequest(http.MethodPost,
				"https://hook.example.test/internal-recovery-code", bytes.NewReader(early))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			webhook.ServeHTTP(rec, req)
			answers <- rec.Code
		}()
	}
	replies.Wait()
	close(answers)
	for status := range answers {
		if status != http.StatusOK {
			t.Fatal("signed concurrent callback replay was not acknowledged")
		}
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.recovery_code_notice_mailgun_receipts
		WHERE message_digest=$1`, createdDigest[:]).Scan(&earlyReceipts); err != nil || earlyReceipts != 1 {
		t.Fatal("duplicate signed callback bypassed replay protection")
	}
	codeState := func(event string) string {
		t.Helper()
		var state string
		if err := admin.QueryRow(ctx, `SELECT provider_state FROM mamony_identity.password_recovery_code_notices
			WHERE event_id=$1`, event).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	if codeState(createdEvent) != "DELIVERED" || codeState(rotatedEvent) != "SUBMITTED" {
		t.Fatal("pre-ACK receipt did not reconcile with event-specific provider ACK")
	}
	rotatedID := senderIDs["RECOVERY_CODES_ROTATED"]
	sendState := func(token, eventName, kind, severity string) {
		t.Helper()
		payload := signedMockSecurityWebhook(t, signingKey, clock,
			strings.Repeat(token, 50), eventName, rotatedID, kind, severity)
		if status := deliverWebhook(payload); status != http.StatusOK {
			t.Fatalf("signed code-notice status callback rejected: %d", status)
		}
	}
	sendState("b", "code-delivered-after-ack", "delivered", "")
	if codeState(rotatedEvent) != "DELIVERED" {
		t.Fatal("signed delivered callback did not elevate submitted state")
	}
	sendState("c", "code-permanent-bounce", "failed", "permanent")
	if codeState(rotatedEvent) != "PERMANENT_FAILURE" {
		t.Fatal("permanent bounce did not outrank delivered")
	}
	sendState("d", "code-out-of-order-delivered", "delivered", "")
	if codeState(rotatedEvent) != "PERMANENT_FAILURE" {
		t.Fatal("late delivered callback masked a permanent bounce")
	}
	sendState("e", "code-complaint", "complained", "")
	if codeState(rotatedEvent) != "COMPLAINED" {
		t.Fatal("complaint did not escalate permanent bounce")
	}
	sendState("f", "code-unsubscribe", "unsubscribed", "")
	if codeState(rotatedEvent) != "UNSUBSCRIBED" {
		t.Fatal("unsubscribe did not retain highest severity")
	}
	unknown := signedMockSecurityWebhook(t, signingKey, clock, strings.Repeat("1", 50),
		"code-unknown-provider-msg", "unknown-code-notice@mailgun.test", "failed", "permanent")
	if status := deliverWebhook(unknown); status != http.StatusOK {
		t.Fatal("valid unmatched signed callback was not durably recorded")
	}
	health, err = ReadRecoveryCodeSecurityNoticeHealth(ctx, monitorDSN)
	if err != nil || health.ProviderFailures < 1 || health.UnmatchedEvents < 1 ||
		health.SubmittedUnconfirmed != 0 || health.Delivered < 1 {
		t.Fatalf("count-only reconciled recovery-code monitor incorrect: %+v %v", health, err)
	}
	forged := signedMockSecurityWebhook(t, []byte("invalid-separate-signing-credential"), clock,
		strings.Repeat("2", 50), "code-forged-provider-event", rotatedID, "delivered", "")
	if status := deliverWebhook(forged); status != http.StatusNotAcceptable {
		t.Fatalf("unsigned/forged provider callback accepted: %d", status)
	}
	wrongDomain := signedMockSecurityWebhook(t, signingKey, clock,
		strings.Repeat("3", 50), "code-wrong-domain", rotatedID, "delivered", "")
	wrongDomain = bytes.Replace(wrongDomain, []byte("mg.example.test"),
		[]byte("evil.example.test"), 1)
	if status := deliverWebhook(wrongDomain); status != http.StatusNotAcceptable {
		t.Fatalf("incorrect signed provider domain accepted: %d", status)
	}
	if codeState(rotatedEvent) != "UNSUBSCRIBED" {
		t.Fatal("forged callback downgraded a recovery-code notification")
	}
	if _, err := ReadRecoveryCodeSecurityNoticeHealth(ctx, noticeDSN); err == nil {
		t.Fatal("sender could use monitor credential")
	}
	// Prove lease fencing, stale-recipient screening and mandatory retention
	// using a synthetic third legitimate v25 rotation event.
	grant = freshGrant("rotation-after-acceptance")
	thirdEvent := randomSyntheticID(t, "evt_")
	third := batch("third-notice-")
	allowed, err = manager.Replace(ctx, sessionDigest, grant, second[0], third, thirdEvent)
	if err != nil || !allowed {
		t.Fatal("third v25 code rotation fixture failed")
	}
	lease1 := randomSyntheticID(t, "lse_")
	job, found, err := store.Claim(ctx, lease1)
	if err != nil || !found || job.EventID != thirdEvent || job.Kind != "RECOVERY_CODES_ROTATED" {
		t.Fatal("third security notice lease did not select correct event type")
	}
	if err := store.Finalize(ctx, thirdEvent, randomSyntheticID(t, "lse_"), usecase.DeliveryTemporary, ""); err == nil {
		t.Fatal("unfenced lease could finalize another sender's claim")
	}
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_recovery_code_notices
		SET leased_until=clock_timestamp()-interval '1 second' WHERE event_id=$1`,
		thirdEvent); err != nil {
		t.Fatal(err)
	}
	if err := store.Finalize(ctx, thirdEvent, lease1, usecase.DeliveryTemporary, ""); err == nil {
		t.Fatal("expired lease was accepted")
	}
	lease2 := randomSyntheticID(t, "lse_")
	job, found, err = store.Claim(ctx, lease2)
	if err != nil || !found || job.EventID != thirdEvent {
		t.Fatal("expired lease could not be safely reclaimed")
	}
	if err := store.Finalize(ctx, thirdEvent, lease2, usecase.DeliveryPermanent, ""); err != nil {
		t.Fatal("permanent bounce not finalized")
	}
	var state string
	if err := admin.QueryRow(ctx, `SELECT status FROM mamony_identity.password_recovery_code_notices
		WHERE event_id=$1`, thirdEvent).Scan(&state); err != nil || state != "DEAD" {
		t.Fatal("permanent delivery failure not dead-lettered")
	}
	// Erase recipient PII after seven days, but leave immutable audit.
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_recovery_code_notices
		SET occurred_at=clock_timestamp()-interval '8 days'
		WHERE event_id=ANY($1)`, []string{createdEvent, rotatedEvent}); err != nil {
		t.Fatal(err)
	}
	maintenanceDSN := guardedDB(t, "TEST_IDENTITY_MAINTENANCE_URL", "mamony_identity_maintenance_runtime")
	maintenance, err := pgx.Connect(ctx, maintenanceDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = maintenance.Close(context.Background()) }()
	var erased int
	if err := maintenance.QueryRow(ctx,
		"SELECT mamony_identity.purge_old_recovery_code_security_notices()").Scan(&erased); err != nil || erased < 2 {
		t.Fatal("bounded recipient-PII notice retention failed")
	}
	// Independent maintenance receipt purge must not touch audit or
	// rows from the v17 password-change/v23 password-recovery queues.
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.recovery_code_notice_mailgun_receipts
		SET recorded_at=clock_timestamp()-interval '8 days'
		WHERE message_digest=$1`, rotatedDigest[:]); err != nil {
		t.Fatal(err)
	}
	var receiptPurge int
	if err := maintenance.QueryRow(ctx,
		"SELECT mamony_identity.purge_old_recovery_code_notice_mailgun_receipts()").
		Scan(&receiptPurge); err != nil || receiptPurge < 5 {
		t.Fatal("bounded recovery-code receipt cleanup did not erase aged digests")
	}
	var audited int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.security_outbox
		WHERE event_id=ANY($1)`, []string{createdEvent, rotatedEvent}).Scan(&audited); err != nil || audited != 2 {
		t.Fatal("purging notice erased immutable identifier-only audit evidence")
	}
}
