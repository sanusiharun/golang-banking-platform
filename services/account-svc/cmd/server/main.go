// Command server is the entrypoint for account-svc.
// Responsibilities:
//   1. Load config (fail fast on missing required vars)
//   2. Configure the global slog logger — must happen before any other init
//   3. Build the dependency container
//   4. Start the HTTP server
//   5. Graceful shutdown on SIGTERM / SIGINT
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/pkg/logger"
	svcconfig "github.com/sanusi/banking/services/account-svc/config"
)

func main() {
	if err := run(); err != nil {
		// slog may not be configured yet if config load failed,
		// so fall back to plain stderr before exiting.
		slog.Error("fatal", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	// ── 1. Config ─────────────────────────────────────────────────────────────
	cfg, err := svcconfig.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// ── 2. Global slog logger ─────────────────────────────────────────────────
	// Must be called before build() so every package that calls slog.Info/Error
	// picks up the correct handler, level, and service/version fields.
	//
	// Extractors pull request-scoped values from the log context on every call.
	// They reference pkg/middleware functions — the logger package itself has
	// zero knowledge of httpx, JWT, or any domain package.
	logger.Setup(logger.Config{
		Level:            cfg.LogLevel,
		Format:           logger.Format(cfg.LogFormat),
		ServiceName:      cfg.ServiceName,
		Version:          cfg.ServiceVersion,
		Environment:      cfg.Environment,
		OTelTraceContext: cfg.OTelEnabled, // inject trace_id/span_id when OTel is on
		Extractors: []logger.ContextExtractor{
			{
				Key:     "request_id",
				Extract: pkgmiddleware.RequestIDFromContext,
			},
			{
				Key: "user_id",
				Extract: func(ctx context.Context) string {
					if c, ok := pkgmiddleware.ClaimsFromContext(ctx); ok {
						return c.UserID
					}
					return ""
				},
			},
			{
				Key: "tenant_id",
				Extract: func(ctx context.Context) string {
					if c, ok := pkgmiddleware.ClaimsFromContext(ctx); ok {
						return c.TenantID
					}
					return ""
				},
			},
		},
	})

	// service, version, env are already pinned to every record via logger.Setup —
	// do NOT repeat them here or they will appear twice.
	slog.Info("starting account-svc")

	// ── 3. Wire dependencies ──────────────────────────────────────────────────
	ctx := context.Background()

	c, err := build(ctx, cfg)
	if err != nil {
		return fmt.Errorf("build container: %w", err)
	}

	// ── 3b. Attach OTel log bridge ────────────────────────────────────────────
	// Must happen AFTER build() so observability.Bootstrap has set the global
	// OTel log provider. Only attach when OTEL_LOGS_ENABLED=true — requires a
	// backend that implements LogsService (not Jaeger). Default: false.
	if cfg.OTelEnabled && cfg.OTelLogsEnabled {
		logger.AttachOTelBridge(cfg.ServiceName)
		slog.Info("OTel log bridge attached — logs flowing via OTLP")
	}

	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := c.otel.Shutdown(shutCtx); err != nil {
			slog.Error("otel shutdown", slog.String("error", err.Error()))
		}
	}()

	// ── 4. Start HTTP server ──────────────────────────────────────────────────
	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("http server listening", slog.String("addr", c.server.Addr))
		if err := c.server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	// ── 5. Wait for signal or server error ───────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)
	case sig := <-quit:
		slog.Info("shutdown signal received", slog.String("signal", sig.String()))
	}

	// ── 6. Graceful shutdown ──────────────────────────────────────────────────
	shutCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := c.server.Shutdown(shutCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	slog.Info("account-svc stopped cleanly")
	return nil
}
