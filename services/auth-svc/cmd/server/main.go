// Command server is the entrypoint for auth-svc.
// Responsibilities:
//  1. Load config (fail fast on missing required vars)
//  2. Configure the global slog logger
//  3. Build the dependency container
//  4. Start the HTTP server
//  5. Graceful shutdown on SIGTERM / SIGINT
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
	svcconfig "github.com/sanusi/banking/services/auth-svc/config"
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

	slog.Info("starting auth-svc")

	// ── 3. Wire dependencies ──────────────────────────────────────────────────
	ctx := context.Background()
	c, err := build(ctx, cfg)
	if err != nil {
		return fmt.Errorf("build container: %w", err)
	}

	if cfg.OTelEnabled && cfg.OTelLogsEnabled {
		logger.AttachOTelBridge(cfg.ServiceName)
	}

	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := c.otel.Shutdown(shutCtx); err != nil {
			slog.Error("otel shutdown", slog.String("error", err.Error()))
		}
	}()

	// ── 4. Start background jobs ──────────────────────────────────────────────
	if c.idempotencyStore != nil {
		go runIdempotencyCleanup(ctx, c.idempotencyStore, cfg.ShutdownTimeout)
	}

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
	shutCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := c.server.Shutdown(shutCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	slog.Info("auth-svc stopped cleanly")
	return nil
}

// runIdempotencyCleanup deletes expired idempotency records every hour.
// Runs until ctx is cancelled (i.e. on graceful shutdown).
func runIdempotencyCleanup(ctx context.Context, store *idempotencyCleanupStore, _ time.Duration) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	slog.Info("idempotency cleanup goroutine started")

	for {
		select {
		case <-ctx.Done():
			slog.Info("idempotency cleanup goroutine stopped")
			return
		case <-ticker.C:
			deleted, err := store.DeleteExpired(ctx)
			if err != nil {
				slog.Error("idempotency cleanup: failed to delete expired records",
					slog.String("error", err.Error()),
				)
			} else if deleted > 0 {
				slog.Info("idempotency cleanup: deleted expired records",
					slog.Int64("count", deleted),
				)
			}
		}
	}
}
