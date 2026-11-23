package handler

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

const testBrowserSubject = "sub_AAAAAAAAAAAAAAAAAAAAAA"
const validBrowserJSON = "{\"email\":\"alice@example.test\",\"password\":\"strong synthetic password\"}"

type browserLoginGate struct {
	allowed bool
	err     error
	calls   int
	peer    netip.Addr
}

func (f *browserLoginGate) Admit(_ context.Context, _ string, peer netip.Addr) (bool, error) {
	f.calls++
	f.peer = peer
	return f.allowed, f.err
}

type browserPasswordPort struct {
	subject string
	err     error
	calls   int
}

func (f *browserPasswordPort) AuthenticateWithEpoch(_ context.Context, _, _ string) (usecase.AuthenticatedSubject, error) {
	f.calls++
	if f.err != nil || f.subject == "" {
		return usecase.AuthenticatedSubject{}, f.err
	}
	return usecase.AuthenticatedSubject{SubjectID: f.subject, SessionEpoch: 1}, nil
}

type browserSessionStore struct {
	allowed    bool
	issueErr   error
	resolveErr error
	revokeErr  error
	issues     int
	resolves   int
	revokes    int
	subjects   map[string]string
	grants     map[string]string
}

func (f *browserSessionStore) Stage(_ context.Context, subject string, expectedEpoch int64, digest []byte) (bool, error) {
	if expectedEpoch <= 0 {
		return false, nil
	}
	if f.grants == nil {
		f.grants = make(map[string]string)
	}
	f.grants[string(digest)] = subject
	return true, nil
}

func (f *browserSessionStore) Issue(_ context.Context, grantDigest, digest []byte, _ string) (bool, error) {
	f.issues++
	if f.issueErr != nil || !f.allowed {
		return f.allowed, f.issueErr
	}
	subject := f.grants[string(grantDigest)]
	if subject == "" {
		return false, nil
	}
	delete(f.grants, string(grantDigest))
	if f.subjects == nil {
		f.subjects = make(map[string]string)
	}
	f.subjects[string(digest)] = subject
	return true, nil
}

func (f *browserSessionStore) Resolve(_ context.Context, digest []byte) (string, error) {
	f.resolves++
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	return f.subjects[string(digest)], nil
}

func (f *browserSessionStore) Revoke(_ context.Context, digest []byte, _ string) error {
	f.revokes++
	if f.revokeErr == nil {
		delete(f.subjects, string(digest))
	}
	return f.revokeErr
}

func browserFixture(t *testing.T) (*BrowserSessionHTTP, *browserLoginGate, *browserPasswordPort, *browserSessionStore) {
	t.Helper()
	peer, err := NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	gate := &browserLoginGate{allowed: true}
	password := &browserPasswordPort{subject: testBrowserSubject}
	store := &browserSessionStore{allowed: true}
	passwordFlow, err := usecase.NewPasswordLoginFlow(gate, password)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := usecase.NewOpaqueSessionAuthority(store)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := usecase.NewAuthenticationGrantAuthority(store)
	if err != nil {
		t.Fatal(err)
	}
	login, err := usecase.NewSessionLogin(passwordFlow, grant, authority)
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewBrowserSessionHTTP(peer, login, authority, bytes.Repeat([]byte{0xd3}, 32), "https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}
	return h, gate, password, store
}

func browserRequest(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, "https://app.example.test"+path, strings.NewReader(body))
	r.TLS = &tls.ConnectionState{}
	r.Host = "app.example.test"
	r.RemoteAddr = "198.51.100.48:33333"
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if method == http.MethodPost {
		r.Header.Set("Origin", "https://app.example.test")
	}
	return r
}

func browserLogin(t *testing.T, h *BrowserSessionHTTP) *http.Cookie {
	t.Helper()
	req := browserRequest(http.MethodPost, browserLoginPath, validBrowserJSON)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatalf("synthetic login failed: %d %q", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("login must send exactly one cookie")
	}
	return cookies[0]
}

