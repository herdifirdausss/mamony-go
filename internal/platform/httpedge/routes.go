package httpedge

import (
	"encoding/json"
	"net/http"
)

// PublicHandler deliberately exposes only process liveness.
func PublicHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", health("alive"))
	return mux
}

// PrivateHandler exposes process-only readiness. It is not a financial service readiness probe.
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
		_ = json.NewEncoder(w).Encode(struct {
			Status string `json:"status"`
		}{Status: status})
	}
}
