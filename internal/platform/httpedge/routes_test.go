package httpedge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListenerRouteIsolation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		h        http.Handler
		path     string
		code     int
		contains string
	}{
		{"public liveness", PublicHandler(), "/livez", 200, "alive"},
		{"private readiness", PrivateHandler(), "/readyz", 200, "http-ready"},
		{"private not public", PublicHandler(), "/readyz", 404, ""},
		{"no fictional financial route", PublicHandler(), "/v1/payments", 404, ""},
		{"login remains unmounted", PublicHandler(), "/v1/identity/login", 404, ""},
		{"session remains unmounted", PublicHandler(), "/v1/identity/session", 404, ""},
		{"csrf remains unmounted", PublicHandler(), "/v1/identity/csrf", 404, ""},
		{"logout remains unmounted", PublicHandler(), "/v1/identity/logout", 404, ""},
		{"login not on private listener", PrivateHandler(), "/v1/identity/login", 404, ""},
		{"session not on private listener", PrivateHandler(), "/v1/identity/session", 404, ""},
		{"csrf not on private listener", PrivateHandler(), "/v1/identity/csrf", 404, ""},
		{"logout not on private listener", PrivateHandler(), "/v1/identity/logout", 404, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			recorder := httptest.NewRecorder()
			tc.h.ServeHTTP(recorder, req)
			if recorder.Code != tc.code {
				t.Fatalf("status=%d want=%d", recorder.Code, tc.code)
			}
			if tc.contains != "" && !strings.Contains(recorder.Body.String(), tc.contains) {
				t.Fatalf("body=%s does not contain %s", recorder.Body.String(), tc.contains)
			}
		})
	}
}

// TestIdentitySessionCandidateMethodsRemainUnmounted checks the intended
// HTTP methods, not just GET. A future accidental POST login/logout mount
// would otherwise evade a GET-only 404 regression test.
func TestIdentitySessionCandidateMethodsRemainUnmounted(t *testing.T) {
	paths := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/identity/login"},
		{http.MethodGet, "/v1/identity/session"},
		{http.MethodGet, "/v1/identity/csrf"},
		{http.MethodPost, "/v1/identity/logout"},
	}
	for _, listener := range []struct {
		name string
		h    http.Handler
	}{
		{"public", PublicHandler()},
		{"private", PrivateHandler()},
	} {
		for _, target := range paths {
			t.Run(listener.name+" "+target.method+" "+target.path, func(t *testing.T) {
				req := httptest.NewRequest(target.method, target.path, nil)
				out := httptest.NewRecorder()
				listener.h.ServeHTTP(out, req)
				if out.Code != http.StatusNotFound {
					t.Fatalf("unapproved Identity route was mounted: %s %s (%d)", target.method, target.path, out.Code)
				}
			})
		}
	}
}