func TestBrowserSessionHTTPLoginCheckCSRFLogout(t *testing.T) {
	h, gate, password, store := browserFixture(t)
	cookie := browserLogin(t, h)
	if gate.calls != 1 || password.calls != 1 || store.issues != 1 ||
		gate.peer.String() != "198.51.100.48" {
		t.Fatal("login bypassed pre-KDF admission, credentials or trusted peer")
	}
	if cookie.Name != browserCookieName || cookie.Path != "/" || cookie.Domain != "" ||
		!cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode ||
		cookie.MaxAge != 43200 || !canonicalSessionBearer(cookie.Value) {
		t.Fatal("cookie must be host-only, secure, HTTP-only, Strict and 12h")
	}

	session := browserRequest(http.MethodGet, browserSessionPath, "")
	session.Header.Set("Cookie", cookie.String())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, session)
	if w.Code != 200 || w.Body.String() != "{\"authenticated\":true}\n" ||
		w.Header().Get("Set-Cookie") != "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("session GET leaked subject, refreshed cookie or cached result")
	}
	csrf := browserRequest(http.MethodGet, browserCSRFPath, "")
	csrf.Header.Set("Cookie", cookie.String())
	w = httptest.NewRecorder()
	h.ServeHTTP(w, csrf)
	var result map[string]string
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil ||
		len(result["csrfToken"]) == 0 || strings.Contains(w.Body.String(), cookie.Value) {
		t.Fatal("session-bound CSRF token unavailable or bearer exposed")
	}
	bad := browserRequest(http.MethodPost, browserLogoutPath, "")
	bad.Header.Set("Cookie", cookie.String())
	w = httptest.NewRecorder()
	h.ServeHTTP(w, bad)
	if w.Code != 403 || store.revokes != 0 {
		t.Fatal("missing CSRF reached logout")
	}
	logout := browserRequest(http.MethodPost, browserLogoutPath, "")
	logout.Header.Set("Cookie", cookie.String())
	logout.Header.Set("X-Mamony-CSRF", result["csrfToken"])
	w = httptest.NewRecorder()
	h.ServeHTTP(w, logout)
	if w.Code != 204 || store.revokes != 1 || w.Body.Len() != 0 {
		t.Fatal("valid logout failed to revoke")
	}
	deleted := w.Result().Cookies()
	if len(deleted) != 1 || deleted[0].Name != browserCookieName ||
		deleted[0].MaxAge != -1 || deleted[0].Domain != "" || deleted[0].Path != "/" ||
		!deleted[0].Secure || !deleted[0].HttpOnly ||
		deleted[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("logout cookie deletion weakened security attributes")
	}
	session = browserRequest(http.MethodGet, browserSessionPath, "")
	session.Header.Set("Cookie", cookie.String())
	w = httptest.NewRecorder()
	h.ServeHTTP(w, session)
	if w.Code != 401 {
		t.Fatal("revoked session still authenticated")
	}
}

func TestBrowserSessionHTTPDeniedUnknownOutageAndThrottled(t *testing.T) {
	h, gate, password, store := browserFixture(t)
	send := func() *httptest.ResponseRecorder {
		r := browserRequest(http.MethodPost, browserLoginPath, validBrowserJSON)
		r.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		h.ServeHTTP(out, r)
		return out
	}
	password.err = usecase.ErrPasswordAuthenticationDenied
	denial := send()
	if denial.Code != 401 || denial.Header().Get("Set-Cookie") != "" {
		t.Fatal("wrong password minted session")
	}
	password.err = errors.New("private SQL DSN")
	if w := send(); w.Code != 503 || strings.Contains(w.Body.String(), "SQL") || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("password outage leaked or minted session")
	}
	password.err = usecase.ErrPasswordAuthenticationDenied
	if w := send(); w.Code != 401 || w.Body.String() != denial.Body.String() {
		t.Fatal("invalid credentials yielded differing body")
	}
	password.err = nil
	gate.allowed = false
	count := password.calls
	if w := send(); w.Code != 429 || password.calls != count {
		t.Fatal("admission-denied request reached KDF")
	}
	gate.allowed = true
	gate.err = errors.New("private DB secret")
	if w := send(); w.Code != 503 || password.calls != count || strings.Contains(w.Body.String(), "secret") {
		t.Fatal("admission DB fault failed open")
	}
	gate.err = nil
	store.issueErr = errors.New("ambiguous session commit and private DSN")
	if w := send(); w.Code != 503 || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("ambiguous issue returned bearer")
	}
	store.issueErr = nil
	cookie := browserLogin(t, h)
	store.resolveErr = errors.New("private session DB")
	r := browserRequest(http.MethodGet, browserSessionPath, "")
	r.Header.Set("Cookie", cookie.String())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 503 || strings.Contains(w.Body.String(), "DB") {
		t.Fatal("session DB unavailability misreported as invalid")
	}
	store.resolveErr = nil
	store.revokeErr = errors.New("ambiguous revoke")
	r = browserRequest(http.MethodPost, browserLogoutPath, "")
	r.Header.Set("Cookie", cookie.String())
	r.Header.Set("X-Mamony-CSRF", h.csrf.issue(cookie.Value))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 503 || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("uncertain revoke must not claim logout success")
	}
}

