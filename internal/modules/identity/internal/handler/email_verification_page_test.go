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

func testVerificationPageRequest(path string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "https://app.example.test"+path, nil)
	r.Host = "app.example.test"
	r.TLS = &tls.ConnectionState{}
	return r
}

func TestEmailVerificationPageHasStrictCSPAndScrubsFragmentBeforeInteraction(t *testing.T) {
	h, err := NewEmailVerificationPage("https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, testVerificationPageRequest("/verify-email"))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
		w.Header().Get("Cache-Control") != "no-store" ||
		w.Header().Get("Referrer-Policy") != "no-referrer" ||
		w.Header().Get("X-Frame-Options") != "DENY" ||
		w.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" ||
		w.Header().Get("Set-Cookie") != "" {
		t.Fatal("browser page missing mandatory private response headers")
	}

	script, err := verificationPageAssets.ReadFile("email_verification_page.js")
	if err != nil {
		t.Fatal(err)
	}
	css, err := verificationPageAssets.ReadFile("email_verification_page.css")
	if err != nil {
		t.Fatal(err)
	}
	scriptDigest := sha256.Sum256(script)
	cssDigest := sha256.Sum256(css)
	csp := w.Header().Get("Content-Security-Policy")
	for _, expected := range []string{
		"default-src 'none'",
		"script-src 'sha256-" + base64.StdEncoding.EncodeToString(scriptDigest[:]) + "'",
		"style-src 'sha256-" + base64.StdEncoding.EncodeToString(cssDigest[:]) + "'",
		"connect-src 'self'",
		"frame-ancestors 'none'",
		"form-action 'none'",
	} {
		if !strings.Contains(csp, expected) {
			t.Fatalf("page CSP missing immutable directive %q", expected)
		}
	}
	for _, bad := range []string{"unsafe-inline", "unsafe-eval", "http:", "https:", "data:", "blob:"} {
		if strings.Contains(csp, bad) {
			t.Fatalf("page CSP permits unapproved content type %q", bad)
		}
	}

	html := w.Body.String()
	start := strings.Index(html, "<script>")
	end := strings.Index(html, "</script>")
	style := strings.Index(html, "<style>")
	body := strings.Index(html, "<body>")
	if start < 0 || end <= start || style <= end || body <= style ||
		html[start+len("<script>"):end] != string(script) ||
		!strings.Contains(html, "<button id=\"confirm\" type=\"button\" hidden>") ||
		!strings.Contains(html, "aria-live=\"polite\"") {
		t.Fatal("first-party inline resources or accessible confirmation UI changed")
	}
	for _, forbidden := range []string{"<script src=", "<link ", "<iframe ", "<img ", "<form ", "mailto:", "http://", "https://"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("page accidentally loaded an external resource or auto-submitting form: %s", forbidden)
		}
	}
	js := string(script)
	scrub := strings.Index(js, "window.history.replaceState")
	dom := strings.Index(js, "DOMContentLoaded")
	click := strings.Index(js, "addEventListener(\"click\"")
	post := strings.Index(js, "fetch(\"/v1/identity/verify-email\"")
	if scrub < 0 || dom <= scrub || click <= dom || post <= click ||
		!strings.Contains(js, "credentials: \"omit\"") ||
		!strings.Contains(js, "redirect: \"error\"") ||
		!strings.Contains(js, "referrerPolicy: \"no-referrer\"") {
		t.Fatal("fragment must be scrubbed before manual click and same-origin POST")
	}
}

func TestEmailVerificationPageRejectsUnsafeNavigationWithoutRenderingSecrets(t *testing.T) {
	h, err := NewEmailVerificationPage("https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		path   string
		mutate func(*http.Request)
		status int
	}{
		{"wrong path", "/v1/identity/verify-email", nil, 404},
		{"wrong method", "/verify-email", func(r *http.Request) { r.Method = http.MethodPost }, 405},
		{"query credential", "/verify-email?token=secret", nil, 403},
		{"empty query marker", "/verify-email?", nil, 403},
		{"URL fragment in server request", "/verify-email#token=secret", nil, 403},
		{"plain HTTP", "/verify-email", func(r *http.Request) { r.TLS = nil }, 403},
		{"spoofed Host", "/verify-email", func(r *http.Request) { r.Host = "evil.test" }, 403},
		{"cookies", "/verify-email", func(r *http.Request) { r.Header.Set("Cookie", "sid=synthetic") }, 403},
		{"authorization", "/verify-email", func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }, 403},
		{"forwarded proto", "/verify-email", func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") }, 403},
		{"forwarded chain", "/verify-email", func(r *http.Request) { r.Header.Set("Forwarded", "for=198.51.100.1") }, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := testVerificationPageRequest(tc.path)
			if tc.mutate != nil {
				tc.mutate(req)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.status || w.Body.Len() != 0 ||
				w.Header().Get("Cache-Control") != "no-store" ||
				strings.Contains(w.Header().Get("Content-Security-Policy"), "'sha256-") {
				t.Fatalf("unsafe navigation rendered executable content: %d, expected %d", w.Code, tc.status)
			}
		})
	}
	if _, err := NewEmailVerificationPage(""); err == nil {
		t.Fatal("empty verification page origin accepted")
	}
	for _, badOrigin := range []string{
		"http://app.example.test",
		"https://app.example.test/path",
		"https://app.example.test/",
		"https://app.example.test?token=secret",
		"https://attacker@app.example.test",
		"https://app.example.test.",
		"https://app.example.test#fragment",
	} {
		if _, err := NewEmailVerificationPage(badOrigin); err == nil {
			t.Fatalf("unsafe trusted origin accepted: %q", badOrigin)
		}
	}
	var absent *EmailVerificationPage
	w := httptest.NewRecorder()
	absent.ServeHTTP(w, testVerificationPageRequest("/verify-email"))
	if w.Code != http.StatusServiceUnavailable || w.Body.Len() != 0 {
		t.Fatal("unconfigured browser page must fail closed")
	}
}
