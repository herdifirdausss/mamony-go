//go:build integration && browser_e2e

package identity

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

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/handler"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/credential"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/herdifirdausss/mamony-go/internal/platform/httpedge"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

type browserRotationAccount struct {
	subject string
	email   string
	old     string
	next    string
	epoch   int64
}

// TestIdentityZBrowserPasswordChangeE2E performs two real Chromium flows
// against v18 disposable PostgreSQL: certain response and lost response AFTER
// a committed password rotation. The handler is mounted only in the local TLS
// fixture; both shipped application listeners must keep returning 404.
func TestIdentityZBrowserPasswordChangeE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 220*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := guardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	authDSN := guardedDB(t, "TEST_IDENTITY_AUTHENTICATOR_URL", "mamony_identity_authenticator_runtime")
	grantDSN := guardedDB(t, "TEST_IDENTITY_AUTH_GRANT_URL", "mamony_identity_auth_grant_runtime")
	sessionDSN := guardedDB(t, "TEST_IDENTITY_SESSION_URL", "mamony_identity_session_runtime")
	changeDSN := guardedDB(t, "TEST_IDENTITY_PASSWORD_CHANGE_URL", "mamony_identity_password_change_runtime")

	adminURL, _ := url.Parse(adminDSN)
	for _, dsn := range []string{regDSN, authDSN, grantDSN, sessionDSN, changeDSN} {
		parsed, _ := url.Parse(dsn)
		if parsed.Host != adminURL.Host || parsed.Path != adminURL.Path {
			t.Fatal("real browser password rotation requires one disposable loopback database")
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
	authPool := open(authDSN, "mamony_identity_authenticator_runtime")
	grantPool := open(grantDSN, "mamony_identity_auth_grant_runtime")
	sessionPool := open(sessionDSN, "mamony_identity_session_runtime")
	changePool := open(changeDSN, "mamony_identity_password_change_runtime")

	kdf, err := credential.NewPasswordHasher(1)
	if err != nil {
		t.Fatal(err)
	}
	authStore, err := identitypg.NewAuthenticationStore(authPool)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := usecase.NewPasswordAuthentication(authStore, kdf)
	if err != nil {
		t.Fatal(err)
	}
	admissionStore, err := identitypg.NewAdmissionStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := usecase.NewLoginAdmission(admissionStore, bytes.Repeat([]byte{0xb4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	passwordFlow, err := usecase.NewPasswordLoginFlow(gate, auth)
	if err != nil {
		t.Fatal(err)
	}
	grantStore, err := identitypg.NewAuthenticationGrantStore(grantPool)
	if err != nil {
		t.Fatal(err)
	}
	grants, err := usecase.NewAuthenticationGrantAuthority(grantStore)
	if err != nil {
		t.Fatal(err)
	}
	sessionStore, err := identitypg.NewOpaqueSessionStore(sessionPool)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := usecase.NewOpaqueSessionAuthority(sessionStore)
	if err != nil {
		t.Fatal(err)
	}
	login, err := usecase.NewSessionLogin(passwordFlow, grants, sessions)
	if err != nil {
		t.Fatal(err)
	}
	rotationStore, err := identitypg.NewPasswordChangeStore(changePool)
	if err != nil {
		t.Fatal(err)
	}
	change, err := usecase.NewAuthenticatedPasswordChange(
		sessions, passwordFlow, grants, kdf, rotationStore,
		domain.PasswordPolicy{IsBlocked: func(v, _ string) bool { return v == "synthetic blocked replacement phrase" }},
	)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := handler.NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	browser, err := handler.NewBrowserSessionHTTP(peer, login, sessions,
		bytes.Repeat([]byte{0xc4}, 32), "https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}
	browserChange, err := handler.NewBrowserPasswordChangeHTTP(browser, change)
	if err != nil {
		t.Fatal(err)
	}
	pageHandler, err := handler.NewPasswordChangePage(browser)
	if err != nil {
		t.Fatal(err)
	}

	accounts := []browserRotationAccount{
		{subject: randomSyntheticID(t, "sub_"),
			email: strings.ToLower(randomSyntheticID(t, "rotate_a_")) + "@example.test",
			old:   "synthetic original phrase for first Chromium rotation",
			next:  "synthetic updated phrase for first Chromium rotation"},
		{subject: randomSyntheticID(t, "sub_"),
			email: strings.ToLower(randomSyntheticID(t, "rotate_b_")) + "@example.test",
			old:   "synthetic original phrase for lost-response Chromium",
			next:  "synthetic updated phrase for lost-response Chromium"},
	}
	for i := range accounts {
		account := &accounts[i]
		verifier, err := kdf.Hash(ctx, account.old)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx,
			"INSERT INTO mamony_identity.subjects (subject_id,created_at) VALUES ($1,clock_timestamp())",
			account.subject); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx,
			"INSERT INTO mamony_identity.email_aliases (canonical_email,subject_id,verified_at,created_at) "+
				"VALUES ($1,$2,clock_timestamp(),clock_timestamp())",
			account.email, account.subject); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx,
			"INSERT INTO mamony_identity.password_verifiers "+
				"(subject_id,kdf_version,memory_kib,iterations,parallelism,salt,digest,created_at) "+
				"VALUES ($1,1,65536,3,1,$2,$3,clock_timestamp())",
			account.subject, verifier.Salt, verifier.Digest); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx,
			"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1",
			account.subject); err != nil {
			t.Fatal(err)
		}
		if err := admin.QueryRow(ctx,
			"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
			account.subject).Scan(&account.epoch); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		for _, account := range accounts {
			if _, err := admin.Exec(cleanup,
				"UPDATE mamony_identity.password_change_notices SET status='DEAD',lease_id=NULL,leased_until=NULL "+
					"WHERE subject_id=$1 AND status='PENDING'",
				account.subject); err != nil {
				t.Errorf("failed to quarantine synthetic password notice: %v", err)
			}
		}
	})

	// Explicitly register ONLY with the disposable TLS mux. No production
	// bootstrap, public HTTP route, actual provider sender or webhook is used.
	mux := http.NewServeMux()
	mux.Handle("POST /v1/identity/login", browser)
	mux.Handle("GET /v1/identity/session", browser)
	mux.Handle("GET /v1/identity/csrf", browser)
	mux.Handle("POST /v1/identity/logout", browser)
	mux.Handle("POST /v1/identity/change-password", browserChange)
	mux.Handle("GET /account/change-password", pageHandler)
	mux.HandleFunc("GET /__test__/password-browser", func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.Host != "app.example.test" {
			http.Error(w, "wrong test origin", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		_, _ = w.Write([]byte("<!doctype html><html><head><title>Isolated password change E2E</title></head><body>Test only</body></html>"))
	})
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	local, err := url.Parse(server.URL)
	if err != nil || local.Hostname() != "127.0.0.1" || local.Port() == "" {
		t.Fatal("password browser E2E TLS server must bind ephemeral loopback")
	}

	cmd := exec.CommandContext(ctx, "node", "../../../tools/browsercheck/real_browser_password_change_e2e.cjs")
	for _, name := range []string{"PATH", "HOME", "LANG", "TMPDIR", "PLAYWRIGHT_BROWSERS_PATH"} {
		if value := os.Getenv(name); value != "" {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	cmd.Env = append(cmd.Env,
		"MAMONY_ROTATION_E2E_LOCAL_TLS="+server.URL,
		"MAMONY_ROTATION_E2E_EMAIL_A="+accounts[0].email,
		"MAMONY_ROTATION_E2E_OLD_A="+accounts[0].old,
		"MAMONY_ROTATION_E2E_NEXT_A="+accounts[0].next,
		"MAMONY_ROTATION_E2E_EMAIL_B="+accounts[1].email,
		"MAMONY_ROTATION_E2E_OLD_B="+accounts[1].old,
		"MAMONY_ROTATION_E2E_NEXT_B="+accounts[1].next)
	output, runErr := cmd.CombinedOutput()
	logText := string(output)
	for _, account := range accounts {
		for _, value := range []string{account.email, account.old, account.next} {
			logText = strings.ReplaceAll(logText, value, "[REDACTED_FIXTURE]")
		}
	}
	logText = regexp.MustCompile("__Host-mamony_sid=[A-Za-z0-9_-]{43}").
		ReplaceAllString(logText, "[REDACTED_SESSION_COOKIE]")
	logText = regexp.MustCompile("\\b[A-Za-z0-9_-]{54}\\b").
		ReplaceAllString(logText, "[REDACTED_CSRF]")
	if runErr != nil {
		t.Fatalf("real Chromium password-change E2E failed: %v (sanitized): %s", runErr, logText)
	}
	if !strings.Contains(logText, "real Chromium password-change/epoch/notices/lost-response: PASS") {
		t.Fatal("Chromium runner did not report the required password-change evidence")
	}

	for _, account := range accounts {
		var epoch int64
		if err := admin.QueryRow(ctx, "SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
			account.subject).Scan(&epoch); err != nil {
			t.Fatal(err)
		}
		if epoch != account.epoch+1 {
			t.Fatalf("password rotation failed to increment session epoch exactly once: %d -> %d",
				account.epoch, epoch)
		}
		var events, notices int
		if err := admin.QueryRow(ctx,
			"SELECT count(*) FROM mamony_identity.security_outbox WHERE subject_id=$1 AND event_type='PASSWORD_CHANGED'",
			account.subject).Scan(&events); err != nil {
			t.Fatal(err)
		}
		if err := admin.QueryRow(ctx,
			"SELECT count(*) FROM mamony_identity.password_change_notices n "+
				"JOIN mamony_identity.security_outbox o USING(event_id) "+
				"WHERE n.subject_id=$1 AND n.recipient=$2 AND n.status='PENDING' "+
				"AND o.event_type='PASSWORD_CHANGED'",
			account.subject, account.email).Scan(&notices); err != nil {
			t.Fatal(err)
		}
		if events != 1 || notices != 1 {
			t.Fatalf("password change must create exactly one atomic audit and notification: events=%d notices=%d",
				events, notices)
		}
	}
	for _, active := range []http.Handler{httpedge.PublicHandler(), httpedge.PrivateHandler()} {
		for _, route := range []struct{ method, path string }{
			{http.MethodPost, "/v1/identity/change-password"},
			{http.MethodGet, "/account/change-password"},
		} {
			rr := httptest.NewRecorder()
			active.ServeHTTP(rr, httptest.NewRequest(route.method,
				"https://app.example.test"+route.path, nil))
			if rr.Code != http.StatusNotFound {
				t.Fatal("browser password-change candidate leaked into an application router")
			}
		}
	}
	t.Log(fmt.Sprintf("real Chromium password rotation E2E: subjects=%d, exactly one event+notice each", len(accounts)))
}
