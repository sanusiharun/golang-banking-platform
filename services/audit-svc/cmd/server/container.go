package main

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/nats-io/nats.go"

	pkgaudit "github.com/sanusi/banking/pkg/audit"
	"github.com/sanusi/banking/pkg/database"
	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/pkg/observability"
	svcconfig "github.com/sanusi/banking/services/audit-svc/config"
	pgRepo "github.com/sanusi/banking/services/audit-svc/internal/repository/postgres"
	"github.com/sanusi/banking/services/audit-svc/internal/services"
	"github.com/sanusi/banking/services/audit-svc/internal/transport"
)

type container struct {
	server   *http.Server
	otel     *observability.Provider
	consumer *transport.Consumer
	nc       *nats.Conn // NATS connection — closed on graceful shutdown
}

// build wires all dependencies for audit-svc.
func build(ctx context.Context, cfg *svcconfig.Config) (*container, error) {
	// ── OpenTelemetry ─────────────────────────────────────────────────────────
	otelProvider, err := observability.Bootstrap(ctx, observability.Config{
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		Environment:    cfg.Environment,
		OTLPEndpoint:   cfg.OTelEndpoint,
		SamplingRate:   cfg.OTelSamplingRate,
		Enabled:        cfg.OTelEnabled,
		LogsEnabled:    cfg.OTelLogsEnabled,
	})
	if err != nil {
		return nil, fmt.Errorf("bootstrap otel: %w", err)
	}

	// ── RSA public key (for JWT verification only) ─────────────────────────────
	publicKey, err := parsePublicKey(cfg.JWTPublicKeyB64)
	if err != nil {
		return nil, fmt.Errorf("parse JWT public key: %w", err)
	}

	// ── Subject decryption key ─────────────────────────────────────────────────
	subjectKey, err := decodeBase64Key(cfg.JWTSubjectKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode subject encryption key: %w", err)
	}

	// ── Migrations ────────────────────────────────────────────────────────────
	if err := runMigrations(cfg); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// ── Database ──────────────────────────────────────────────────────────────
	db, err := database.New(&database.Config{
		Host:         cfg.DBHost,
		Port:         cfg.DBPort,
		Database:     cfg.DBName,
		User:         cfg.DBUser,
		Password:     cfg.DBPassword,
		SSLMode:      cfg.DBSSLMode,
		MaxOpenConns: cfg.DBMaxConns,
		MaxIdleConns: cfg.DBMinConns,
		LogLevel:     cfg.DBLogLevel,
	})
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	// ── NATS JetStream ─────────────────────────────────────────────────────────
	nc, err := nats.Connect(cfg.NATSUrl,
		nats.Name("audit-svc"),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1), // reconnect indefinitely
	)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	// RetryOnFailedConnect returns immediately even if not yet connected.
	// Wait for the connection to be established before proceeding.
	if err := waitForNATS(ctx, nc, cfg.NATSUrl); err != nil {
		return nil, err
	}
	slog.Info("nats connected", slog.String("url", cfg.NATSUrl))

	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("get jetstream context: %w", err)
	}

	// Ensure the AUDIT stream exists (idempotent — publishers also call this).
	if err := pkgaudit.EnsureStream(js); err != nil {
		return nil, fmt.Errorf("ensure audit stream: %w", err)
	}

	// ── Wiring ────────────────────────────────────────────────────────────────
	repo := pgRepo.New(db)
	auditSvc := services.New(repo)
	validate := validator.New()
	auditHandler := transport.NewAuditHandler(auditSvc, validate)

	// ── NATS consumer (started in main after build) ────────────────────────────
	consumer, err := transport.NewConsumer(js, cfg.NATSConsumer, auditSvc)
	if err != nil {
		return nil, fmt.Errorf("create nats consumer: %w", err)
	}

	// ── Health checks ─────────────────────────────────────────────────────────
	health := observability.NewHealthHandler()
	health.Register("postgres", func(hctx context.Context) error {
		return database.HealthCheck(hctx, db)
	})
	health.Register("nats", func(_ context.Context) error {
		if !nc.IsConnected() {
			return fmt.Errorf("nats disconnected")
		}
		return nil
	})

	// ── Router ────────────────────────────────────────────────────────────────
	router := transport.NewRouter(transport.RouterConfig{
		AuditHandler: auditHandler,
		Health:       health,
		JWTConfig: pkgmiddleware.JWTConfig{
			PublicKey:  publicKey,
			Issuer:     cfg.JWTIssuer,
			SubjectKey: subjectKey,
		},
		// audit-svc accepts API key auth too (no Redis — pass empty config)
		APIKeyConfig:   pkgmiddleware.APIKeyConfig{},
		RateLimitRPS:   cfg.RateLimitRPS,
		RateLimitBurst: cfg.RateLimitBurst,
		RequestTimeout: cfg.HandlerTimeout,
		Environment:    cfg.Environment,
	})

	// ── HTTP server ───────────────────────────────────────────────────────────
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return &container{
		server:   server,
		otel:     otelProvider,
		consumer: consumer,
		nc:       nc,
	}, nil
}

// ── key helpers (same as account-svc) ────────────────────────────────────────

func decodeBase64Key(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("subject key must be 32 bytes (AES-256), got %d", len(key))
	}
	return key, nil
}

// waitForNATS blocks until the NATS connection is established or ctx is cancelled.
// RetryOnFailedConnect returns a non-nil conn immediately even when not yet connected,
// so we must poll IsConnected before attempting JetStream operations.
func waitForNATS(ctx context.Context, nc *nats.Conn, url string) error {
	const maxWait = 10 * time.Second
	deadline := time.Now().Add(maxWait)
	for !nc.IsConnected() {
		if time.Now().After(deadline) {
			return fmt.Errorf("nats: timed out waiting for connection to %s after %s", url, maxWait)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("nats: context cancelled while waiting for connection: %w", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil
}

func parsePublicKey(b64 string) (*rsa.PublicKey, error) {
	pemBytes, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in public key")
	}
	keyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKIX public key: %w", err)
	}
	rsaKey, ok := keyInterface.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not an RSA public key (got %T)", keyInterface)
	}
	return rsaKey, nil
}
