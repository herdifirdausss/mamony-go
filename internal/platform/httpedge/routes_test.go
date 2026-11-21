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
