package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/pkg/observability"
)

// RouterConfig holds all dependencies for the payment-svc HTTP router.
type RouterConfig struct {
	PaymentHandler  *PaymentHandler
	InquiryHandler  *InquiryHandler
	Health          *observability.HealthHandler
	JWTConfig       pkgmiddleware.JWTConfig
	APIKeyConfig    pkgmiddleware.APIKeyConfig
	RateLimitRPS    int
	RateLimitBurst  int
	RequestTimeout  int
	Environment     string
}

// NewRouter builds the fully configured chi router for payment-svc.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// ── Global middleware ─────────────────────────────────────────────────────
	r.Use(chimiddleware.RealIP)
	r.Use(pkgmiddleware.RequestID)
	r.Use(pkgmiddleware.RequestLogger)
	r.Use(pkgmiddleware.Tracing("payment-svc"))
	r.Use(pkgmiddleware.NewMetrics("payment_svc").Handler())
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

		// Payment initiation
		r.Route("/v1/payments", func(r chi.Router) {
			r.With(pkgmiddleware.RequireRole("TELLER", "ADMIN")).
				Post("/transfer", cfg.PaymentHandler.Transfer)

			r.With(pkgmiddleware.RequireRole("TELLER", "ADMIN")).
				Post("/merchant", cfg.PaymentHandler.MerchantPayment)

			r.With(pkgmiddleware.RequireRole("ADMIN")).
				Post("/fee", cfg.PaymentHandler.Fee)

			r.With(pkgmiddleware.RequireRole("TELLER", "ADMIN")).
				Post("/refund", cfg.PaymentHandler.Refund)

			// Inquiry
			r.With(pkgmiddleware.RequireRole("TELLER", "ADMIN")).
				Get("/", cfg.InquiryHandler.List)

			r.Route("/{id}", func(r chi.Router) {
				r.With(pkgmiddleware.RequireRole("TELLER", "ADMIN")).
					Get("/", cfg.InquiryHandler.GetByID)

				r.With(pkgmiddleware.RequireRole("TELLER", "ADMIN")).
					Post("/reverse", cfg.PaymentHandler.Reverse)

				r.With(pkgmiddleware.RequireRole("TELLER", "ADMIN")).
					Post("/cancel", cfg.PaymentHandler.Cancel)

				r.With(pkgmiddleware.RequireRole("TELLER", "ADMIN")).
					Post("/retry", cfg.PaymentHandler.Retry)
			})
		})
	})

	return r
}
