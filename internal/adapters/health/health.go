// Package health provides a small HTTP server exposing a readiness/health
// endpoint so platforms like Railway can healthcheck the (otherwise SMTP-only)
// gateway over HTTP.
package health

import (
	"encoding/json"
	"net/http"
)

// ReadyFunc reports whether the service is ready to accept and relay mail.
type ReadyFunc func() bool

// Handler returns an http.Handler serving GET /health.
//
// It responds 200 {"status":"ok"} when ready returns true, and
// 503 {"status":"unavailable"} otherwise. Any method other than GET/HEAD on
// /health yields 405, and unknown paths yield 404.
func Handler(ready ReadyFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status := http.StatusOK
		body := map[string]string{"status": "ok"}
		if ready == nil || !ready() {
			status = http.StatusServiceUnavailable
			body = map[string]string{"status": "unavailable"}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	})
	return mux
}

// NewServer builds an *http.Server bound to addr serving the health handler.
func NewServer(addr string, ready ReadyFunc) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: Handler(ready),
	}
}
