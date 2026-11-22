package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/platform/httpedge"
)

// This is a release-deny gate, not an assertion that password recovery has
// been implemented. The first recovery slice contributes ONLY a proof codec.
// Future changes must explicitly review and replace this safety contract.
func TestPasswordRecoveryRoutesRemainUnmounted(t *testing.T) {
	for _, handler := range []struct {
		name string
		mux  http.Handler
	}{
		{"public", httpedge.PublicHandler()},
		{"private", httpedge.PrivateHandler()},
	} {
		for _, route := range []struct {
			method, path string
		}{
			{http.MethodPost, "/v1/identity/password-recovery/request"},
			{http.MethodPost, "/v1/identity/password-recovery/redeem"},
			{http.MethodGet, "/account/recover-password"},
		} {
			t.Run(handler.name+" "+route.method+" "+route.path, func(t *testing.T) {
				response := httptest.NewRecorder()
				handler.mux.ServeHTTP(response, httptest.NewRequest(
					route.method, "https://app.example.test"+route.path, nil))
				if response.Code != http.StatusNotFound ||
					response.Header().Get("Set-Cookie") != "" {
					t.Fatalf("unapproved password recovery route mounted: %s %s -> %d",
						route.method, route.path, response.Code)
				}
			})
		}
	}
}
