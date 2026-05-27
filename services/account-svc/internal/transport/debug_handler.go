package transport

import (
	"log/slog"
	"net/http"
	"time"
)

// DebugHandler exposes test endpoints for local development only.
// These routes are NEVER registered in staging or production.
//
// Endpoints:
//
//	GET /debug/ping        — returns pong, emits INFO log
//	GET /debug/warn        — emits WARN log, returns 200
//	GET /debug/error       — emits ERROR log, returns 500
//	GET /debug/slow        — sleeps 3 seconds (tests latency alerts)
//	GET /debug/panic       — triggers a panic (tests recovery middleware)
type DebugHandler struct{}

func NewDebugHandler() *DebugHandler {
	return &DebugHandler{}
}

// Ping emits an INFO log and returns 200. Use to verify traces appear in Jaeger.
func (h *DebugHandler) Ping(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "debug ping",
		slog.String("endpoint", "/debug/ping"),
		slog.String("remote_addr", r.RemoteAddr),
	)
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "pong",
	})
}

// Warn emits a WARN log. Use to verify warn-level logs appear in Grafana.
func (h *DebugHandler) Warn(w http.ResponseWriter, r *http.Request) {
	slog.WarnContext(r.Context(), "debug warning emitted",
		slog.String("endpoint", "/debug/warn"),
		slog.String("hint", "this is a test warning — check Grafana logs"),
	)
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "warning emitted — check Grafana logs",
	})
}

// Error emits an ERROR log and returns 500. Use to trigger Alertmanager alert rules.
func (h *DebugHandler) Error(w http.ResponseWriter, r *http.Request) {
	slog.ErrorContext(r.Context(), "debug error emitted",
		slog.String("endpoint", "/debug/error"),
		slog.String("hint", "this is a test error — check Alertmanager"),
	)
	writeJSON(w, http.StatusInternalServerError, map[string]string{
		"status":  "error",
		"message": "error emitted — check Prometheus alerts and Alertmanager",
	})
}

// Slow sleeps for 3 seconds. Use to trigger latency-based alert rules.
func (h *DebugHandler) Slow(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "debug slow request started",
		slog.String("endpoint", "/debug/slow"),
	)

	select {
	case <-time.After(3 * time.Second):
		slog.InfoContext(r.Context(), "debug slow request completed",
			slog.String("duration", "3s"),
		)
		writeJSON(w, http.StatusOK, map[string]string{
			"status":   "ok",
			"message":  "slow response after 3s — check Grafana latency metrics",
			"duration": "3s",
		})
	case <-r.Context().Done():
		// Request was cancelled (e.g. by Timeout middleware)
		slog.WarnContext(r.Context(), "debug slow request cancelled by context",
			slog.String("reason", r.Context().Err().Error()),
		)
	}
}

// Panic triggers a panic to test the Recovery middleware.
// The recovery middleware catches it, logs it as ERROR, and returns 500.
func (h *DebugHandler) Panic(w http.ResponseWriter, r *http.Request) {
	slog.InfoContext(r.Context(), "debug panic about to fire",
		slog.String("endpoint", "/debug/panic"),
	)
	panic("intentional debug panic — testing Recovery middleware")
}
