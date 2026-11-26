//go:build integration && browser_e2e

package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/handler"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/integration/internal/fixture"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailcrypto"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailgun"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/herdifirdausss/mamony-go/internal/platform/httpedge"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// This suite is purposefully NOT in ordinary go test or in the public router.
// It requires a disposable loopback PostgreSQL fixture already prepared by
// make integration, the pinned Chromium browser and a test-only TLS route
// bridge. The browser genuinely executes the embedded page JS and CSP,
// while the bridge translates app.example.test to an ephemeral local listener.
func TestIdentityZBrowserVerificationE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 125*time.Second)
	defer cancel()

	adminDSN := fixture.GuardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := fixture.GuardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	verifyDSN := fixture.GuardedDB(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	deliveryDSN := fixture.GuardedDB(t, "TEST_IDENTITY_DELIVERY_URL", "mamony_identity_delivery_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, dsn := range []string{regDSN, verifyDSN, deliveryDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("E2E role DSNs must refer to the exact same disposable loopback database")
		}
	}

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		pool, err := platformpg.Open(ctx, platformpg.Config{
			DSN: dsn, RuntimeRole: role, MaxConns: 4,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		return pool
	}
	regPool := open(regDSN, "mamony_identity_runtime")
	proofPool := open(verifyDSN, "mamony_identity_verifier_runtime")
	deliveryPool := open(deliveryDSN, "mamony_identity_delivery_runtime")
	regStore, err := identitypg.NewStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	admissionStore, err := identitypg.NewAdmissionStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	proofStore, err := identitypg.NewProofStore(proofPool)
	if err != nil {
		t.Fatal(err)
	}
	deliveryStore, err := identitypg.NewDeliveryStore(deliveryPool)
	if err != nil {
		t.Fatal(err)
	}
	// Existing end-to-end tests may leave PENDING synthetic jobs after a
	// failure. This operation is permitted ONLY against a guarded disposable
	// database; never use a real provider, delete a proof or alter an account.
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_proof_deliveries
		SET status='DEAD' WHERE status='PENDING'`); err != nil {
		t.Fatal(err)
	}

	keyring, err := mailcrypto.NewKeyring("browser-e2e", map[string][]byte{
		"browser-e2e": bytes.Repeat([]byte{0x9c}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := usecase.NewRegistration(regStore, fixture.EnrollmentHasher{},
		domain.PasswordPolicy{IsBlocked: func(string, string) bool { return false }})
	if err != nil {
		t.Fatal(err)
	}
	proofs, err := usecase.NewEmailVerification(proofStore, keyring)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := usecase.NewEnrollmentFlow(reg, proofStore, proofs)
	if err != nil {
		t.Fatal(err)
	}
	anonymous, err := usecase.NewAnonymousAdmission(admissionStore, bytes.Repeat([]byte{0x51}, 32))
	if err != nil {
		t.Fatal(err)
	}
	confirmAdmission, err := usecase.NewConfirmationAdmission(admissionStore, bytes.Repeat([]byte{0x52}, 32))
	if err != nil {
		t.Fatal(err)
	}
	peer, err := handler.NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	anonBoundary, err := handler.NewAnonymousBoundary(peer, anonymous)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := handler.NewEnrollmentHTTP(anonBoundary, flow, "https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}
	confirmation, err := handler.NewEmailConfirmationHTTP(peer, confirmAdmission, proofs,
		"https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}
	page, err := handler.NewEmailVerificationPage("https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}
	// A deliberately scoped test-only mux: the application's actual
	// PublicHandler/PrivateHandler are not modified or mounted.
	mux := http.NewServeMux()
	mux.Handle("POST /v1/identity/register", enrollment)
	mux.Handle("POST /v1/identity/verify-email", confirmation)
	mux.Handle("GET /verify-email", page)
	tlsServer := httptest.NewTLSServer(mux)
	defer tlsServer.Close()
	localURL, err := url.Parse(tlsServer.URL)
	if err != nil || localURL.Hostname() != "127.0.0.1" || localURL.Port() == "" {
		t.Fatal("E2E test HTTPS listener must be an ephemeral loopback socket")
	}
	browserHTTPSClient := tlsServer.Client()

	type proofCase struct {
		email, subject, link string
	}
	cases := make([]proofCase, 3)
	for i := range cases {
		email := strings.ToLower(fixture.RandomID(t, "browser_")) + "@example.test"
		body, err := json.Marshal(map[string]string{
			"email": email, "password": "synthetic browser e2e password phrase",
		})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, tlsServer.URL+"/v1/identity/register",
			bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "app.example.test"
		req.Header.Set("Origin", "https://app.example.test")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Content-Type", "application/json")
		reply, err := browserHTTPSClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = reply.Body.Close()
		if reply.StatusCode != http.StatusAccepted {
			t.Fatalf("test-only HTTPS registration returned %d", reply.StatusCode)
		}
		var subject string
		err = admin.QueryRow(ctx, `SELECT s.subject_id FROM mamony_identity.subjects s
			JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
			WHERE a.canonical_email=$1 AND s.status='UNVERIFIED'`, email).Scan(&subject)
		if err != nil {
			t.Fatalf("registration didn't persist one unverified account: %v", err)
		}
		cases[i] = proofCase{email: email, subject: subject}
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		for _, record := range cases {
			if _, err := admin.Exec(cleanupCtx, `UPDATE mamony_identity.email_proof_deliveries d
				SET status='DEAD' FROM mamony_identity.email_verification_challenges c
				WHERE c.challenge_id=d.challenge_id AND c.subject_id=$1
				  AND d.status='PENDING'`, record.subject); err != nil {
				t.Errorf("failed to quarantine browser fixture delivery: %v", err)
			}
		}
	})

	// Real encrypted delivery store + bounded worker send the email template
	// to a local Mailgun API mock; no external network or real key is used.
	linkPattern := regexp.MustCompile(`https://app[.]example[.]test/verify-email#token=[A-Za-z0-9_-]{43}`)
	var mailMu sync.Mutex
	mailLinks := make(map[string]string)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "api" || password != "synthetic-mailgun-key" ||
			r.Method != http.MethodPost || r.URL.Path != "/v3/mg.example.test/messages" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if err := r.ParseMultipartForm(8192); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		to := r.FormValue("to")
		if r.FormValue("o:tracking") != "no" ||
			r.FormValue("o:tracking-clicks") != "no" ||
			r.FormValue("o:tracking-opens") != "no" ||
			r.FormValue("o:require-tls") != "yes" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		matches := linkPattern.FindAllString(r.FormValue("text"), -1)
		if len(matches) != 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mailMu.Lock()
		mailLinks[to] = matches[0]
		mailMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"<browser-mock@mailgun.test>","message":"Queued"}`))
	}))
	defer mock.Close()
	mockURL, err := url.Parse(mock.URL)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := mailgun.NewSender(mailgun.Config{
		Region:          mailgun.RegionUS,
		Domain:          "mg.example.test",
		From:            "Mamony Security <security@mg.example.test>",
		APIKey:          "synthetic-mailgun-key",
		VerificationURL: "https://app.example.test/verify-email",
	}, &http.Client{
		Timeout:   3 * time.Second,
		Transport: fixture.MailgunTransport{Origin: mockURL},
	})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := usecase.NewProofDeliveryWorker(deliveryStore, keyring, sender)
	if err != nil {
		t.Fatal(err)
	}
	for range cases {
		worked, err := worker.DispatchOnce(ctx)
		if err != nil || !worked {
			t.Fatalf("mock Mailgun dispatch failed (worked=%t): %v", worked, err)
		}
	}
	mailMu.Lock()
	for i := range cases {
		cases[i].link = mailLinks[cases[i].email]
		if !linkPattern.MatchString(cases[i].link) {
			mailMu.Unlock()
			t.Fatal("Mailgun mock did not receive a valid first-party verification link")
		}
	}
	if len(mailLinks) != len(cases) {
		mailMu.Unlock()
		t.Fatal("unexpected number of provider delivery recipients")
	}
	mailMu.Unlock()

	// The third link is expired by the authoritative DB clock, not by JS.
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_verification_challenges
		SET requested_at=clock_timestamp()-interval '30 minutes',
		    expires_at=clock_timestamp()-interval '1 minute'
		WHERE subject_id=$1`, cases[2].subject); err != nil {
		t.Fatal(err)
	}
	assertState := func(subject, want string, events int) {
		t.Helper()
		var status string
		var verified bool
		var count int
		err := admin.QueryRow(ctx, `SELECT s.status, a.verified_at IS NOT NULL,
			(SELECT count(*) FROM mamony_identity.security_outbox o
			WHERE o.subject_id=s.subject_id AND o.event_type='CONTACT_VERIFIED')
			FROM mamony_identity.subjects s
			JOIN mamony_identity.email_aliases a USING(subject_id)
			WHERE s.subject_id=$1`, subject).Scan(&status, &verified, &count)
		if err != nil || status != want || verified != (want == "ACTIVE") || count != events {
			t.Fatalf("Identity authority state mismatch: status=%s verified=%t events=%d err=%v",
				status, verified, count, err)
		}
	}
	for _, record := range cases {
		assertState(record.subject, "UNVERIFIED", 0)
	}

	cmd := exec.CommandContext(ctx, "node", browsercheckDir+"real_browser_e2e.cjs")
	for _, name := range []string{"PATH", "HOME", "LANG", "TMPDIR", "PLAYWRIGHT_BROWSERS_PATH"} {
		if value := os.Getenv(name); value != "" {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	cmd.Env = append(cmd.Env,
		"MAMONY_E2E_LOCAL_TLS="+tlsServer.URL,
		"MAMONY_E2E_LINK_SUCCESS="+cases[0].link,
		"MAMONY_E2E_LINK_AMBIGUOUS="+cases[1].link,
		"MAMONY_E2E_LINK_EXPIRED="+cases[2].link)
	output, runErr := cmd.CombinedOutput()
	logText := string(output)
	for _, record := range cases {
		// Never render the synthetic bearer or full URL, even on browser
		// assertion failures, Playwright traces or unexpected crashes.
		logText = strings.ReplaceAll(logText, record.link, "[REDACTED_LINK]")
		parsed, err := url.Parse(record.link)
		if err == nil {
			logText = strings.ReplaceAll(logText, strings.TrimPrefix(parsed.Fragment, "token="), "[REDACTED_TOKEN]")
		}
	}
	if runErr != nil {
		t.Fatalf("real Chromium end-to-end verification failed: %v (sanitized): %s", runErr, logText)
	}
	if !strings.Contains(logText, "real Chromium HTTPS-origin/CSP/GET/POST verification: PASS") {
		t.Fatal("real Chromium runner returned no positive proof of completion")
	}
	for i, record := range cases {
		if i < 2 {
			assertState(record.subject, "ACTIVE", 1)
		} else {
			assertState(record.subject, "UNVERIFIED", 0)
		}
	}
	// Lost response after commit makes a replay definitely invalid, not a
	// second activation or reason to resend. Verify HTTP and audit evidence.
	linkURL, _ := url.Parse(cases[1].link)
	bearer := strings.TrimPrefix(linkURL.Fragment, "token=")
	jsonBody, _ := json.Marshal(map[string]string{"token": bearer})
	replay, err := http.NewRequestWithContext(ctx, http.MethodPost,
		tlsServer.URL+"/v1/identity/verify-email", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatal(err)
	}
	replay.Host = "app.example.test"
	replay.Header.Set("Origin", "https://app.example.test")
	replay.Header.Set("Sec-Fetch-Site", "same-origin")
	replay.Header.Set("Content-Type", "application/json")
	reply, err := browserHTTPSClient.Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	_ = reply.Body.Close()
	if reply.StatusCode != http.StatusBadRequest {
		t.Fatalf("already-committed ambiguous proof replay did not reject with 400: %d", reply.StatusCode)
	}
	assertState(cases[1].subject, "ACTIVE", 1)

	// The running public and private HTTP routers are still health-only.
	for _, active := range []http.Handler{httpedge.PublicHandler(), httpedge.PrivateHandler()} {
		for _, route := range []struct{ method, path string }{
			{http.MethodGet, "/verify-email"},
			{http.MethodPost, "/v1/identity/register"},
			{http.MethodPost, "/v1/identity/verify-email"},
		} {
			check := httptest.NewRecorder()
			active.ServeHTTP(check, httptest.NewRequest(route.method,
				"https://app.example.test"+route.path, nil))
			if check.Code != http.StatusNotFound {
				t.Fatalf("test-only route leaked into running router: %s %s", route.method, route.path)
			}
		}
	}
	t.Log(fmt.Sprintf("isolated browser integration: verified=%d ambiguous-committed=%d expired-denied=%d",
		1, 1, 1))
}
