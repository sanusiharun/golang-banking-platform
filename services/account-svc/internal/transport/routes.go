package transport

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/pkg/observability"
)

// RouterConfig holds dependencies needed to build the HTTP router.
// account-svc no longer has an AuthHandler — POST /auth/login is in auth-svc.
type RouterConfig struct {
	AccountHandler *AccountHandler
	Health         *observability.HealthHandler
	JWTConfig      pkgmiddleware.JWTConfig
	RateLimitRPS   int
	RateLimitBurst int
	RequestTimeout int // seconds
	Environment    string
}

// NewRouter builds the fully configured chi router.
//
// Route tiers (outermost → innermost):
//  1. Global middleware — request ID, logging, tracing, metrics, recovery
//  2. Public routes    — /healthz/*, /metrics, /auth/login
//  3. Debug routes     — /debug/* (local environment only)
//  4. Protected routes — /v1/* — rate limit + timeout + JWT auth + RBAC
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// ── Global middleware ─────────────────────────────────────────────────────
	r.Use(chimiddleware.RealIP)
	r.Use(pkgmiddleware.RequestID)
	r.Use(pkgmiddleware.RequestLogger) // uses global slog
	r.Use(pkgmiddleware.Tracing("account-svc"))
	r.Use(pkgmiddleware.NewMetrics("account_svc").Handler())
	r.Use(pkgmiddleware.Recovery)

	// ── Public routes ─────────────────────────────────────────────────────────
	r.Get("/healthz/live", cfg.Health.LivenessHandler())
	r.Get("/healthz/ready", cfg.Health.ReadinessHandler())
	r.Handle("/metrics", pkgmiddleware.PrometheusHandler())

	// ── Debug routes — local development only ─────────────────────────────────
	// These routes are stripped out in staging and production.
	// Use them to generate traces in Jaeger and trigger Alertmanager alert rules.
	//
	//   GET /debug/ping   → INFO log  + 200
	//   GET /debug/warn   → WARN log  + 200
	//   GET /debug/error  → ERROR log + 500  ← triggers log-based alert
	//   GET /debug/slow   → sleeps 3s        ← triggers latency alert
	//   GET /debug/panic  → panic recovered  ← tests Recovery middleware
	if cfg.Environment == "local" {
		debug := NewDebugHandler()
		r.Route("/debug", func(r chi.Router) {
			r.Get("/ping", debug.Ping)
			r.Get("/warn", debug.Warn)
			r.Get("/error", debug.Error)
			r.Get("/slow", debug.Slow)
			r.Get("/panic", debug.Panic)
		})
	}

	// ── Protected API routes ──────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(pkgmiddleware.Timeout(time.Duration(cfg.RequestTimeout) * time.Second))
		r.Use(pkgmiddleware.RateLimit(pkgmiddleware.RateLimitConfig{
			RequestsPerSecond: float64(cfg.RateLimitRPS),
			Burst:             float64(cfg.RateLimitBurst),
		}))
		r.Use(pkgmiddleware.Authenticate(cfg.JWTConfig))

		r.Route("/v1/accounts", func(r chi.Router) {
			r.With(pkgmiddleware.RequireRole("ADMIN")).
				Get("/", cfg.AccountHandler.ListAccounts)

			r.With(pkgmiddleware.RequireRole("ADMIN")).
				Post("/", cfg.AccountHandler.CreateAccount)

			r.Route("/{id}", func(r chi.Router) {
				r.With(pkgmiddleware.RequireRole("TELLER", "ADMIN")).
					Get("/", cfg.AccountHandler.GetAccount)

				r.With(pkgmiddleware.RequireRole("TELLER", "ADMIN")).
					Get("/balance", cfg.AccountHandler.GetBalance)

				r.With(pkgmiddleware.RequireRole("TELLER", "ADMIN")).
					Post("/credit", cfg.AccountHandler.Credit)

				r.With(pkgmiddleware.RequireRole("TELLER", "ADMIN")).
					Post("/debit", cfg.AccountHandler.Debit)
			})
		})
	})

	return r
}
