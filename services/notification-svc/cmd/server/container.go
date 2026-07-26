package main

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
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
	svcconfig "github.com/sanusi/banking/services/notification-svc/config"
	"github.com/sanusi/banking/services/notification-svc/internal/channel"
	"github.com/sanusi/banking/services/notification-svc/internal/channel/email"
	"github.com/sanusi/banking/services/notification-svc/internal/channel/push"
	"github.com/sanusi/banking/services/notification-svc/internal/channel/sms"
	"github.com/sanusi/banking/services/notification-svc/internal/channel/webhook"
	"github.com/sanusi/banking/services/notification-svc/internal/channel/whatsapp"
	"github.com/sanusi/banking/services/notification-svc/internal/repository"
	"github.com/sanusi/banking/services/notification-svc/internal/services"
	tmpl "github.com/sanusi/banking/services/notification-svc/internal/template"
	"github.com/sanusi/banking/services/notification-svc/internal/transport"
	"github.com/sanusi/banking/services/notification-svc/internal/worker"
)

type container struct {
	server     *http.Server
	otel       *observability.Provider
	consumer   *transport.Consumer
	dispatcher *worker.Dispatcher
	scheduler  *worker.SchedulerWorker
	nc         *nats.Conn
}

// build wires all dependencies for notification-svc.
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

	// ── RSA public key (for JWT verification) ─────────────────────────────────
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

	// ── NATS JetStream ─────────────────────────────────────────────────────────
	nc, err := nats.Connect(cfg.NATSUrl,
		nats.Name("notification-svc"),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	if waitErr := waitForNATS(ctx, nc, cfg.NATSUrl); waitErr != nil {
		return nil, waitErr
	}
	slog.Info("nats connected", slog.String("url", cfg.NATSUrl))

	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("get jetstream context: %w", err)
	}

	// Ensure the NOTIFICATIONS stream exists.
	if streamErr := ensureNotificationsStream(js); streamErr != nil {
		return nil, fmt.Errorf("ensure notifications stream: %w", streamErr)
	}

	// Ensure the AUDIT stream exists (for audit publishing).
	if auditStreamErr := pkgaudit.EnsureStream(js); auditStreamErr != nil {
		return nil, fmt.Errorf("ensure audit stream: %w", auditStreamErr)
	}

	// ── Channel registry ──────────────────────────────────────────────────────
	registry := channel.NewRegistry()
	registry.Register(email.New())
	registry.Register(sms.New())
	registry.Register(push.New())
	registry.Register(whatsapp.New())
	registry.Register(webhook.New())

	// ── Template engine ───────────────────────────────────────────────────────
	engine := tmpl.New()

	// ── Repositories ──────────────────────────────────────────────────────────
	notifRepo := repository.NewNotificationRepository(db)
	tmplRepo := repository.NewTemplateRepository(db)
	schedRepo := repository.NewScheduleRepository(db)

	// ── Services ──────────────────────────────────────────────────────────────
	validate := validator.New()
	notifSvc := services.NewNotificationService(notifRepo)
	tmplSvc := services.NewTemplateService(tmplRepo, engine)
	schedSvc := services.NewSchedulerService(schedRepo)

	// ── Workers ───────────────────────────────────────────────────────────────
	dispatcher := worker.NewDispatcher(
		worker.DispatcherConfig{
			Workers:   cfg.WorkerCount,
			BatchSize: cfg.WorkerBatchSize,
			PollEvery: time.Duration(cfg.WorkerPollSecs) * time.Second,
		},
		notifRepo,
		tmplRepo,
		registry,
		engine,
	)
	schedulerWorker := worker.NewSchedulerWorker(schedRepo, notifRepo)

	// ── NATS consumer ─────────────────────────────────────────────────────────
	consumer, err := transport.NewConsumer(js, cfg.NATSConsumer, notifSvc)
	if err != nil {
		return nil, fmt.Errorf("create nats consumer: %w", err)
	}

	// ── Handlers ──────────────────────────────────────────────────────────────
	notifHandler := transport.NewNotificationHandler(notifSvc, validate)
	tmplHandler := transport.NewTemplateHandler(tmplSvc, validate)
	schedHandler := transport.NewScheduleHandler(schedSvc, validate)

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
		NotificationHandler: notifHandler,
		TemplateHandler:     tmplHandler,
		ScheduleHandler:     schedHandler,
		Health:              health,
		JWTConfig: pkgmiddleware.JWTConfig{
			PublicKey:  publicKey,
			Issuer:     cfg.JWTIssuer,
			SubjectKey: subjectKey,
		},
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
		server:     server,
		otel:       otelProvider,
		consumer:   consumer,
		dispatcher: dispatcher,
		scheduler:  schedulerWorker,
		nc:         nc,
	}, nil
}

// ensureNotificationsStream creates the NOTIFICATIONS JetStream stream if absent.
func ensureNotificationsStream(js nats.JetStreamContext) error {
	_, err := js.StreamInfo("NOTIFICATIONS")
	if err == nil {
		return nil
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("check stream: %w", err)
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "NOTIFICATIONS",
		Subjects: []string{"notifications.>"},
		Storage:  nats.FileStorage,
		Replicas: 1,
	})
	return err
}

// ── key helpers (identical to other services) ─────────────────────────────────

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

// waitForNATS blocks until the NATS connection is established or ctx is cancelled.
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
