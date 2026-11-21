package handler

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

type confirmationGateFake struct {
	calls   int
	allowed bool
	err     error
}

func (g *confirmationGateFake) Admit(_ context.Context, _ netip.Addr) (bool, error) {
	g.calls++
	return g.allowed, g.err
}

type confirmationWorkFake struct {
	calls int
	seen  string
	err   error
}

func (w *confirmationWorkFake) Confirm(_ context.Context, token string) error {
	w.calls++
	w.seen = token
	return w.err
}

func confirmationToken() string {
	return base64.RawURLEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
}

func confirmationRequest(path, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "https://app.example.test"+path, strings.NewReader(body))
	req.TLS = &tls.ConnectionState{}
	req.Host = "app.example.test"
	req.RemoteAddr = "198.51.100.71:34567"
	req.Header.Set("Origin", "https://app.example.test")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Content-Type", "application/json")
	return req
}

func confirmationFixture(t *testing.T) (*EmailConfirmationHTTP, *confirmationGateFake, *confirmationWorkFake) {
	t.Helper()
	peer, err := NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	gate := &confirmationGateFake{allowed: true}
	work := &confirmationWorkFake{}
	h, err := NewEmailConfirmationHTTP(peer, gate, work, "https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}
	return h, gate, work
}

func TestEmailConfirmationHTTPAcceptedAndFailures(t *testing.T) {
	h, gate, work := confirmationFixture(t)
	body := `{"token":"` + confirmationToken() + `"}`
	w := httptest.NewRecorder()
	h.ServeHTTP(w, confirmationRequest(confirmEmailPath, body))
	if w.Code != 204 || w.Body.Len() != 0 ||
		w.Header().Get("Cache-Control") != "no-store" ||
		w.Header().Get("Referrer-Policy") != "no-referrer" ||
		work.calls != 1 || work.seen != confirmationToken() || gate.calls != 1 {
		t.Fatal("valid confirmation failed or produced a token-bearing response")
	}
	var genericFailure string
	for _, err := range []error{usecase.ErrEmailProofInvalid, fmt.Errorf("expired: %w", usecase.ErrEmailProofInvalid)} {
		work.err = err
		w = httptest.NewRecorder()
		h.ServeHTTP(w, confirmationRequest(confirmEmailPath, body))
		if w.Code != 400 || strings.Contains(w.Body.String(), "sensitive") ||
			strings.Contains(w.Body.String(), confirmationToken()) {
			t.Fatal("invalid/expired/replayed proof must be redacted")
		}
		if genericFailure != "" && w.Body.String() != genericFailure {
			t.Fatal("invalid/expired/replayed responses differ")
		}
		genericFailure = w.Body.String()
	}
	var genericOutage string
	for _, failure := range []error{
		usecase.ErrEmailProofUnavailable,
		errors.New("secret SQL DSN with password"),
	} {
		work.err = failure
		w = httptest.NewRecorder()
		h.ServeHTTP(w, confirmationRequest(confirmEmailPath, body))
		if w.Code != http.StatusServiceUnavailable ||
			strings.Contains(w.Body.String(), "secret") ||
			strings.Contains(w.Body.String(), confirmationToken()) ||
			w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("verifier fault must return a redacted non-cacheable 503")
		}
		if genericOutage != "" && genericOutage != w.Body.String() {
			t.Fatal("unexpected infrastructure faults had distinguishable HTTP bodies")
		}
		genericOutage = w.Body.String()
	}
	if genericFailure == genericOutage {
		t.Fatal("invalid proof and unavailable authority must not be conflated")
	}
	work.err = nil
	gate.allowed = false
	before := work.calls
	w = httptest.NewRecorder()
	h.ServeHTTP(w, confirmationRequest(confirmEmailPath, body))
	if w.Code != 429 || work.calls != before {
		t.Fatal("budget-denied request reached proof-consumption usecase")
	}
	gate.err = errors.New("secret rate-limiter failure")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, confirmationRequest(confirmEmailPath, body))
	if w.Code != 503 || work.calls != before ||
		strings.Contains(w.Body.String(), "secret") {
		t.Fatal("limiter outage did not fail closed with generic response")
	}
}