func TestBrowserSessionHTTPRejectsUnsafeInputsBeforeAnyIdentityWork(t *testing.T) {
	h, gate, password, store := browserFixture(t)
	for _, tc := range []struct {
		name   string
		body   string
		change func(*http.Request)
		want   int
	}{
		{"duplicate email", "{\"email\":\"a@example.test\",\"email\":\"b@example.test\",\"password\":\"pw\"}", nil, 400},
		{"duplicate password", "{\"email\":\"a@example.test\",\"password\":\"one\",\"password\":\"two\"}", nil, 400},
		{"unknown role", "{\"email\":\"a@example.test\",\"password\":\"pw\",\"role\":\"admin\"}", nil, 400},
		{"invalid email", "{\"email\":\"broken\",\"password\":\"pw\"}", nil, 400},
		{"missing password", "{\"email\":\"a@example.test\"}", nil, 400},
		{"nested password", "{\"email\":\"a@example.test\",\"password\":{\"value\":\"pw\"}}", nil, 400},
		{"multiple JSON", validBrowserJSON + validBrowserJSON, nil, 400},
		{"oversized", "{\"email\":\"a@example.test\",\"password\":\"" + strings.Repeat("x", 3000) + "\"}", nil, 413},
		{"missing origin", validBrowserJSON, func(r *http.Request) { r.Header.Del("Origin") }, 403},
		{"cross-site origin", validBrowserJSON, func(r *http.Request) { r.Header.Set("Origin", "https://evil.test") }, 403},
		{"duplicate origin", validBrowserJSON, func(r *http.Request) { r.Header.Add("Origin", "https://app.example.test") }, 403},
		{"host mismatch", validBrowserJSON, func(r *http.Request) { r.Host = "evil.test" }, 403},
		{"plain HTTP", validBrowserJSON, func(r *http.Request) { r.TLS = nil }, 403},
		{"forwarded scheme", validBrowserJSON, func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") }, 403},
		{"forwarded identity", validBrowserJSON, func(r *http.Request) { r.Header.Set("X-Forwarded-For", "203.0.113.1") }, 403},
		{"cross-site metadata", validBrowserJSON, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, 403},
		{"cookie fixation", validBrowserJSON, func(r *http.Request) { r.Header.Set("Cookie", browserCookieName+"=fixation") }, 403},
		{"authorization smuggle", validBrowserJSON, func(r *http.Request) { r.Header.Set("Authorization", "Bearer bogus") }, 403},
		{"URL token", validBrowserJSON, func(r *http.Request) { r.URL.RawQuery = "token=secret" }, 403},
		{"invalid content type", validBrowserJSON, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415},
		{"duplicate content type", validBrowserJSON, func(r *http.Request) { r.Header.Add("Content-Type", "application/json") }, 415},
		{"peer invalid", validBrowserJSON, func(r *http.Request) { r.RemoteAddr = "invalid" }, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := browserRequest(http.MethodPost, browserLoginPath, tc.body)
			r.Header.Set("Content-Type", "application/json")
			if tc.change != nil {
				tc.change(r)
			}
			beforeGate, beforePassword, beforeIssue := gate.calls, password.calls, store.issues
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || gate.calls != beforeGate || password.calls != beforePassword ||
				store.issues != beforeIssue || w.Header().Get("Set-Cookie") != "" {
				t.Fatalf("invalid request reached KDF/issuer: got %d want %d body=%q", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestBrowserSessionHTTPRejectsDuplicateCookiesAndCrossSiteRead(t *testing.T) {
	h, _, _, store := browserFixture(t)
	cookie := browserLogin(t, h)
	csrf := h.csrf.issue(cookie.Value)
	for _, tc := range []struct {
		name   string
		cookie string
		token  string
		want   int
	}{
		{"duplicate cookie", cookie.String() + "; " + cookie.String(), csrf, 401},
		{"wrong cookie", browserCookieName + "=" + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xdd}, 32)), csrf, 403},
		{"tampered csrf", cookie.String(), strings.Repeat("A", len(csrf)), 403},
		{"missing csrf", cookie.String(), "", 403},
	} {
		req := browserRequest(http.MethodPost, browserLogoutPath, "")
		req.Header.Set("Cookie", tc.cookie)
		if tc.token != "" {
			req.Header.Set("X-Mamony-CSRF", tc.token)
		}
		before := store.revokes
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.want || store.revokes != before {
			t.Fatalf("%s: got %d want %d", tc.name, w.Code, tc.want)
		}
	}
	req := browserRequest(http.MethodGet, browserSessionPath, "")
	req.Header.Set("Cookie", cookie.String())
	req.Header.Set("Origin", "https://evil.test")
	before := store.resolves
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 || store.resolves != before {
		t.Fatal("cross-origin GET accepted")
	}
	req.Header.Del("Origin")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal("legitimate GET without Origin must work")
	}
}

func TestBrowserSessionHTTPConfigRequiresExactOriginAndSecret(t *testing.T) {
	h, _, _, _ := browserFixture(t)
	for _, origin := range []string{"", "http://app.example.test", "https://app.example.test/path",
		"https://app.example.test/", "https://evil.test@host.test"} {
		if _, err := NewBrowserSessionHTTP(h.peer, h.login, h.session,
			bytes.Repeat([]byte{0xd3}, 32), origin); !errors.Is(err, ErrBrowserSessionHTTPConfig) {
			t.Fatal("invalid origin accepted")
		}
	}
	for _, key := range [][]byte{nil, []byte("weak"), bytes.Repeat([]byte{0xd3}, 31)} {
		if _, err := NewBrowserSessionHTTP(h.peer, h.login, h.session, key, h.origin); !errors.Is(err, ErrBrowserSessionHTTPConfig) {
			t.Fatal("missing strong CSRF key accepted")
		}
	}
	if _, err := NewBrowserSessionHTTP(nil, h.login, h.session, bytes.Repeat([]byte{0xd3}, 32), h.origin); !errors.Is(err, ErrBrowserSessionHTTPConfig) {
		t.Fatal("nil peer accepted")
	}
	if _, err := NewBrowserSessionHTTP(h.peer, nil, h.session, bytes.Repeat([]byte{0xd3}, 32), h.origin); !errors.Is(err, ErrBrowserSessionHTTPConfig) {
		t.Fatal("nil login accepted")
	}
	if _, err := NewBrowserSessionHTTP(h.peer, h.login, nil, bytes.Repeat([]byte{0xd3}, 32), h.origin); !errors.Is(err, ErrBrowserSessionHTTPConfig) {
		t.Fatal("nil resolver accepted")
	}
}

func TestBrowserCSRFHMACBindsSessionOriginAndExpiry(t *testing.T) {
	first := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	second := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x12}, 32))
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	a, err := newSessionCSRF(bytes.Repeat([]byte{0xc1}, 32), "https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}
	a.clock = func() time.Time { return now }
	csrf := a.issue(first)
	if !a.verify(first, csrf) || a.verify(second, csrf) || a.verify(first, csrf+"x") ||
		a.verify(first, strings.Repeat("?", len(csrf))) || strings.Contains(csrf, first) {
		t.Fatal("CSRF did not bind secret/origin or reject altered tokens")
	}
	other, _ := newSessionCSRF(bytes.Repeat([]byte{0xc1}, 32), "https://other.example.test")
	other.clock = a.clock
	if other.verify(first, csrf) {
		t.Fatal("CSRF reused across origins")
	}
	other, _ = newSessionCSRF(bytes.Repeat([]byte{0xc2}, 32), "https://app.example.test")
	other.clock = a.clock
	if other.verify(first, csrf) {
		t.Fatal("CSRF reused across key rotation")
	}
	a.clock = func() time.Time { return now.Add(14*time.Minute + 59*time.Second) }
	if !a.verify(first, csrf) {
		t.Fatal("CSRF expired early")
	}
	a.clock = func() time.Time { return now.Add(15 * time.Minute) }
	if a.verify(first, csrf) {
		t.Fatal("expired CSRF remains accepted")
	}
	a.clock = func() time.Time { return now.Add(-time.Hour) }
	if a.verify(first, csrf) {
		t.Fatal("CSRF from future was accepted")
	}
}
