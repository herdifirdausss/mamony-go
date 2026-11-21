package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

type testGate struct {
	called bool
	allowed bool
	errorValue error
	peer netip.Addr
}

func (f *testGate) Admit(_ context.Context, _ usecase.AnonymousAction, _ string, p netip.Addr) (bool,error) {
	f.called = true
	f.peer = p
	return f.allowed, f.errorValue
}

func TestClientIPResolverTrustsOnlyDirectPeerByDefault(t *testing.T) {
	resolver, err := NewClientIPResolver(nil)
	if err != nil { t.Fatal(err) }
	req := httptest.NewRequest(http.MethodPost, "https://example.test/", nil)
	req.RemoteAddr = "198.51.100.1:40123"
	req.Header.Set("X-Forwarded-For", "1.1.1.1")
	req.Header.Set("Forwarded", "for=192.0.2.2")
	req.Header.Set("X-Real-IP", "8.8.8.8")
	got, err := resolver.Resolve(req)
	if err != nil || got.String() != "198.51.100.1" {
		t.Fatalf("spoofed forwarding header influenced direct TCP peer: %s %v", got, err)
	}
	for _, bad := range []string{"spoofed.example.test:3000", "198.51.100.1", "[::1%eth0]:443", "0.0.0.0:10"} {
		req.RemoteAddr = bad
		if _, err := resolver.Resolve(req); !errors.Is(err, ErrClientPeerUntrusted) {
			t.Fatalf("invalid peer accepted: %q", bad)
		}
	}
}

func TestClientIPResolverRejectsAmbiguousProxyHeaders(t *testing.T) {
	for _, bad := range []string{"0.0.0.0/0","10.0.0.0/8","::/0","2001:db8::/64"} {
		if _, err := NewClientIPResolver([]netip.Prefix{netip.MustParsePrefix(bad)}); err == nil {
			t.Fatalf("unsafe trusted proxy scope accepted: %s", bad)
		}
	}
	resolver, err := NewClientIPResolver([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	if err != nil { t.Fatal(err) }
	newRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "https://example.test/", nil)
		req.RemoteAddr = "127.0.0.1:44888"
		req.Header.Set("X-Forwarded-For", "198.51.100.10")
		return req
	}
	req := newRequest()
	got, err := resolver.Resolve(req)
	if err != nil || got.String() != "198.51.100.10" {
		t.Fatalf("sanitized exact proxy IP not honored: %s %v", got, err)
	}
	for _, mutate := range []func(*http.Request){
		func(req *http.Request){ req.Header.Del("X-Forwarded-For") },
		func(req *http.Request){ req.Header.Set("X-Forwarded-For","198.51.100.10, 1.1.1.1") },
		func(req *http.Request){ req.Header.Add("X-Forwarded-For","8.8.8.8") },
		func(req *http.Request){ req.Header.Set("X-Forwarded-For", "spoofed.example.test") },
		func(req *http.Request){ req.Header.Set("Forwarded", "for=1.1.1.1") },
		func(req *http.Request){ req.Header.Set("X-Real-IP", "1.1.1.1") },
		func(req *http.Request){ req.Header.Set("True-Client-IP", "1.1.1.1") },
	} {
		badReq := newRequest()
		mutate(badReq)
		if _, err := resolver.Resolve(badReq); !errors.Is(err, ErrClientPeerUntrusted) {
			t.Fatalf("ambiguous forwarded identity accepted: %v", badReq.Header)
		}
	}
}

func TestAnonymousBoundaryReceiptDoesNotEnumerateAccounts(t *testing.T) {
	resolver, _ := NewClientIPResolver(nil)
	f := &testGate{allowed: true}
	b, err := NewAnonymousBoundary(resolver, f)
	if err != nil { t.Fatal(err) }
	req := httptest.NewRequest(http.MethodPost, "https://example.test/", nil)
	req.RemoteAddr = "198.51.100.45:8080"
	var baseline string
	for _, tc := range []struct{name, email string; allowed bool}{
		{"known account", "known@example.test", true},
		{"unknown account", "missing@example.test", true},
		{"exhausted anonymous budget", "known@example.test", false},
		{"exhausted nonexistent account", "missing@example.test", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.allowed = tc.allowed
			f.called = false
			_, err := b.Admit(req, usecase.AnonymousResend, tc.email)
			if err != nil || !f.called || f.peer.String() != "198.51.100.45" {
				t.Fatalf("request not checked against peer budget: %v", err)
			}
			w := httptest.NewRecorder()
			WriteOpaqueAdmissionReceipt(w)
			if w.Code != 202 || w.Header().Get("Cache-Control") != "no-store" ||
				w.Header().Get("Content-Type") != "application/json" {
				t.Fatal("opaque receipt response contract changed")
			}
			got := w.Result().Status + "\n" + w.Body.String()
			if baseline == "" { baseline = got } else if got != baseline {
				t.Fatalf("known/unknown/limited accounts have distinguishable body/status: %q vs %q", got, baseline)
			}
		})
	}
	f.errorValue = errors.New("sensitive SQL failure")
	if _, err := b.Admit(req, usecase.AnonymousResend, "known@example.test"); !errors.Is(err, usecase.ErrAnonymousAdmissionUnavailable) {
		t.Fatalf("persistence failure escaped unsanitized: %v", err)
	}
	if _, err := NewAnonymousBoundary(nil, f); !errors.Is(err, ErrClientPeerUntrusted) {
		t.Fatal("missing peer resolver accepted")
	}
}
