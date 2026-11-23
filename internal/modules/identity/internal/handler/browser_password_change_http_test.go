package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

const syntheticCurrentPhrase = "an old synthetic very strong password"
const syntheticReplacementPhrase = "a fresh synthetic very strong password"

type browserChangeHasher struct{ calls int }

func (h *browserChangeHasher) Hash(_ context.Context, _ string) (domain.PasswordVerifier, error) {
	h.calls++
	return domain.PasswordVerifier{
		Version: 1, MemoryKiB: 65536, Iterations: 3, Parallelism: 1,
		Salt: bytes.Repeat([]byte{0x74}, 16), Digest: bytes.Repeat([]byte{0x84}, 32),
	}, nil
}

type browserChangeStore struct {
	calls    int
	allowed  bool
	err      error
	sessions *browserSessionStore
}

func (s *browserChangeStore) Rotate(_ context.Context, _, _ []byte, _ domain.PasswordVerifier, _ string) (bool, error) {
	s.calls++
	if s.allowed {
		// The fake models the epoch-triggered invalidation on a commit.
		clear(s.sessions.subjects)
	}
	return s.allowed, s.err
}

func browserChangeFixture(t *testing.T) (*BrowserPasswordChangeHTTP, *BrowserSessionHTTP, *browserLoginGate, *browserPasswordPort, *browserChangeHasher, *browserChangeStore, *http.Cookie) {
	t.Helper()
	browser, gate, password, sessions := browserFixture(t)
	cookie := browserLogin(t, browser)
	flow, err := usecase.NewPasswordLoginFlow(gate, password)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := usecase.NewAuthenticationGrantAuthority(sessions)
	if err != nil {
		t.Fatal(err)
	}
	hasher := &browserChangeHasher{}
	store := &browserChangeStore{allowed: true, sessions: sessions}
	change, err := usecase.NewAuthenticatedPasswordChange(
		browser.session, flow, grant, hasher, store,
		domain.PasswordPolicy{IsBlocked: func(value, _ string) bool { return value == "blocked synthetic replacement password" }},
	)
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewBrowserPasswordChangeHTTP(browser, change)
	if err != nil {
		t.Fatal(err)
	}
	return h, browser, gate, password, hasher, store, cookie
}

func browserChangeBody() string {
	return `{"email":"alice@example.test","currentPassword":"` + syntheticCurrentPhrase + `","nextPassword":"` + syntheticReplacementPhrase + `"}`
}

func browserChangeRequest(cookie *http.Cookie, csrf, body string) *http.Request {
	r := browserRequest(http.MethodPost, browserRotatePath, body)
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.Header.Set("Cookie", cookie.String())
	}
	if csrf != "" {
		r.Header.Set("X-Mamony-CSRF", csrf)
	}
	return r
}

func TestBrowserPasswordChangeCandidateSuccessInvalidatesOldCookie(t *testing.T) {
	h, browser, gate, password, hasher, store, cookie := browserChangeFixture(t)
	csrf := browser.csrf.issue(cookie.Value)
	r := browserChangeRequest(cookie, csrf, browserChangeBody())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 || w.Body.Len() != 0 || store.calls != 1 ||
		hasher.calls != 1 || password.calls != 2 || gate.calls != 2 {
		t.Fatalf("change bypassed reauthentication or rotation: status=%d gate=%d password=%d hash=%d store=%d",
			w.Code, gate.calls, password.calls, hasher.calls, store.calls)
	}
	deleted := w.Result().Cookies()
	if len(deleted) != 1 || deleted[0].Name != browserCookieName ||
		deleted[0].MaxAge != -1 || !deleted[0].Secure || !deleted[0].HttpOnly ||
		deleted[0].Domain != "" || deleted[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("certain password change must clear the existing host-only browser cookie")
	}
	after := browserRequest(http.MethodGet, browserSessionPath, "")
	after.Header.Set("Cookie", cookie.String())
	out := httptest.NewRecorder()
	browser.ServeHTTP(out, after)
	if out.Code != 401 {
		t.Fatal("stale browser bearer survived password change")
	}
}

