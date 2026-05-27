package observability

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Checker is a function that reports the health of a dependency.
// It returns an error if the dependency is unhealthy.
type Checker func(ctx context.Context) error

// HealthStatus represents the overall health status.
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
)

// HealthResponse is the JSON payload returned by health endpoints.
type HealthResponse struct {
	Status    HealthStatus              `json:"status"`
	Timestamp time.Time                 `json:"timestamp"`
	Uptime    string                    `json:"uptime"`
	Checks    map[string]CheckResult    `json:"checks,omitempty"`
}

// CheckResult holds the result of a single health check.
type CheckResult struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
	Latency string       `json:"latency,omitempty"`
}

// HealthHandler manages liveness and readiness endpoints.
type HealthHandler struct {
	mu       sync.RWMutex
	checkers map[string]Checker
	startedAt time.Time
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{
		checkers:  make(map[string]Checker),
		startedAt: time.Now(),
	}
}

// Register adds a named health checker (e.g., "postgres", "redis").
func (h *HealthHandler) Register(name string, checker Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers[name] = checker
}

// LivenessHandler returns HTTP 200 immediately — the process is alive.
// Kubernetes uses this to decide whether to restart the pod.
func (h *HealthHandler) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":    "alive",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// ReadinessHandler runs all registered checkers and returns 200 only if all pass.
// Kubernetes uses this to decide whether to route traffic to the pod.
func (h *HealthHandler) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.mu.RLock()
		checkers := make(map[string]Checker, len(h.checkers))
		for k, v := range h.checkers {
			checkers[k] = v
		}
		h.mu.RUnlock()

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		checks := make(map[string]CheckResult, len(checkers))
		overallStatus := StatusHealthy

		for name, checker := range checkers {
			start := time.Now()
			err := checker(ctx)
			latency := time.Since(start)

			result := CheckResult{
				Status:  StatusHealthy,
				Latency: latency.String(),
			}
			if err != nil {
				result.Status = StatusUnhealthy
				result.Message = err.Error()
				overallStatus = StatusUnhealthy
			}
			checks[name] = result
		}

		resp := HealthResponse{
			Status:    overallStatus,
			Timestamp: time.Now().UTC(),
			Uptime:    time.Since(h.startedAt).String(),
			Checks:    checks,
		}

		statusCode := http.StatusOK
		if overallStatus == StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
