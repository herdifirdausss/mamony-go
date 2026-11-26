//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/integration/internal/fixture"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityMailgunMockRecoverySecurityNotice uses disposable PostgreSQL,
// the real restricted v19 issuer and v21 redemption roles and a LOCAL fake
// HTTP Mailgun endpoint. No live email, reset route or real assurance issuer.
func TestIdentityMailgunMockRecoverySecurityNotice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminDSN := fixture.GuardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	issueDSN := fixture.GuardedDB(t, "TEST_IDENTITY_RECOVERY_ISSUER_URL", "mamony_identity_recovery_issuer_runtime")
	redeemDSN := fixture.GuardedDB(t, "TEST_IDENTITY_RECOVERY_REDEEMER_URL", "mamony_identity_recovery_redeemer_runtime")
	noticeDSN := fixture.GuardedDB(t, "TEST_IDENTITY_RECOVERY_NOTICE_URL", "mamony_identity_recovery_notice_runtime")
	monitorDSN := fixture.GuardedDB(t, "TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL", "mamony_identity_recovery_notice_monitor_runtime")
	root, _ := url.Parse(adminDSN)
	for _, raw := range []string{issueDSN, redeemDSN, noticeDSN, monitorDSN} {
		parsed, _ := url.Parse(raw)
		if parsed.Host != root.Host || parsed.Path != root.Path {
			t.Fatal("Mailgun recovery alert fixture requires one disposable DB")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: 2})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	issuer, err := identitypg.NewRecoveryIssuerStore(open(issueDSN, "mamony_identity_recovery_issuer_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	redeemer, err := identitypg.NewRecoveryRedemptionStore(open(redeemDSN, "mamony_identity_recovery_redeemer_runtime"))
	if err != nil {
		t.Fatal(err)
	}

	subject := fixture.RandomID(t, "sub_")
	email := strings.ToLower(fixture.RandomID(t, "alert_")) + "@example.test"
	challenge := fixture.RandomID(t, "rcv_")
	event := fixture.RandomID(t, "evt_")
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
		VALUES($1,1,65536,3,1,$2,$3,clock_timestamp())`,
		subject, bytes.Repeat([]byte{0x51}, 16), bytes.Repeat([]byte{0x63}, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	var epoch int64
	if err := admin.QueryRow(ctx, "SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1", subject).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	token, err := usecase.NewPasswordRecoveryProof()
	if err != nil {
		t.Fatal(err)
	}
	issued, err := issuer.Issue(ctx, usecase.RecoveryIssue{
		CanonicalEmail: email, ChallengeID: challenge, TokenDigest: token.Digest(),
		KeyID: "synthetic-notice", SealedToken: bytes.Repeat([]byte{0x37}, 71),
	})
	if err != nil || !issued {
		t.Fatal("v19 synthetic recovery issuance failed")
	}
	// An assurance grant is added by disposable test admin only. No
	// production role may issue or mutate these approvals.
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.password_recovery_assurance_grants
		(subject_id,subject_epoch,challenge_id,approved_at,expires_at)
		VALUES($1,$2,$3,clock_timestamp(),clock_timestamp()+interval '9 minutes')`, subject, epoch, challenge); err != nil {
		t.Fatal(err)
	}
	allowed, err := redeemer.Redeem(ctx, token.Digest(), email, domain.PasswordVerifier{
		Version: 1, MemoryKiB: 65536, Iterations: 3, Parallelism: 1,
		Salt: bytes.Repeat([]byte{0x72}, 16), Digest: bytes.Repeat([]byte{0x84}, 32),
	}, event)
	if err != nil || !allowed {
		t.Fatal("v21 synthetic redemption did not enqueue a recovery alert")
	}
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 4*time.Second)
		defer done()
		conn, err := pgx.Connect(cleanupCtx, adminDSN)
		if err != nil {
			t.Errorf("cleanup connect: %v", err)
			return
		}
		defer func() { _ = conn.Close(context.Background()) }()
		for _, sql := range []string{
			"UPDATE mamony_identity.password_recovery_notices SET status='DEAD',lease_id=NULL,leased_until=NULL WHERE event_id=$1 AND status='PENDING'",
			"UPDATE mamony_identity.password_recovery_deliveries SET status='DEAD',lease_id=NULL,leased_until=NULL WHERE challenge_id=$1 AND status='PENDING'",
		} {
			arg := event
			if strings.Contains(sql, "deliveries") {
				arg = challenge
			}
			if _, err := conn.Exec(cleanupCtx, sql, arg); err != nil {
				t.Errorf("recovery alert fixture cleanup: %v", err)
			}
		}
	})
	var pending int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.password_recovery_notices
		WHERE event_id=$1 AND recipient=$2 AND status='PENDING'`, event, email).Scan(&pending); err != nil || pending != 1 {
		t.Fatal("v21 did not atomically queue token-free recovery alert")
	}

	var calls atomic.Int32
	var statusCode atomic.Int32
	statusCode.Store(429)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		user, key, authorized := req.BasicAuth()
		if !authorized || user != "api" || key != "synthetic-recovery-alert-key" ||
			req.Method != http.MethodPost || req.URL.Path != "/v3/mg.example.test/messages" {
			t.Error("recovery notice used incorrect Mailgun authorization/endpoint")
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
		body := fields["text"]
		if fields["to"] != email || fields["subject"] != "Mamony security alert: password recovered" ||
			fields["o:tracking"] != "no" || fields["o:tracking-clicks"] != "no" ||
			fields["o:tracking-opens"] != "no" || fields["o:require-tls"] != "yes" ||
			fields["o:deliver-within"] != "5m" || !strings.Contains(body, "account recovery") ||
			strings.Contains(body, event) || strings.Contains(body, challenge) ||
			strings.Contains(body, token.ForTrustedDelivery()) ||
			strings.Contains(body, "https://") || strings.Contains(body, "http://") {
			t.Error("Mailgun recovery security alert exposed a token/link or lacked privacy options")
		}
		w.Header().Set("Content-Type", "application/json")
		code := int(statusCode.Load())
		w.WriteHeader(code)
		if code == 200 {
			_, _ = fmt.Fprint(w, `{"id":"<fixture-recovery-notice@mailgun.test>"}`)
		} else {
			_, _ = fmt.Fprint(w, `{"message":"redact-this-provider-retry"}`)
		}
	}))
	defer mock.Close()
	mockOrigin, err := url.Parse(mock.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := identity.RecoverySecurityNoticeConfig{
		DSN: noticeDSN, Region: "US", Domain: "mg.example.test",
		From:         "Mamony Security <security@mg.example.test>",
		MailgunKey:   "synthetic-recovery-alert-key",
		PollInterval: 100 * time.Millisecond,
		HTTPClient:   &http.Client{Timeout: 2 * time.Second, Transport: fixture.MailgunTransport{Origin: mockOrigin}},
	}

	check := func() (string, int, []byte) {
		t.Helper()
		var state string
		var attempts int
		var digest []byte
		if err := admin.QueryRow(ctx, `SELECT status,attempts,provider_message_digest
			FROM mamony_identity.password_recovery_notices WHERE event_id=$1`,
			event).Scan(&state, &attempts, &digest); err != nil {
			t.Fatal(err)
		}
		return state, attempts, digest
	}
	runUntil := func(target string, wantAttempts int) {
		t.Helper()
		runCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- identity.RunRecoverySecurityNotices(runCtx, cfg) }()
		deadline := time.Now().Add(7 * time.Second)
		for {
			state, attempts, _ := check()
			if state == target && attempts >= wantAttempts {
				break
			}
			if time.Now().After(deadline) {
				stop()
				t.Fatalf("recovery notice worker stuck: %s/%d want %s/%d", state, attempts, target, wantAttempts)
			}
			time.Sleep(25 * time.Millisecond)
		}
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("recovery notice runner cancellation: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("recovery notice runner did not stop")
		}
	}
	runUntil("PENDING", 1)
	if calls.Load() != 1 {
		t.Fatal("temporary Mailgun response must attempt once")
	}
	health, err := identity.ReadRecoverySecurityNoticeHealth(ctx, monitorDSN)
	if err != nil || health.Retrying < 1 {
		t.Fatal("recovery notice count-only health did not observe retry")
	}
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_recovery_notices
		SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE event_id=$1`, event); err != nil {
		t.Fatal(err)
	}
	statusCode.Store(200)
	runUntil("ACCEPTED", 2)
	state, attempts, digest := check()
	expected := sha256.Sum256([]byte("fixture-recovery-notice@mailgun.test"))
	if state != "ACCEPTED" || attempts != 2 || calls.Load() != 2 ||
		!bytes.Equal(digest, expected[:]) {
		t.Fatal("Mailgun acceptance was not durably correlated by message digest")
	}
	health, err = identity.ReadRecoverySecurityNoticeHealth(ctx, monitorDSN)
	if err != nil || health.SubmittedUnconfirmed < 1 {
		t.Fatal("recovery notice monitoring falsely omitted provider-submitted unknown delivery")
	}
	// An accepted HTTP response is not evidence of mailbox delivery.
	if err := identity.RunRecoverySecurityNotices(ctx, identity.RecoverySecurityNoticeConfig{
		DSN: monitorDSN, Region: "US", Domain: "mg.example.test",
		From: "security@mg.example.test", MailgunKey: "synthetic-recovery-alert-key",
		PollInterval: 100 * time.Millisecond, HTTPClient: cfg.HTTPClient,
	}); err == nil {
		t.Fatal("recovery notice worker accepted monitor-only credential")
	}
	if _, err := identity.ReadRecoverySecurityNoticeHealth(ctx, noticeDSN); err == nil {
		t.Fatal("count-only monitor facade accepted sender DB role")
	}
}
