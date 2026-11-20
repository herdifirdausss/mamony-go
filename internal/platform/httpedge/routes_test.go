package httpedge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouteIsolation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {name string; h http.Handler; path string; code int; body string}{
		{"public live",PublicHandler(),"/livez",200,"alive"},
		{"private ready",PrivateHandler(),"/readyz",200,"http-ready"},
		{"private route hidden",PublicHandler(),"/readyz",404,""},
		{"unimplemented payment",PublicHandler(),"/v1/payments",404,""},
	} {
		t.Run(tc.name,func(t *testing.T){
			rec := httptest.NewRecorder()
			tc.h.ServeHTTP(rec,httptest.NewRequest(http.MethodGet,tc.path,nil))
			if rec.Code != tc.code {t.Fatalf("got %d want %d",rec.Code,tc.code)}
			if tc.body != "" && !strings.Contains(rec.Body.String(),tc.body) {t.Fatalf("body=%s",rec.Body.String())}
		})
	}
}
