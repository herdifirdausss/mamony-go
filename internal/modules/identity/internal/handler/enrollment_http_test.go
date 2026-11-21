package handler

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

type enrollmentWorkFake struct {
	registered int
	resent     int
	err        error
}

func (f *enrollmentWorkFake) Register(_ context.Context, _, _ string) error {
	f.registered++
	return f.err
}

func (f *enrollmentWorkFake) Resend(_ context.Context, _ string) error {
	f.resent++
	return f.err
}

func enrollmentRequest(path, data string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "https://app.example.test"+path, strings.NewReader(data))
	req.TLS = &tls.ConnectionState{}
	req.RemoteAddr = "198.51.100.42:33000"
	req.Header.Set("Origin", "https://app.example.test")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	return req
}

func enrollmentFixture(t *testing.T) (*EnrollmentHTTP, *testGate, *enrollmentWorkFake) {
	t.Helper()
	resolver, err := NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	gate := &testGate{allowed: true}
	boundary, err := NewAnonymousBoundary(resolver, gate)
	if err != nil {
		t.Fatal(err)
	}
	workflow := &enrollmentWorkFake{}
	h, err := NewEnrollmentHTTP(boundary, workflow, "https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}
	return h, gate, workflow
}

func TestIdentityEnrollmentHTTPReceiptIsOpaque(t *testing.T) {
	h, gate, work := enrollmentFixture(t)
	var receipt string
	for _, tc := range []struct {
		path    string
		body    string
		allowed bool
		err     error
	}{
		{registerPath, `{"email":"new@example.test","password":"a long synthetic password"}`, true, nil},
		{registerPath, `{"email":"existing@example.test","password":"a long synthetic password"}`, true, errors.New("secret database failure")},
		{registerPath, `{"email":"existing@example.test","password":"a long synthetic password"}`, false, nil},
		{resendPath, `{"email":"known@example.test"}`, true, nil},
		{resendPath, `{"email":"missing@example.test"}`, true, nil},
		{resendPath, `{"email":"known@example.test"}`, false, nil},
		{resendPath, `{"email":"missing@example.test"}`, true, errors.New("secret lookup failure")},
	} {
		gate.allowed = tc.allowed
		work.err = tc.err
		beforeRegister, beforeResend := work.registered, work.resent
		w := httptest.NewRecorder()
		h.ServeHTTP(w, enrollmentRequest(tc.path, tc.body))
		if w.Code != 202 || w.Header().Get("Cache-Control") != "no-store" ||
			w.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("valid outcomes must return opaque 202: %d %s", w.Code, w.Body.String())
		}
		if receipt == "" {
			receipt = w.Body.String()
		}
		if w.Body.String() != receipt {
			t.Fatal("known/unknown/throttled outcomes have distinguishable body")
		}
		if !tc.allowed && (work.registered != beforeRegister || work.resent != beforeResend) {
			t.Fatal("quota-rejected request reached Identity workflow")
		}
	}
	work.err = domain.ErrInvalidPassword
	w := httptest.NewRecorder()
	h.ServeHTTP(w, enrollmentRequest(registerPath, `{"email":"valid@example.test","password":"bad"}`))
	if w.Code != 400 || strings.Contains(w.Body.String(), "password") {
		t.Fatal("invalid password must have generic validation error")
	}
}

func TestIdentityEnrollmentHTTPRejectsUnsafeRequests(t *testing.T) {
	h, gate, work := enrollmentFixture(t)
	for _, tc := range []struct {
		name   string
		path   string
		body   string
		mutate func(*http.Request)
		want   int
	}{
		{"duplicate email", registerPath, `{"email":"a@example.test","email":"b@example.test","password":"pass"}`, nil, 400},
		{"duplicate password", registerPath, `{"email":"a@example.test","password":"first","password":"second"}`, nil, 400},
		{"unknown field", registerPath, `{"email":"a@example.test","password":"pass","role":"ADMIN"}`, nil, 400},
		{"subject ID smuggling", resendPath, `{"email":"a@example.test","subjectID":"sub_AAAAAAAAAAAAAAAAAAAAAA"}`, nil, 400},
		{"missing email", resendPath, `{}`, nil, 400},
		{"missing password", registerPath, `{"email":"a@example.test"}`, nil, 400},
		{"object password", registerPath, `{"email":"a@example.test","password":{"value":"secret"}}`, nil, 400},
		{"extra JSON", resendPath, `{"email":"a@example.test"}{"email":"b@example.test"}`, nil, 400},
		{"invalid email", resendPath, `{"email":"not an email"}`, nil, 400},
		{"oversized", registerPath, `{"email":"a@example.test","password":"` + strings.Repeat("a", 3000) + `"}`, nil, 413},
		{"missing Origin", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.Header.Del("Origin") }, 403},
		{"cross-site Origin", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.Header.Set("Origin", "https://evil.test") }, 403},
		{"duplicate Origin", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.Header.Add("Origin", "https://app.example.test") }, 403},
		{"host spoof", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.Host = "evil.test" }, 403},
		{"plain HTTP", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.TLS = nil }, 403},
		{"forwarded scheme bypass", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) {
			r.TLS = nil
			r.Header.Set("X-Forwarded-Proto", "https")
		}, 403},
		{"cookie", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.Header.Set("Cookie", "sid=synthetic") }, 403},
		{"authorization", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }, 403},
		{"cross-site fetch", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, 403},
		{"query string", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.URL.RawQuery = "subjectID=123" }, 403},
		{"wrong content type", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415},
		{"duplicate content type", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.Header.Add("Content-Type", "application/json") }, 415},
		{"invalid charset", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.Header.Set("Content-Type", "application/json; charset=latin1") }, 415},
		{"missing client peer", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.RemoteAddr = "spoofed.test:1234" }, 503},
		{"method other than POST", resendPath, `{"email":"a@example.test"}`, func(r *http.Request) { r.Method = http.MethodGet }, 405},
		{"unknown route", "/v1/identity/login", `{"email":"a@example.test"}`, nil, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := enrollmentRequest(tc.path, tc.body)
			if tc.mutate != nil {
				tc.mutate(req)
			}
			beforeReg, beforeResend := work.registered, work.resent
			gate.called = false
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.want || work.registered != beforeReg || work.resent != beforeResend ||
				strings.Contains(strings.ToLower(w.Body.String()), "secret") {
				t.Fatalf("untrusted request escaped: status=%d want=%d body=%q", w.Code, tc.want, w.Body.String())
			}
			if gate.called {
				t.Fatal("rejected request unexpectedly reached anonymous admission")
			}
		})
	}
	gate.errorValue = errors.New("secret database connection error")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, enrollmentRequest(resendPath, `{"email":"a@example.test"}`))
	if w.Code != 503 || strings.Contains(w.Body.String(), "connection") {
		t.Fatal("admission outage must deny generically")
	}
	for _, bad := range []string{"", "http://app.example.test", "https://app.example.test/path", "https://app.example.test?foo=1", "https://app.example.test/"} {
		if _, err := NewEnrollmentHTTP(h.admission, work, bad); !errors.Is(err, ErrEnrollmentHandlerConfig) {
			t.Fatalf("unsafe origin accepted %q", bad)
		}
	}
}