func TestEmailConfirmationHTTPRejectsUnsafeInputsBeforeAdmission(t *testing.T) {
	h, gate, work := confirmationFixture(t)
	valid := `{"token":"` + confirmationToken() + `"}`
	for _, tc := range []struct {
		name   string
		path   string
		body   string
		mutate func(*http.Request)
		want   int
	}{
		{"missing token", confirmEmailPath, `{}`, nil, 400},
		{"duplicate token", confirmEmailPath, `{"token":"one","token":"two"}`, nil, 400},
		{"subject smuggling", confirmEmailPath, `{"token":"` + confirmationToken() + `","subjectID":"other"}`, nil, 400},
		{"unknown field", confirmEmailPath, `{"token":"` + confirmationToken() + `","role":"ADMIN"}`, nil, 400},
		{"wrong type", confirmEmailPath, `{"token":{"secret":"hello"}}`, nil, 400},
		{"multiple documents", confirmEmailPath, valid + valid, nil, 400},
		{"noncanonical token", confirmEmailPath, `{"token":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}`, nil, 400},
		{"missing origin", confirmEmailPath, valid, func(r *http.Request) { r.Header.Del("Origin") }, 403},
		{"cross-site origin", confirmEmailPath, valid, func(r *http.Request) { r.Header.Set("Origin", "https://evil.test") }, 403},
		{"double origin", confirmEmailPath, valid, func(r *http.Request) { r.Header.Add("Origin", "https://app.example.test") }, 403},
		{"host spoofing", confirmEmailPath, valid, func(r *http.Request) { r.Host = "evil.test" }, 403},
		{"plain HTTP", confirmEmailPath, valid, func(r *http.Request) { r.TLS = nil }, 403},
		{"spoofed forwarded proto", confirmEmailPath, valid, func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") }, 403},
		{"cross-site fetch metadata", confirmEmailPath, valid, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, 403},
		{"cookie", confirmEmailPath, valid, func(r *http.Request) { r.Header.Set("Cookie", "sid=synthetic") }, 403},
		{"authorization header", confirmEmailPath, valid, func(r *http.Request) { r.Header.Set("Authorization", "Bearer x") }, 403},
		{"query token", confirmEmailPath, valid, func(r *http.Request) { r.URL.RawQuery = "token=secret" }, 403},
		{"url fragment", confirmEmailPath, valid, func(r *http.Request) { r.URL.Fragment = "token=secret" }, 403},
		{"wrong content type", confirmEmailPath, valid, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415},
		{"duplicate content type", confirmEmailPath, valid, func(r *http.Request) { r.Header.Add("Content-Type", "application/json") }, 415},
		{"wrong charset", confirmEmailPath, valid, func(r *http.Request) { r.Header.Set("Content-Type", "application/json; charset=latin1") }, 415},
		{"too large", confirmEmailPath, `{"token":"` + strings.Repeat("x", 350) + `"}`, nil, 413},
		{"unknown path", "/v1/identity/login", valid, nil, 404},
		{"wrong method", confirmEmailPath, valid, func(r *http.Request) { r.Method = http.MethodGet }, 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := confirmationRequest(tc.path, tc.body)
			if tc.mutate != nil {
				tc.mutate(req)
			}
			beforeAdmission, beforeWork := gate.calls, work.calls
			out := httptest.NewRecorder()
			h.ServeHTTP(out, req)
			if out.Code != tc.want || gate.calls != beforeAdmission ||
				work.calls != beforeWork ||
				strings.Contains(out.Body.String(), "secret") {
				t.Fatalf("unsafe request crossed boundary: got=%d want=%d body=%q", out.Code, tc.want, out.Body.String())
			}
		})
	}
	for _, bad := range []string{"", "http://app.example.test", "https://app.example.test/path", "https://app.example.test/", "https://app.example.test?token=x"} {
		peer, _ := NewClientIPResolver(nil)
		if _, err := NewEmailConfirmationHTTP(peer, gate, work, bad); !errors.Is(err, ErrConfirmationHandlerConfig) {
			t.Fatalf("unsafe trusted origin accepted: %q", bad)
		}
	}
	if _, err := NewEmailConfirmationHTTP(nil, gate, work, "https://app.example.test"); !errors.Is(err, ErrConfirmationHandlerConfig) {
		t.Fatal("missing trusted client resolver accepted")
	}
	if _, err := NewEmailConfirmationHTTP(h.peer, nil, work, "https://app.example.test"); !errors.Is(err, ErrConfirmationHandlerConfig) {
		t.Fatal("missing abuse gate accepted")
	}
	if _, err := NewEmailConfirmationHTTP(h.peer, gate, nil, "https://app.example.test"); !errors.Is(err, ErrConfirmationHandlerConfig) {
		t.Fatal("missing proof verifier accepted")
	}
}