func TestBrowserPasswordChangeCandidateRejectsUnsafeInputsBeforeKDF(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		edit func(*http.Request)
		want int
	}{
		{"wrong route", browserChangeBody(), func(r *http.Request) { r.URL.Path = "/v1/identity/other" }, 404},
		{"wrong method", browserChangeBody(), func(r *http.Request) { r.Method = "GET" }, 405},
		{"missing origin", browserChangeBody(), func(r *http.Request) { r.Header.Del("Origin") }, 403},
		{"other origin", browserChangeBody(), func(r *http.Request) { r.Header.Set("Origin", "https://hostile.test") }, 403},
		{"other host", browserChangeBody(), func(r *http.Request) { r.Host = "hostile.test" }, 403},
		{"no TLS", browserChangeBody(), func(r *http.Request) { r.TLS = nil }, 403},
		{"cross site", browserChangeBody(), func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, 403},
		{"spoof proxy", browserChangeBody(), func(r *http.Request) { r.Header.Set("X-Forwarded-For", "203.0.113.30") }, 403},
		{"auth header", browserChangeBody(), func(r *http.Request) { r.Header.Set("Authorization", "Bearer example") }, 403},
		{"URL secret", browserChangeBody(), func(r *http.Request) { r.URL.RawQuery = "token=secret" }, 403},
		{"no cookie", browserChangeBody(), func(r *http.Request) { r.Header.Del("Cookie") }, 401},
		{"duplicate cookie", browserChangeBody(), func(r *http.Request) { r.Header.Add("Cookie", r.Header.Get("Cookie")) }, 401},
		{"no CSRF", browserChangeBody(), func(r *http.Request) { r.Header.Del("X-Mamony-CSRF") }, 403},
		{"wrong CSRF", browserChangeBody(), func(r *http.Request) { r.Header.Set("X-Mamony-CSRF", "forged") }, 403},
		{"duplicate CSRF", browserChangeBody(), func(r *http.Request) { r.Header.Add("X-Mamony-CSRF", r.Header.Get("X-Mamony-CSRF")) }, 403},
		{"non JSON", browserChangeBody(), func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415},
		{"duplicate type", browserChangeBody(), func(r *http.Request) { r.Header.Add("Content-Type", "application/json") }, 415},
		{"missing email", `{"currentPassword":"old","nextPassword":"next"}`, nil, 400},
		{"invalid email", `{"email":"broken","currentPassword":"old","nextPassword":"next"}`, nil, 400},
		{"duplicate email", `{"email":"alice@example.test","email":"bob@example.test","currentPassword":"old","nextPassword":"next"}`, nil, 400},
		{"duplicate current", `{"email":"alice@example.test","currentPassword":"old","currentPassword":"other","nextPassword":"next"}`, nil, 400},
		{"duplicate next", `{"email":"alice@example.test","currentPassword":"old","nextPassword":"next","nextPassword":"other"}`, nil, 400},
		{"unknown privilege", `{"email":"alice@example.test","currentPassword":"old","nextPassword":"next","role":"admin"}`, nil, 400},
		{"nested password", `{"email":"alice@example.test","currentPassword":{"token":"old"},"nextPassword":"next"}`, nil, 400},
		{"trailing document", browserChangeBody() + browserChangeBody(), nil, 400},
		{"oversize", strings.Repeat("A", 5000), nil, 413},
		{"chunked oversized JSON", `{"email":"alice@example.test","currentPassword":"old","nextPassword":"` + strings.Repeat("A", 5000) + `"}`, func(r *http.Request) {
			r.ContentLength = -1
			r.TransferEncoding = []string{"chunked"}
		}, 413},
		{"untrusted peer", browserChangeBody(), func(r *http.Request) { r.RemoteAddr = "bad peer" }, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, browser, gate, password, hasher, store, cookie := browserChangeFixture(t)
			r := browserChangeRequest(cookie, browser.csrf.issue(cookie.Value), tc.body)
			if tc.edit != nil {
				tc.edit(r)
			}
			baseGate, basePassword := gate.calls, password.calls
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || gate.calls != baseGate || password.calls != basePassword ||
				hasher.calls != 0 || store.calls != 0 || w.Header().Get("Set-Cookie") != "" {
				t.Fatalf("unsafe input reached credential work: %d expected %d, body=%q",
					w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestBrowserPasswordChangeCandidateDenialAndAmbiguousCommit(t *testing.T) {
	t.Run("old password refused", func(t *testing.T) {
		h, browser, _, password, hasher, store, cookie := browserChangeFixture(t)
		password.err = usecase.ErrPasswordAuthenticationDenied
		w := httptest.NewRecorder()
		h.ServeHTTP(w, browserChangeRequest(cookie, browser.csrf.issue(cookie.Value), browserChangeBody()))
		if w.Code != 401 || hasher.calls != 0 || store.calls != 0 || w.Header().Get("Set-Cookie") != "" {
			t.Fatal("wrong old password reached rotation or replaced the cookie")
		}
	})
	t.Run("cross-account password proof refused", func(t *testing.T) {
		h, browser, _, password, hasher, store, cookie := browserChangeFixture(t)
		password.subject = "sub_BBBBBBBBBBBBBBBBBBBBBB"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, browserChangeRequest(cookie, browser.csrf.issue(cookie.Value), browserChangeBody()))
		if w.Code != 401 || hasher.calls != 0 || store.calls != 0 {
			t.Fatal("a different account's password changed the active session's owner")
		}
	})
	t.Run("credential rate budget denies", func(t *testing.T) {
		h, browser, gate, password, hasher, store, cookie := browserChangeFixture(t)
		gate.allowed = false
		calls := password.calls
		w := httptest.NewRecorder()
		h.ServeHTTP(w, browserChangeRequest(cookie, browser.csrf.issue(cookie.Value), browserChangeBody()))
		if w.Code != 401 || password.calls != calls || hasher.calls != 0 || store.calls != 0 {
			t.Fatal("denied pre-KDF budget reached verifier or mutation")
		}
	})
	t.Run("blocked replacement refused", func(t *testing.T) {
		h, browser, _, _, hasher, store, cookie := browserChangeFixture(t)
		body := strings.Replace(browserChangeBody(), syntheticReplacementPhrase, "blocked synthetic replacement password", 1)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, browserChangeRequest(cookie, browser.csrf.issue(cookie.Value), body))
		if w.Code != 401 || hasher.calls != 0 || store.calls != 0 {
			t.Fatal("blocked password entered new-password hash/rotation")
		}
	})
	t.Run("commit ambiguous returns unavailable without cookie clear", func(t *testing.T) {
		h, browser, _, _, _, store, cookie := browserChangeFixture(t)
		store.err = errors.New("private SQL connection lost after commit")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, browserChangeRequest(cookie, browser.csrf.issue(cookie.Value), browserChangeBody()))
		if w.Code != 503 || store.calls != 1 || w.Header().Get("Set-Cookie") != "" ||
			strings.Contains(w.Body.String(), "SQL") {
			t.Fatal("ambiguous password rotation declared success or leaked SQL details")
		}
	})
}

func TestBrowserPasswordChangeCandidateCSRFBoundToSession(t *testing.T) {
	h, browser, gate, _, hasher, store, first := browserChangeFixture(t)
	second := browserLogin(t, browser)
	r := browserChangeRequest(second, browser.csrf.issue(first.Value), browserChangeBody())
	before := gate.calls
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 || gate.calls != before || hasher.calls != 0 || store.calls != 0 {
		t.Fatal("a CSRF token from one browser session authorized another")
	}
}

func TestBrowserPasswordChangeCandidateConstructorAndPayload(t *testing.T) {
	browser, _, _, _ := browserFixture(t)
	if _, err := NewBrowserPasswordChangeHTTP(nil, nil); !errors.Is(err, ErrBrowserSessionHTTPConfig) {
		t.Fatal("nil browser and workflow accepted")
	}
	if _, err := NewBrowserPasswordChangeHTTP(browser, nil); !errors.Is(err, ErrBrowserSessionHTTPConfig) {
		t.Fatal("nil trusted password-change workflow accepted")
	}
	email, old, next, err := decodePasswordChangeJSON(strings.NewReader(browserChangeBody()))
	if err != nil || email != "alice@example.test" || old != syntheticCurrentPhrase || next != syntheticReplacementPhrase {
		t.Fatal("valid strict password change JSON did not decode")
	}
}
