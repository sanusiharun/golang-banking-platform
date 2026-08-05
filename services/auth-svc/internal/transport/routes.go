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
	AuthHandler   *AuthHandler
	APIKeyHandler *APIKeyHandler
	Health        *observability.HealthHandler
	Environment   string

	// JWT validation — used to protect internal/admin routes.
	PublicKey  *rsa.PublicKey
	SubjectKey []byte
	Issuer     string
}

// NewRouter builds the auth-svc chi router.
//
// Route groups:
//   - /auth/*          — public (no auth); auth-svc's job IS to authenticate
//   - /internal/*      — JWT-protected, admin role required; service account management
//   - /healthz/, /metrics — always public
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// ── Global middleware ─────────────────────────────────────────────────────
	r.Use(chimiddleware.RealIP) //nolint:staticcheck // known IP-spoofing risk (GHSA-3fxj-6jh8-hvhx): services are also directly reachable on their own ports per HANDOFF.md, not just via the Traefik gateway, so a caller can forge X-Forwarded-For. Tracked as a pending fix (trusted-proxy-scoped IP resolution), not silently accepted -- see HANDOFF.md.
	r.Use(pkgmiddleware.RequestID)
	r.Use(pkgmiddleware.RequestLogger)
	r.Use(pkgmiddleware.Tracing("auth-svc"))
	r.Use(pkgmiddleware.NewMetrics("auth_svc").Handler())
	r.Use(pkgmiddleware.Recovery)

	// ── Health & metrics ──────────────────────────────────────────────────────
	r.Get("/healthz/live", cfg.Health.LivenessHandler())
	r.Get("/healthz/ready", cfg.Health.ReadinessHandler())
	r.Handle("/metrics", pkgmiddleware.PrometheusHandler())

	jwtCfg := pkgmiddleware.JWTConfig{
		PublicKey:  cfg.PublicKey,
		Issuer:     cfg.Issuer,
		SubjectKey: cfg.SubjectKey,
	}

	// ── Auth endpoints (public) ───────────────────────────────────────────────
	r.Post("/auth/login", cfg.AuthHandler.Login)
	r.Post("/auth/refresh", cfg.AuthHandler.Refresh)
	r.With(pkgmiddleware.Authenticate(jwtCfg)).Post("/auth/logout", cfg.AuthHandler.Logout)

	// ── API key introspection — called by downstream services ────────────────
	// Accepts a SHA-256 hash, returns ServiceAccountIdentity. No JWT required.
	// Only reachable over the internal Docker network (banking-net).
	if cfg.APIKeyHandler != nil {
		r.Post("/auth/apikey/introspect", cfg.APIKeyHandler.IntrospectAPIKey)
	}

	// ── Internal / admin endpoints (JWT + admin role required) ───────────────
	// Service account and API key management must be done by human admins,
	// not by service accounts themselves (bootstrapping concern).
	if cfg.APIKeyHandler != nil {
		r.Group(func(r chi.Router) {
			r.Use(pkgmiddleware.Authenticate(jwtCfg))
			r.Use(pkgmiddleware.RequireRole("ADMIN"))

			// Service accounts
			r.Get("/internal/service-accounts", cfg.APIKeyHandler.ListServiceAccounts)
			r.Post("/internal/service-accounts", cfg.APIKeyHandler.CreateServiceAccount)
			r.Get("/internal/service-accounts/{id}", cfg.APIKeyHandler.GetServiceAccount)
			r.Patch("/internal/service-accounts/{id}", cfg.APIKeyHandler.UpdateServiceAccount)

			// API keys (scoped under their service account)
			r.Get("/internal/service-accounts/{id}/api-keys", cfg.APIKeyHandler.ListAPIKeys)
			r.Post("/internal/service-accounts/{id}/api-keys", cfg.APIKeyHandler.CreateAPIKey)
			r.Delete("/internal/service-accounts/{id}/api-keys/{keyID}", cfg.APIKeyHandler.RevokeAPIKey)
		})
	}

	// ── Debug endpoints (local only) ──────────────────────────────────────────
	if cfg.Environment == "local" {
		inspect := NewInspectHandler(cfg.PublicKey, cfg.SubjectKey, cfg.Issuer)
		r.Post("/auth/inspect", inspect.Inspect)
	}

	return r
}
