package transport

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/pkg/observability"
)

// RouterConfig holds all dependencies for the notification-svc router.
type RouterConfig struct {
	NotificationHandler *NotificationHandler
	TemplateHandler     *TemplateHandler
	ScheduleHandler     *ScheduleHandler
	Health              *observability.HealthHandler
	JWTConfig           pkgmiddleware.JWTConfig
	APIKeyConfig        pkgmiddleware.APIKeyConfig
	RateLimitRPS        int
	RateLimitBurst      int
	RequestTimeout      int
	Environment         string
}

// NewRouter builds the fully configured chi router for notification-svc.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// ── Global middleware ─────────────────────────────────────────────────────
	r.Use(chimiddleware.RealIP) //nolint:staticcheck // known IP-spoofing risk (GHSA-3fxj-6jh8-hvhx): services are also directly reachable on their own ports per HANDOFF.md, not just via the Traefik gateway, so a caller can forge X-Forwarded-For. Tracked as a pending fix (trusted-proxy-scoped IP resolution), not silently accepted -- see HANDOFF.md.
	r.Use(pkgmiddleware.RequestID)
	r.Use(pkgmiddleware.RequestLogger)
	r.Use(pkgmiddleware.Tracing("notification-svc"))
	r.Use(pkgmiddleware.NewMetrics("notification_svc").Handler())
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

		// Notifications
		r.Route("/v1/notifications", func(r chi.Router) {
			r.With(pkgmiddleware.RequireRole("ADMIN", "TELLER")).
				Post("/", cfg.NotificationHandler.Send)

			r.With(pkgmiddleware.RequireRole("ADMIN")).
				Get("/", cfg.NotificationHandler.List)

			r.Route("/{id}", func(r chi.Router) {
				r.With(pkgmiddleware.RequireRole("ADMIN", "TELLER")).
					Get("/", cfg.NotificationHandler.GetByID)

				r.With(pkgmiddleware.RequireRole("ADMIN")).
					Post("/retry", cfg.NotificationHandler.Retry)

				r.With(pkgmiddleware.RequireRole("ADMIN", "TELLER")).
					Post("/cancel", cfg.NotificationHandler.Cancel)
			})
		})

		// Templates
		r.Route("/v1/templates", func(r chi.Router) {
			r.With(pkgmiddleware.RequireRole("ADMIN")).
				Post("/", cfg.TemplateHandler.Create)

			r.With(pkgmiddleware.RequireRole("ADMIN", "TELLER")).
				Get("/", cfg.TemplateHandler.List)

			r.Route("/{id}", func(r chi.Router) {
				r.With(pkgmiddleware.RequireRole("ADMIN", "TELLER")).
					Get("/", cfg.TemplateHandler.GetByID)

				r.With(pkgmiddleware.RequireRole("ADMIN")).
					Put("/", cfg.TemplateHandler.Update)

				r.With(pkgmiddleware.RequireRole("ADMIN")).
					Delete("/", cfg.TemplateHandler.Delete)

				r.With(pkgmiddleware.RequireRole("ADMIN", "TELLER")).
					Post("/preview", cfg.TemplateHandler.Preview)
			})
		})

		// Schedules
		r.Route("/v1/schedules", func(r chi.Router) {
			r.With(pkgmiddleware.RequireRole("ADMIN")).
				Post("/", cfg.ScheduleHandler.Create)

			r.With(pkgmiddleware.RequireRole("ADMIN")).
				Get("/", cfg.ScheduleHandler.List)

			r.Route("/{id}", func(r chi.Router) {
				r.With(pkgmiddleware.RequireRole("ADMIN")).
					Get("/", cfg.ScheduleHandler.GetByID)

				r.With(pkgmiddleware.RequireRole("ADMIN")).
					Put("/", cfg.ScheduleHandler.Update)

				r.With(pkgmiddleware.RequireRole("ADMIN")).
					Delete("/", cfg.ScheduleHandler.Delete)

				r.With(pkgmiddleware.RequireRole("ADMIN")).
					Post("/enable", cfg.ScheduleHandler.Enable)

				r.With(pkgmiddleware.RequireRole("ADMIN")).
					Post("/disable", cfg.ScheduleHandler.Disable)
			})
		})
	})

	return r
}
