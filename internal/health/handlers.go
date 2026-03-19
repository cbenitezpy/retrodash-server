package health

import "net/http"

// LivenessHandler returns an http.HandlerFunc for the /healthz liveness probe.
// Always returns 200 "ok" — if the server can respond, it's alive.
// MUST NOT check dependencies (browser, network) to avoid cascade restarts.
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

// ReadinessHandler returns an http.HandlerFunc for the /readyz readiness probe.
// Returns 200 "ok" when the StatusProvider reports ready or when the server is in
// standby mode (waiting for configuration via API). Returns 503 "not ready" otherwise.
// The modeProvider parameter is optional; if nil, only the StatusProvider is checked.
func ReadinessHandler(provider StatusProvider, modeProvider ModeProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain")

		// Standby mode: server is ready to receive configuration via API
		if modeProvider != nil && modeProvider.Mode() == "standby" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}

		if provider != nil && provider.IsReady() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
		}
	}
}
