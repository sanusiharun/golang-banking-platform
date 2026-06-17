// Command server is the entrypoint for notification-svc.
// Responsibilities:
//  1. Load config (fail fast on missing required vars)
//  2. Configure global slog logger
//  3. Build the dependency container (DB, NATS, channel registry, workers, HTTP server)
//  4. Start the NATS consumer, dispatcher worker pool, and scheduler worker in goroutines
//  5. Start the HTTP server
//  6. Graceful shutdown on SIGTERM / SIGINT
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

	"github.com/sanusi/banking/pkg/logger"
	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	svcconfig "github.com/sanusi/banking/services/notification-svc/config"
)

func main() {
	if err := run(); err != nil {
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
	logger.Setup(logger.Config{
		Level:            cfg.LogLevel,
		Format:           logger.Format(cfg.LogFormat),
		ServiceName:      cfg.ServiceName,
		Version:          cfg.ServiceVersion,
		Environment:      cfg.Environment,
		OTelTraceContext: cfg.OTelEnabled,
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
		},
	})

	slog.Info("starting notification-svc")

	// ── 3. Wire dependencies ──────────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, err := build(ctx, cfg)
	if err != nil {
		return fmt.Errorf("build container: %w", err)
	}

	if cfg.OTelEnabled && cfg.OTelLogsEnabled {
		logger.AttachOTelBridge(cfg.ServiceName)
	}

	defer func() {
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutCancel()
		if err := c.otel.Shutdown(shutCtx); err != nil {
			slog.Error("otel shutdown", slog.String("error", err.Error()))
		}
		c.nc.Drain() //nolint:errcheck — best-effort drain on shutdown
	}()

	// ── 4. Start background workers ───────────────────────────────────────────
	go func() { c.consumer.Start(ctx) }()
	go func() { c.dispatcher.Start(ctx) }()
	go func() { c.scheduler.Start(ctx) }()

	// ── 5. Start HTTP server ──────────────────────────────────────────────────
	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("http server listening", slog.String("addr", c.server.Addr))
		if err := c.server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	// ── 6. Wait for signal or server error ───────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)
	case sig := <-quit:
		slog.Info("shutdown signal received", slog.String("signal", sig.String()))
	}

	// ── 7. Graceful shutdown ──────────────────────────────────────────────────
	cancel() // stops all background workers

	shutCtx, shutCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutCancel()

	if err := c.server.Shutdown(shutCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	slog.Info("notification-svc stopped cleanly")
	return nil
}
