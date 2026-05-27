package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/pkg/observability"
)

// RouterConfig holds all dependencies for the auth-svc router.
type RouterConfig struct {
	AuthHandler *AuthHandler
	Health      *observability.HealthHandler
	Environment string
}

// NewRouter builds the auth-svc chi router.
//
// All routes except health and metrics are public by design —
// auth-svc's job IS to authenticate, so it cannot protect itself with JWT.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// ── Global middleware ─────────────────────────────────────────────────────
	r.Use(chimiddleware.RealIP)
	r.Use(pkgmiddleware.RequestID)
	r.Use(pkgmiddleware.RequestLogger)
	r.Use(pkgmiddleware.Tracing("auth-svc"))
	r.Use(pkgmiddleware.NewMetrics("auth_svc").Handler())
	r.Use(pkgmiddleware.Recovery)

	// ── Health & metrics ──────────────────────────────────────────────────────
	r.Get("/healthz/live", cfg.Health.LivenessHandler())
	r.Get("/healthz/ready", cfg.Health.ReadinessHandler())
	r.Handle("/metrics", pkgmiddleware.PrometheusHandler())

	// ── Auth endpoints (public) ───────────────────────────────────────────────
	// POST /auth/login  — exchange credentials for a signed RS256 JWT
	r.Post("/auth/login", cfg.AuthHandler.Login)

	return r
}
