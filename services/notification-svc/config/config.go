// Package config loads and validates notification-svc configuration from
// environment variables at startup. The returned Config is immutable after Load().
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for notification-svc.
type Config struct {
	// Service identity
	ServiceName    string
	ServiceVersion string
	Environment    string

	// HTTP server
	HTTPPort        int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	HandlerTimeout  int

	// Logging
	LogLevel  string
	LogFormat string

	// PostgreSQL
	DBHost     string
	DBPort     int
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string
	DBMaxConns int
	DBMinConns int
	DBLogLevel string

	// NATS JetStream
	NATSUrl      string
	NATSConsumer string

	// JWT
	JWTPublicKeyB64  string
	JWTIssuer        string
	JWTSubjectKeyB64 string

	// Rate limiting
	RateLimitRPS   int
	RateLimitBurst int

	// Observability
	OTelEnabled      bool
	OTelLogsEnabled  bool
	OTelEndpoint     string
	OTelSamplingRate float64

	// Worker
	WorkerCount     int
	WorkerBatchSize int
	WorkerPollSecs  int
}

// Load reads config from environment variables (and .env if present).
func Load() (*Config, error) {
	_ = loadDotEnv(".env") //nolint:errcheck // .env is optional in production; missing file is not an error

	cfg := &Config{
		ServiceName:      getEnv("SERVICE_NAME", "notification-svc"),
		ServiceVersion:   getEnv("SERVICE_VERSION", "dev"),
		Environment:      getEnv("ENVIRONMENT", "local"),
		HTTPPort:         getEnvInt("HTTP_PORT", 8084),
		ReadTimeout:      getEnvDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:     getEnvDuration("WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:      getEnvDuration("IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:  getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		HandlerTimeout:   getEnvInt("HANDLER_TIMEOUT_SECS", 25),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		LogFormat:        getEnv("LOG_FORMAT", "json"),
		DBHost:           getEnv("DB_HOST", ""),
		DBPort:           getEnvInt("DB_PORT", 5432),
		DBName:           getEnv("DB_NAME", ""),
		DBUser:           getEnv("DB_USER", ""),
		DBPassword:       getEnv("DB_PASSWORD", ""),
		DBSSLMode:        getEnv("DB_SSLMODE", "disable"),
		DBMaxConns:       getEnvInt("DB_MAX_CONNS", 25),
		DBMinConns:       getEnvInt("DB_MIN_CONNS", 5),
		DBLogLevel:       getEnv("DB_LOG_LEVEL", "silent"),
		NATSUrl:          getEnv("NATS_URL", "nats://localhost:9053"),
		NATSConsumer:     getEnv("NATS_CONSUMER", "notification-svc-consumer"),
		JWTPublicKeyB64:  getEnv("JWT_PUBLIC_KEY_B64", ""),
		JWTIssuer:        getEnv("JWT_ISSUER", "banking-platform"),
		JWTSubjectKeyB64: getEnv("JWT_SUBJECT_ENCRYPTION_KEY", ""),
		RateLimitRPS:     getEnvInt("RATE_LIMIT_RPS", 1000),
		RateLimitBurst:   getEnvInt("RATE_LIMIT_BURST", 2000),
		OTelEnabled:      getEnvBool("OTEL_ENABLED", false),
		OTelLogsEnabled:  getEnvBool("OTEL_LOGS_ENABLED", false),
		OTelEndpoint:     getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTelSamplingRate: getEnvFloat("OTEL_SAMPLING_RATE", 1.0),
		WorkerCount:      getEnvInt("WORKER_COUNT", 5),
		WorkerBatchSize:  getEnvInt("WORKER_BATCH_SIZE", 10),
		WorkerPollSecs:   getEnvInt("WORKER_POLL_SECS", 5),
	}

	return cfg, cfg.validate()
}

// IsDevelopment returns true when running in the local environment.
func (c *Config) IsDevelopment() bool { return c.Environment == "local" }

func (c *Config) validate() error {
	var errs []error
	if c.DBHost == "" {
		errs = append(errs, errors.New("DB_HOST is required"))
	}
	if c.DBName == "" {
		errs = append(errs, errors.New("DB_NAME is required"))
	}
	if c.DBUser == "" {
		errs = append(errs, errors.New("DB_USER is required"))
	}
	if c.DBPassword == "" {
		errs = append(errs, errors.New("DB_PASSWORD is required"))
	}
	if c.JWTPublicKeyB64 == "" {
		errs = append(errs, errors.New("JWT_PUBLIC_KEY_B64 is required"))
	}
	if len(errs) > 0 {
		return fmt.Errorf("config validation failed: %w", errors.Join(errs...))
	}
	return nil
}

// ── env helpers ───────────────────────────────────────────────────────────────

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func loadDotEnv(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if idx := strings.Index(val, " #"); idx >= 0 {
			val = strings.TrimSpace(val[:idx])
		}
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, val) //nolint:errcheck // os.Setenv only fails on empty key, already guarded above
		}
	}
	return nil
}
