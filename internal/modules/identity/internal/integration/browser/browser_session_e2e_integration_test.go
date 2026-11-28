//go:build integration && browser_e2e

package browser

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/handler"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/integration/internal/fixture"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/credential"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailcrypto"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/herdifirdausss/mamony-go/internal/platform/httpedge"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityZBrowserSessionE2E exercises actual Chromium cookie rules and
// the full real-Argon2id -> PostgreSQL admission -> v11 grant-bound session transaction
// chain. It MUST only run with disposable loopback credentials and a test-only
// TLS mux. None of these paths are registered in the application router.
func TestIdentityZBrowserSessionE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 140*time.Second)
	defer cancel()

	adminDSN := fixture.GuardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := fixture.GuardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	verifyDSN := fixture.GuardedDB(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	authDSN := fixture.GuardedDB(t, "TEST_IDENTITY_AUTHENTICATOR_URL", "mamony_identity_authenticator_runtime")
	grantDSN := fixture.GuardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := fixture.GuardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, dsn := range []string{regDSN, verifyDSN, authDSN, grantDSN, sessionDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("session browser E2E requires the same disposable loopback database")
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
			DSN: dsn, RuntimeRole: role, MaxConns: 3,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		return pool
	}
	regPool := open(regDSN, "mamony_identity_runtime")
	verifyPool := open(verifyDSN, "mamony_identity_verifier_runtime")
	authPool := open(authDSN, "mamony_identity_authenticator_runtime")
	grantPool := open(grantDSN, "mamony_identity_auth_grant_runtime")
	sessionPool := open(sessionDSN, "mamony_identity_session_runtime")

	reg, err := identitypg.NewRegistrationStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	proofs, err := identitypg.NewProofStore(verifyPool)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := identitypg.NewAuthenticationStore(authPool)
	if err != nil {
		t.Fatal(err)
	}
	admissionStore, err := identitypg.NewAdmissionStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	sessionStore, err := identitypg.NewOpaqueSessionStore(sessionPool)
	if err != nil {
		t.Fatal(err)
	}
	grantStore, err := identitypg.NewAuthenticationGrantStore(grantPool)
	if err != nil {
		t.Fatal(err)
	}
	grantAuthority, err := usecase.NewAuthenticationGrantAuthority(grantStore)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := usecase.NewOpaqueSessionAuthority(sessionStore)
	if err != nil {
		t.Fatal(err)
	}
	kdf, err := credential.NewPasswordHasher(1)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := usecase.NewPasswordAuthentication(reader, kdf)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := usecase.NewLoginAdmission(admissionStore, bytes.Repeat([]byte{0xd4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	passwordFlow, err := usecase.NewPasswordLoginFlow(gate, auth)
	if err != nil {
		t.Fatal(err)
	}
	login, err := usecase.NewSessionLogin(passwordFlow, grantAuthority, authority)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := handler.NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	browserHandler, err := handler.NewBrowserSessionHTTP(peer, login, authority,
		bytes.Repeat([]byte{0xc4}, 32), "https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}

	email := strings.ToLower(fixture.RandomID(t, "browser_session_")) + "@example.test"
	password := "unique synthetic password for actual Chromium session E2E"
	subject := fixture.RandomID(t, "sub_")
	verifier, err := kdf.Hash(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.CreatePendingWithObligation(ctx, usecase.PendingRegistration{
		SubjectID: subject, EventID: fixture.RandomID(t, "evt_"),
		CanonicalEmail: email, Status: "UNVERIFIED", CreatedAt: time.Now().UTC(),
		PasswordVerifier: verifier,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanupCtx,
			"UPDATE mamony_identity.email_proof_deliveries d SET status='DEAD' "+
				"FROM mamony_identity.email_verification_challenges c "+
				"WHERE c.challenge_id=d.challenge_id AND c.subject_id=$1 AND d.status='PENDING'",
			subject); err != nil {
			t.Errorf("failed to quarantine session-browser synthetic proof delivery: %v", err)
		}
	})
	sealer, err := mailcrypto.NewKeyring("session-e2e", map[string][]byte{
		"session-e2e": bytes.Repeat([]byte{0xa4}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	verify, err := usecase.NewEmailVerification(proofs, sealer)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := verify.Prepare(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	_, proofSecret := prepared.ForTrustedDelivery()
	if err := verify.Confirm(ctx, proofSecret); err != nil {
		t.Fatal(err)
	}

	// A standalone, test-only origin shell allows Chromium's own fetch and
	// cookie rules to execute. It is never registered in production httpedge.
	mux := http.NewServeMux()
	mux.Handle("POST /v1/identity/login", browserHandler)
	mux.Handle("GET /v1/identity/session", browserHandler)
	mux.Handle("GET /v1/identity/csrf", browserHandler)
	mux.Handle("POST /v1/identity/logout", browserHandler)
	mux.HandleFunc("GET /__test__/session-browser", func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.Host != "app.example.test" {
			http.Error(w, "wrong test origin", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		_, _ = w.Write([]byte("<!doctype html><html><head><title>Isolated session E2E</title></head><body>Test only</body></html>"))
	})
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	local, err := url.Parse(server.URL)
	if err != nil || local.Hostname() != "127.0.0.1" || local.Port() == "" {
		t.Fatal("test session HTTPS server must bind ephemeral loopback")
	}

	cmd := exec.CommandContext(ctx, "node", browsercheckDir+"real_browser_session_e2e.cjs")
	for _, name := range []string{"PATH", "HOME", "LANG", "TMPDIR", "PLAYWRIGHT_BROWSERS_PATH"} {
		if value := os.Getenv(name); value != "" {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	cmd.Env = append(cmd.Env,
		"MAMONY_SESSION_E2E_LOCAL_TLS="+server.URL,
		"MAMONY_SESSION_E2E_EMAIL="+email,
		"MAMONY_SESSION_E2E_PASSWORD="+password)
	output, runErr := cmd.CombinedOutput()
	logText := strings.ReplaceAll(string(output), password, "[REDACTED_PASSWORD]")
	logText = strings.ReplaceAll(logText, email, "[REDACTED_EMAIL]")
	// A browser assertion failure must not echo synthetic HttpOnly bearer or CSRF.
	logText = regexp.MustCompile("__Host-mamony_sid=[A-Za-z0-9_-]{43}").ReplaceAllString(logText, "[REDACTED_SESSION_COOKIE]")
	logText = regexp.MustCompile("\\b[A-Za-z0-9_-]{54}\\b").ReplaceAllString(logText, "[REDACTED_CSRF_TOKEN]")
	if runErr != nil {
		t.Fatalf("real Chromium session E2E failed: %v (sanitized): %s", runErr, logText)
	}
	if !strings.Contains(logText, "real Chromium cookie/CSRF/logout/uncertain-revoke: PASS") {
		t.Fatal("Chromium runner returned without session security evidence")
	}

	var created, revoked int
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM mamony_identity.security_outbox WHERE subject_id=$1 AND event_type='SESSION_CREATED'",
		subject).Scan(&created); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM mamony_identity.security_outbox WHERE subject_id=$1 AND event_type='SESSION_REVOKED'",
		subject).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if created != 2 || revoked != 2 {
		t.Fatalf("real browser transaction/audit mismatch: created=%d revoked=%d", created, revoked)
	}

	// Production listeners remain health-only even when tests construct and
	// exercise a completely separate, manually mounted TLS fixture mux.
	for _, active := range []http.Handler{httpedge.PublicHandler(), httpedge.PrivateHandler()} {
		for _, route := range []struct{ method, path string }{
			{http.MethodPost, "/v1/identity/login"},
			{http.MethodGet, "/v1/identity/session"},
			{http.MethodGet, "/v1/identity/csrf"},
			{http.MethodPost, "/v1/identity/logout"},
		} {
			got := httptest.NewRecorder()
			active.ServeHTTP(got, httptest.NewRequest(route.method,
				"https://app.example.test"+route.path, nil))
			if got.Code != http.StatusNotFound {
				t.Fatalf("test-only session path leaked into production router: %s %s",
					route.method, route.path)
			}
		}
	}
	t.Log(fmt.Sprintf("isolated session Chromium E2E: real-created=%d real-revoked=%d",
		created, revoked))
}
