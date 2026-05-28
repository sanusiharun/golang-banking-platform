package transport

import (
	"crypto/rsa"
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

	// Used by InspectHandler (local dev only).
	PublicKey  *rsa.PublicKey
	SubjectKey []byte
	Issuer     string
}

// NewRouter builds the auth-svc chi router.
//
// All routes except health and metrics are public by design —
// auth-svc's job IS to authenticate, so it cannot protect itself with JWT.
//
// In local environment only, POST /auth/inspect is registered for debugging tokens.
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
	r.Post("/auth/login",   cfg.AuthHandler.Login)
	r.Post("/auth/refresh", cfg.AuthHandler.Refresh)
	r.Post("/auth/logout",  cfg.AuthHandler.Logout)

	// ── Debug endpoints (local only) ──────────────────────────────────────────
	// POST /auth/inspect — decode and decrypt a JWT for debugging
	if cfg.Environment == "local" {
		inspect := NewInspectHandler(cfg.PublicKey, cfg.SubjectKey, cfg.Issuer)
		r.Post("/auth/inspect", inspect.Inspect)
	}

	return r
}
