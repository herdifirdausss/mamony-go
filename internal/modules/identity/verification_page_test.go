package identity

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/handler"
	"github.com/herdifirdausss/mamony-go/internal/platform/httpedge"
)

func TestIdentityVerificationPageRemainsUnmounted(t *testing.T) {
	page, err := handler.NewEmailVerificationPage("https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "https://app.example.test/verify-email", nil)
	req.TLS = &tls.ConnectionState{}
	candidate := httptest.NewRecorder()
	page.ServeHTTP(candidate, req)
	if candidate.Code != http.StatusOK {
		t.Fatal("unmounted candidate must be constructible for synthetic tests")
	}
	for _, router := range []http.Handler{httpedge.PublicHandler(), httpedge.PrivateHandler()} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("Identity verification page unexpectedly mounted: %d", w.Code)
		}
	}
}
