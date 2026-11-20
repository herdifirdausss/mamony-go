package httpedge

import (
	"encoding/json"
	"net/http"
)

// PublicHandler exposes only process liveness, never private diagnostics.
func PublicHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", health("alive"))
	return mux
}

// PrivateHandler is process-only readiness (not database or financial readiness).
func PrivateHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", health("alive"))
	mux.HandleFunc("GET /readyz", health("http-ready"))
	return mux
}

func health(status string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(struct { Status string `json:"status"` }{Status:status})
	}
}
