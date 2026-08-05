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
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	pkgaudit "github.com/sanusi/banking/pkg/audit"
	"github.com/sanusi/banking/pkg/database"
	"github.com/sanusi/banking/pkg/featureflag"
	"github.com/sanusi/banking/pkg/idempotency"
	"github.com/sanusi/banking/pkg/observability"
	svcconfig "github.com/sanusi/banking/services/auth-svc/config"
	"github.com/sanusi/banking/services/auth-svc/internal/repository"
	"github.com/sanusi/banking/services/auth-svc/internal/services"
	"github.com/sanusi/banking/services/auth-svc/internal/transport"
)

// idempotencyCleanupStore is a type alias so main.go can reference DeleteExpired
// without importing the idempotency package directly.
type idempotencyCleanupStore = idempotency.PostgresStore

type container struct {
	server           *http.Server
	otel             *observability.Provider
	idempotencyStore *idempotencyCleanupStore // nil when Postgres store is unavailable
	nc               *nats.Conn               // nil when NATS is not configured
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
	slog.Info("otel bootstrap complete",
		slog.Bool("enabled", cfg.OTelEnabled),
		slog.String("endpoint", cfg.OTelEndpoint),
		slog.Float64("sampling_rate", cfg.OTelSamplingRate),
	)

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
	if migrateErr := runMigrations(cfg); migrateErr != nil {
		return nil, fmt.Errorf("run migrations: %w", migrateErr)
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

	// ── API Key store (postgres + optional redis cache) ────────────────────────
	apiKeyStore, saStore := buildAPIKeyStore(db, redisClient)

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

	apiKeySvc := services.NewAPIKeyService(saStore, apiKeyStore, cfg.Environment)

	featureflag.Init(cfg.FliptURL, "default")

	// ── Audit publisher ────────────────────────────────────────────────────────
	auditPublisher, nc := pkgaudit.NewPublisher(ctx, pkgaudit.PublisherConfig{
		NATSURL:     cfg.NATSUrl,
		ServiceName: "auth-svc",
	})

	authHandler := transport.NewAuthHandler(authSvc, validate, auditPublisher)
	apiKeyHandler := transport.NewAPIKeyHandler(apiKeySvc, validate, auditPublisher)

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
		AuthHandler:            authHandler,
		APIKeyHandler:          apiKeyHandler,
		Health:                 health,
		Environment:            cfg.Environment,
		PublicKey:              &privateKey.PublicKey,
		SubjectKey:             subjectKey,
		Issuer:                 cfg.JWTIssuer,
		IntrospectSharedSecret: cfg.IntrospectSharedSecret,
	})

	// ── Idempotency store ─────────────────────────────────────────────────────
	pgIdempStore := idempotency.NewPostgresStore(db, 24*time.Hour)
	var idempStore idempotency.Store
	if redisClient != nil {
		redisIdempStore := idempotency.NewRedisStore(redisClient, 24*time.Hour)
		idempStore = idempotency.NewDualStore(redisIdempStore, pgIdempStore)
	} else {
		idempStore = pgIdempStore
	}
	_ = idempStore // used by account-svc and other services via middleware injection

	// ── HTTP server ───────────────────────────────────────────────────────────
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return &container{
		server:           server,
		otel:             otelProvider,
		idempotencyStore: pgIdempStore, // cleanup goroutine always uses postgres directly
		nc:               nc,
	}, nil
}

// buildAPIKeyStore wires the API key and service account stores.
// When Redis is available, the API key store is wrapped with a Redis cache layer.
func buildAPIKeyStore(db *gorm.DB, redisClient *redis.Client) (repository.APIKeyStore, repository.ServiceAccountStore) {
	saStore := repository.NewPostgresServiceAccountStore(db)
	pgKeyStore := repository.NewPostgresAPIKeyStore(db)

	if redisClient != nil {
		slog.Info("api key store: postgres + redis cache")
		return repository.NewRedisAPIKeyStore(pgKeyStore, redisClient), saStore
	}

	slog.Info("api key store: postgres only (no redis cache)")
	return pgKeyStore, saStore
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
