//go:build integration

package identity

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/credential"
	"github.com/jackc/pgx/v5"
)

// TestIdentityMailgunMockSecurityNoticePipeline exercises a real v16
// PostgreSQL queue with a local HTTP Mailgun mock and the actual separately
// wired facade/worker. No external provider account or live email is used.
// Real old-password Argon2id reauthentication remains mandatory separately
// in TestIdentityZAuthenticatedPasswordChange.
func TestIdentityMailgunMockSecurityNoticePipeline(t *testing.T) {
	ctx, stop := context.WithTimeout(context.Background(), 90*time.Second)
	defer stop()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	grantDSN := guardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	changeDSN := guardedDB(t, "TEST_IDENTITY_PASSWORD_CHANGE_URL", "mamony_identity_password_change_runtime")
	noticeDSN := guardedDB(t, "TEST_IDENTITY_SECURITY_NOTICE_URL", "mamony_identity_security_notice_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, raw := range []string{grantDSN, sessionDSN, changeDSN, noticeDSN} {
		u, _ := url.Parse(raw)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("Mailgun security notice E2E requires one disposable DB")
		}
	}
	open := func(raw string) *pgx.Conn {
		t.Helper()
		c, err := pgx.Connect(ctx, raw)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close(context.Background()) })
		return c
	}
	admin := open(adminDSN)
	grant := open(grantDSN)
	session := open(sessionDSN)
	change := open(changeDSN)
	subject := randomSyntheticID(t, "sub_")
	email := strings.ToLower(randomSyntheticID(t, "notice_")) + "@example.test"
	event := randomSyntheticID(t, "evt_")
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if _, err := admin.Exec(cleanupCtx,
			`UPDATE mamony_identity.password_change_notices
			  SET status='DEAD',lease_id=NULL,leased_until=NULL
			  WHERE subject_id=$1 AND status='PENDING'`, subject); err != nil {
			t.Errorf("synthetic security notice cleanup failed: %v", err)
		}
	})
	kdf, err := credential.NewPasswordHasher(1)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := kdf.Hash(ctx, "synthetic security notice initial verifier")
	if err != nil {
		t.Fatal(err)
	}
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
		VALUES ($1,1,65536,3,1,$2,$3,clock_timestamp())`,
		subject, verifier.Salt, verifier.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1",
		subject); err != nil {
		t.Fatal(err)
	}
	var epoch int64
	if err := admin.QueryRow(ctx,
		"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
		subject).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	loginProof := sha256.Sum256([]byte("synthetic:notice-mailgun:login:" + subject))
	changeProof := sha256.Sum256([]byte("synthetic:notice-mailgun:change:" + subject))
	sessionDigest := sha256.Sum256([]byte("synthetic:notice-mailgun:session:" + subject))
	var approved bool
	if err := grant.QueryRow(ctx,
		"SELECT mamony_identity.stage_password_auth_grant($1,$2,$3)",
		subject, epoch, loginProof[:]).Scan(&approved); err != nil || !approved {
		t.Fatalf("could not stage test login grant: %v %v", err, approved)
	}
	if err := session.QueryRow(ctx,
		"SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
		loginProof[:], sessionDigest[:], randomSyntheticID(t, "evt_")).
		Scan(&approved); err != nil || !approved {
		t.Fatalf("could not create synthetic current-epoch session: %v %v", err, approved)
	}
	if err := grant.QueryRow(ctx,
		"SELECT mamony_identity.stage_password_change_grant($1,$2,$3)",
		subject, epoch, changeProof[:]).Scan(&approved); err != nil || !approved {
		t.Fatalf("could not stage purpose-bound synthetic change: %v %v", err, approved)
	}
	nextVerifier, err := kdf.Hash(ctx, "synthetic security notice rotated verifier")
	if err != nil {
		t.Fatal(err)
	}
	if err := change.QueryRow(ctx,
		"SELECT mamony_identity.rotate_password_verifier($1,$2,$3,$4,$5)",
		sessionDigest[:], changeProof[:], nextVerifier.Salt, nextVerifier.Digest,
		event).Scan(&approved); err != nil || !approved {
		t.Fatalf("password rotation did not atomically enqueue notice: %v %v", err, approved)
	}
	var pending int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.password_change_notices
		WHERE event_id=$1 AND recipient=$2 AND status='PENDING'`,
		event, email).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("no transactionally persisted notice after password rotation: %v %d", err, pending)
	}
	var requests atomic.Int32
	var responseCode atomic.Int32
	responseCode.Store(429)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		name, key, ok := r.BasicAuth()
		if !ok || name != "api" || key != "synthetic-security-sending-key" ||
			r.Method != http.MethodPost || r.URL.Path != "/v3/mg.example.test/messages" {
			t.Error("Mailgun mock got incorrect endpoint or sending key")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 12<<10)
		if err := r.ParseMultipartForm(8192); err != nil {
			t.Error(err)
			return
		}
		if r.FormValue("to") != email ||
			r.FormValue("subject") != "Mamony security alert: password changed" ||
			r.FormValue("o:tracking") != "no" ||
			r.FormValue("o:tracking-opens") != "no" ||
			r.FormValue("o:tracking-clicks") != "no" ||
			r.FormValue("o:require-tls") != "yes" ||
			r.FormValue("o:deliver-within") != "5m" ||
			strings.Contains(r.FormValue("text"), event) ||
			strings.Contains(r.FormValue("text"), "https://") {
			t.Error("security mail included secret/link or wrong Mailgun options")
		}
		w.Header().Set("Content-Type", "application/json")
		status := int(responseCode.Load())
		w.WriteHeader(status)
		if status == 200 {
			_, _ = fmt.Fprint(w, `{"id":"<synthetic@mailgun.test>","message":"Queued."}`)
		} else {
			_, _ = fmt.Fprint(w, `{"message":"synthetic provider retry"}`)
		}
	}))
	defer mock.Close()
	mockOrigin, err := url.Parse(mock.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := SecurityNoticeDeliveryConfig{
		DSN: noticeDSN, Region: "US", Domain: "mg.example.test",
		From: "Mamony Security <security@mg.example.test>",
		MailgunKey: "synthetic-security-sending-key",
		PollInterval: 100 * time.Millisecond,
		HTTPClient: &http.Client{
			Timeout: 2 * time.Second,
			Transport: mockMailgunTransport{origin: mockOrigin},
		},
	}
	checkState := func() (string, int) {
		t.Helper()
		var state string
		var attempts int
		if err := admin.QueryRow(ctx, `SELECT status,attempts
			FROM mamony_identity.password_change_notices WHERE event_id=$1`,
			event).Scan(&state, &attempts); err != nil {
			t.Fatal(err)
		}
		return state, attempts
	}
	runUntil := func(expected string, minAttempts int) {
		t.Helper()
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- RunSecurityNoticeDelivery(runCtx, cfg) }()
		defer cancel()
		until := time.Now().Add(6 * time.Second)
		for {
			state, attempts := checkState()
			if state == expected && attempts >= minAttempts {
				break
			}
			if time.Now().After(until) {
				t.Fatalf("mock security notice runner did not reach %s/%d: got %s/%d",
					expected, minAttempts, state, attempts)
			}
			time.Sleep(25 * time.Millisecond)
		}
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("facade runner stopped unexpectedly: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("security notice runner ignored cancellation")
		}
	}
	runUntil("PENDING", 1)
	if requests.Load() != 1 {
		t.Fatalf("expected one mocked temporary failure: %d", requests.Load())
	}
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.password_change_notices
		SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE event_id=$1`,
		event); err != nil {
		t.Fatal(err)
	}
	responseCode.Store(200)
	runUntil("ACCEPTED", 2)
	state, attempts := checkState()
	if state != "ACCEPTED" || attempts != 2 || requests.Load() != 2 {
		t.Fatalf("at-least-once mock status not persisted: %s/%d sends=%d",
			state, attempts, requests.Load())
	}
	// The provider accepted the submission; the database still has only
	// acceptance metadata, not proof that an inbox received the message.
	var events int
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM mamony_identity.security_outbox
		WHERE event_id=$1 AND event_type='PASSWORD_CHANGED'`, event).
		Scan(&events); err != nil || events != 1 {
		t.Fatal("notification worker changed or duplicated audit")
	}
	// A wrong SQL credential cannot be substituted for the notice runtime.
	badCfg := cfg
	badCfg.DSN = grantDSN
	if err := RunSecurityNoticeDelivery(ctx, badCfg); err == nil {
		t.Fatal("notice facade accepted an authentication grant database role")
	}
}
