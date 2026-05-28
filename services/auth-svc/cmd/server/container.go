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

	"github.com/go-playground/validator/v10"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/sanusi/banking/pkg/database"
	"github.com/sanusi/banking/pkg/observability"
	svcconfig "github.com/sanusi/banking/services/auth-svc/config"
	"github.com/sanusi/banking/services/auth-svc/internal/repository"
	"github.com/sanusi/banking/services/auth-svc/internal/services"
	"github.com/sanusi/banking/services/auth-svc/internal/transport"
)

type container struct {
	server *http.Server
	otel   *observability.Provider
}

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

	// ── RSA private key ────────────────────────────────────────────────────────
	privateKey, err := parsePrivateKey(cfg.JWTPrivateKeyB64)
	if err != nil {
		return nil, fmt.Errorf("parse JWT private key: %w", err)
	}

	// ── Subject encryption key ────────────────────────────────────────────────
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

	// ── Token store (pluggable: postgres | redis | memory) ────────────────────
	tokenStore, redisClient := buildTokenStore(cfg, db)

	// ── Wiring ────────────────────────────────────────────────────────────────
	userRepo := repository.NewUserRepository(db)
	validate := validator.New()

	authSvc := services.NewAuthService(userRepo, tokenStore, services.AuthConfig{
		PrivateKey:           privateKey,
		Issuer:               cfg.JWTIssuer,
		AccessTokenTTL:       cfg.AccessTokenTTL,
		RefreshTokenTTL:      cfg.RefreshTokenTTL,
		SubjectEncryptionKey: subjectKey,
		BCryptCost:           cfg.BCryptCost,
	})

	authHandler := transport.NewAuthHandler(authSvc, validate)

	// ── Health checks ─────────────────────────────────────────────────────────
	health := observability.NewHealthHandler()
	health.Register("postgres", func(hctx context.Context) error {
		return database.HealthCheck(hctx, db)
	})
	if redisClient != nil {
		health.Register("redis", func(hctx context.Context) error {
			return redisClient.Ping(hctx).Err()
		})
	}

	// ── Router ────────────────────────────────────────────────────────────────
	router := transport.NewRouter(transport.RouterConfig{
		AuthHandler: authHandler,
		Health:      health,
		Environment: cfg.Environment,
		PublicKey:   &privateKey.PublicKey,
		SubjectKey:  subjectKey,
		Issuer:      cfg.JWTIssuer,
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

// buildTokenStore selects a TokenStore implementation based on cfg.TokenStore.
// It returns the store and, if Redis is used, the *redis.Client so it can be
// registered as a health-check target. A nil *redis.Client means Redis is not used.
func buildTokenStore(cfg *svcconfig.Config, db *gorm.DB) (repository.TokenStore, *redis.Client) {
	switch cfg.TokenStore {
	case "redis":
		client := redis.NewClient(&redis.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
		})
		slog.Info("token store: redis", slog.String("addr", cfg.RedisAddr))
		return repository.NewRedisTokenStore(client, cfg.RefreshTokenTTL), client

	case "memory":
		slog.Warn("token store: in-memory (not suitable for production)")
		return repository.NewMemoryTokenStore(), nil

	default: // "postgres" or anything unrecognised
		if cfg.TokenStore != "postgres" {
			slog.Warn("token store: unknown value, falling back to postgres",
				slog.String("value", cfg.TokenStore))
		}
		slog.Info("token store: postgres")
		return repository.NewPostgresTokenStore(db), nil
	}
}

// decodeBase64Key decodes a standard base64-encoded AES key from an env var.
// Returns nil (no error) when the value is empty — callers treat nil as "no encryption".
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

// parsePrivateKey decodes a base64-encoded PKCS#8 PEM private key.
// The base64 encoding allows the key to be stored safely in an env var.
func parsePrivateKey(b64 string) (*rsa.PrivateKey, error) {
	pemBytes, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in private key")
	}

	// openssl genrsa produces PKCS#8 format ("BEGIN PRIVATE KEY")
	keyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS#8 private key: %w", err)
	}

	rsaKey, ok := keyInterface.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not an RSA private key (got %T)", keyInterface)
	}

	return rsaKey, nil
}
