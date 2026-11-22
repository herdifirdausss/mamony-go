package handler

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func passwordPageRequest(path string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "https://app.example.test"+path, nil)
	r.TLS = &tls.ConnectionState{}
	r.Host = "app.example.test"
	return r
}

func TestPasswordChangePageCSPAndManualOneShotControls(t *testing.T) {
	browser, _, _, _ := browserFixture(t)
	page, err := NewPasswordChangePage(browser)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	page.ServeHTTP(w, passwordPageRequest(passwordChangePagePath))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" ||
		w.Header().Get("Referrer-Policy") != "no-referrer" ||
		w.Header().Get("X-Frame-Options") != "DENY" ||
		w.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
		w.Header().Get("Set-Cookie") != "" {
		t.Fatal("sensitive browser page missing mandatory private headers")
	}
	script, err := passwordChangePageAssets.ReadFile("password_change_page.js")
	if err != nil {
		t.Fatal(err)
	}
	css, err := passwordChangePageAssets.ReadFile("password_change_page.css")
	if err != nil {
		t.Fatal(err)
	}
	sh := sha256.Sum256(script)
	ch := sha256.Sum256(css)
	csp := w.Header().Get("Content-Security-Policy")
	for _, expected := range []string{
		"default-src 'none'",
		"script-src 'sha256-" + base64.StdEncoding.EncodeToString(sh[:]) + "'",
		"style-src 'sha256-" + base64.StdEncoding.EncodeToString(ch[:]) + "'",
		"connect-src 'self'",
		"form-action 'none'",
		"frame-ancestors 'none'",
		"worker-src 'none'",
	} {
		if !strings.Contains(csp, expected) {
			t.Fatalf("page CSP missing required directive %q", expected)
		}
	}
	for _, unsafe := range []string{"unsafe-inline", "unsafe-eval", "data:", "blob:", "http:", "https:"} {
		if strings.Contains(csp, unsafe) {
			t.Fatalf("page CSP permits unwanted source %q", unsafe)
		}
	}
	html := w.Body.String()
	start := strings.Index(html, "<script>")
	end := strings.Index(html, "</script>")
	style := strings.Index(html, "<style>")
	if start < 0 || end <= start || style <= end ||
		html[start+len("<script>"):end] != string(script) ||
		!strings.Contains(html, "<form id=\"change-form\" hidden") ||
		!strings.Contains(html, "role=\"status\" aria-live=\"polite\"") ||
		!strings.Contains(html, "type=\"password\" autocomplete=\"current-password\"") ||
		!strings.Contains(html, "type=\"password\" autocomplete=\"new-password\"") {
		t.Fatal("password page must ship only embedded resources and accessible controls")
	}
	for _, forbidden := range []string{"<script src=", "<iframe", "<img ", "<link ", "http://", "https://",
		"localStorage", "sessionStorage", "document.cookie", "mailto:"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("browser page accidentally used unsafe resource/storage: %q", forbidden)
		}
	}
	js := string(script)
	if strings.Contains(js, "localStorage") || strings.Contains(js, "sessionStorage") ||
		strings.Contains(js, "window.location") || strings.Contains(js, "document.cookie") ||
		!strings.Contains(js, "form.addEventListener(\"submit\"") ||
		!strings.Contains(js, "event.preventDefault()") ||
		!strings.Contains(js, "credentials: \"same-origin\"") ||
		!strings.Contains(js, "redirect: \"error\"") ||
		!strings.Contains(js, "cache: \"no-store\"") ||
		!strings.Contains(js, "lock(message.uncertain)") ||
		!strings.Contains(js, "current.value = \"\"") ||
		!strings.Contains(js, "replacement.value = \"\"") {
		t.Fatal("password page must clear credentials and lock on uncertain outcome")
	}
}

func TestPasswordChangePageRejectsUntrustedNavigation(t *testing.T) {
	browser, _, _, _ := browserFixture(t)
	page, err := NewPasswordChangePage(browser)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		path   string
		edit   func(*http.Request)
		status int
	}{
		{"wrong route", "/account/missing", nil, 404},
		{"wrong method", passwordChangePagePath, func(r *http.Request) { r.Method = http.MethodPost }, 405},
		{"URL query", passwordChangePagePath + "?password=secret", nil, 403},
		{"empty query", passwordChangePagePath + "?", nil, 403},
		{"fragment", passwordChangePagePath, func(r *http.Request) { r.URL.Fragment = "secret" }, 403},
		{"insecure request", passwordChangePagePath, func(r *http.Request) { r.TLS = nil }, 403},
		{"spoof host", passwordChangePagePath, func(r *http.Request) { r.Host = "evil.test" }, 403},
		{"cross origin", passwordChangePagePath, func(r *http.Request) { r.Header.Set("Origin", "https://evil.test") }, 403},
		{"duplicate origin", passwordChangePagePath, func(r *http.Request) { r.Header.Add("Origin", "https://app.example.test"); r.Header.Add("Origin", "https://app.example.test") }, 403},
		{"cross-site metadata", passwordChangePagePath, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, 403},
		{"forwarded proto", passwordChangePagePath, func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") }, 403},
		{"forwarded chain", passwordChangePagePath, func(r *http.Request) { r.Header.Set("X-Forwarded-For", "203.0.113.6") }, 403},
		{"authorization", passwordChangePagePath, func(r *http.Request) { r.Header.Set("Authorization", "Bearer fake") }, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := passwordPageRequest(tc.path)
			if tc.edit != nil {
				tc.edit(req)
			}
			w := httptest.NewRecorder()
			page.ServeHTTP(w, req)
			if w.Code != tc.status || w.Body.Len() != 0 ||
				w.Header().Get("Set-Cookie") != "" ||
				strings.Contains(w.Header().Get("Content-Security-Policy"), "sha256-") {
				t.Fatalf("rejected navigation rendered page: code=%d want=%d", w.Code, tc.status)
			}
		})
	}
	// The navigation can carry a session cookie without revealing it in the
	// response. The JS still must check session and obtain session-bound CSRF.
	req := passwordPageRequest(passwordChangePagePath)
	req.Header.Set("Cookie", "__Host-mamony_sid=opaque-synthetic-session")
	w := httptest.NewRecorder()
	page.ServeHTTP(w, req)
	if w.Code != 200 || strings.Contains(w.Body.String(), "opaque-synthetic-session") {
		t.Fatal("page leaked the ambient session in its HTML")
	}
	if _, err := NewPasswordChangePage(nil); err == nil {
		t.Fatal("page constructor accepted a missing trusted browser boundary")
	}
	var absent *PasswordChangePage
	w = httptest.NewRecorder()
	absent.ServeHTTP(w, passwordPageRequest(passwordChangePagePath))
	if w.Code != 503 || w.Body.Len() != 0 {
		t.Fatal("unconfigured candidate page must fail closed")
	}
}
