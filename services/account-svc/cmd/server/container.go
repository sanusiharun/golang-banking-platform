package main

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"

	"github.com/go-playground/validator/v10"

	"github.com/sanusi/banking/pkg/database"
	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/pkg/observability"
	svcconfig "github.com/sanusi/banking/services/account-svc/config"
	"github.com/sanusi/banking/services/account-svc/internal/repository"
	"github.com/sanusi/banking/services/account-svc/internal/services"
	"github.com/sanusi/banking/services/account-svc/internal/transport"
)

type container struct {
	server *http.Server
	otel   *observability.Provider
}

// build wires all dependencies for account-svc.
// account-svc holds ONLY the RSA public key — it can verify JWTs but never issue them.
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

	// ── Repositories & services ───────────────────────────────────────────────
	accountRepo := repository.NewAccountRepository(db)
	validate := validator.New()
	accountSvc := services.NewAccountService(accountRepo)
	accountHandler := transport.NewAccountHandler(accountSvc, validate)

	// ── Health checks ─────────────────────────────────────────────────────────
	health := observability.NewHealthHandler()
	health.Register("postgres", func(hctx context.Context) error {
		return database.HealthCheck(hctx, db)
	})

	// ── Router ────────────────────────────────────────────────────────────────
	router := transport.NewRouter(transport.RouterConfig{
		AccountHandler: accountHandler,
		Health:         health,
		JWTConfig: pkgmiddleware.JWTConfig{
			PublicKey: publicKey,
			Issuer:    cfg.JWTIssuer,
		},
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

	return &container{server: server, otel: otelProvider}, nil
}

// parsePublicKey decodes a base64-encoded PKIX PEM public key.
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
