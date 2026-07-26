package transport

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/pkg/observability"
)

// RouterConfig holds dependencies needed to build the audit-svc HTTP router.
type RouterConfig struct {
	AuditHandler   *AuditHandler
	Health         *observability.HealthHandler
	JWTConfig      pkgmiddleware.JWTConfig
	APIKeyConfig   pkgmiddleware.APIKeyConfig
	RateLimitRPS   int
	RateLimitBurst int
	RequestTimeout int // seconds
	Environment    string
}

// NewRouter builds the fully configured chi router for audit-svc.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// ── Global middleware ─────────────────────────────────────────────────────
	r.Use(chimiddleware.RealIP) //nolint:staticcheck // known IP-spoofing risk (GHSA-3fxj-6jh8-hvhx): services are also directly reachable on their own ports per HANDOFF.md, not just via the Traefik gateway, so a caller can forge X-Forwarded-For. Tracked as a pending fix (trusted-proxy-scoped IP resolution), not silently accepted -- see HANDOFF.md.
	r.Use(pkgmiddleware.RequestID)
	r.Use(pkgmiddleware.RequestLogger)
	r.Use(pkgmiddleware.Tracing("audit-svc"))
	r.Use(pkgmiddleware.NewMetrics("audit_svc").Handler())
	r.Use(pkgmiddleware.Recovery)

	// ── Public routes ─────────────────────────────────────────────────────────
	r.Get("/healthz/live", cfg.Health.LivenessHandler())
	r.Get("/healthz/ready", cfg.Health.ReadinessHandler())
	r.Handle("/metrics", pkgmiddleware.PrometheusHandler())

	// ── Protected API routes ──────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(pkgmiddleware.Timeout(time.Duration(cfg.RequestTimeout) * time.Second))
		r.Use(pkgmiddleware.RateLimit(pkgmiddleware.RateLimitConfig{
			RequestsPerSecond: float64(cfg.RateLimitRPS),
			Burst:             float64(cfg.RateLimitBurst),
		}))
		r.Use(pkgmiddleware.AuthenticateAny(cfg.JWTConfig, cfg.APIKeyConfig))

		r.Route("/v1/audit", func(r chi.Router) {
			// Sync ingest (HTTP fallback path for compliance-sensitive operations)
			r.With(pkgmiddleware.RequireRole("ADMIN", "TELLER")).
				Post("/events", cfg.AuditHandler.IngestEvent)

			// Query endpoints — ADMIN only for now (per plan: internal access)
			r.With(pkgmiddleware.RequireRole("ADMIN")).
				Get("/events", cfg.AuditHandler.ListEvents)

			r.With(pkgmiddleware.RequireRole("ADMIN")).
				Get("/events/{id}", cfg.AuditHandler.GetEvent)

			r.With(pkgmiddleware.RequireRole("ADMIN")).
				Get("/actors/{actor_id}/events", cfg.AuditHandler.ListActorEvents)

			r.With(pkgmiddleware.RequireRole("ADMIN")).
				Get("/resources/{resource}/{resource_id}/events", cfg.AuditHandler.ListResourceEvents)
		})
	})

	return r
}
