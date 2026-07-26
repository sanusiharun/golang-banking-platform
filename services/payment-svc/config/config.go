// Package config loads and validates payment-svc configuration from
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

// Config holds all configuration for payment-svc.
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

	// Redis
	RedisAddr           string
	RedisPassword       string
	IdempotencyTTLHours int

	// NATS JetStream
	NATSUrl      string
	NATSConsumer string

	// Account Service
	AccountSvcURL    string
	AccountSvcAPIKey string

	// JWT
	JWTPublicKeyB64  string
	JWTIssuer        string
	JWTSubjectKeyB64 string

	// Rate limiting
	RateLimitRPS   int
	RateLimitBurst int

	// QRIS
	QRISAcquirerGUID     string
	QRISDefaultMCC       string
	QRISCurrency         string
	QRISChargeTTLSeconds int

	// Observability
	OTelEnabled      bool
	OTelLogsEnabled  bool
	OTelEndpoint     string
	OTelSamplingRate float64
}

// Load reads config from environment variables (and .env if present).
func Load() (*Config, error) {
	_ = loadDotEnv(".env") //nolint:errcheck // .env is optional in production; missing file is not an error

	cfg := &Config{
		ServiceName:          getEnv("SERVICE_NAME", "payment-svc"),
		ServiceVersion:       getEnv("SERVICE_VERSION", "dev"),
		Environment:          getEnv("ENVIRONMENT", "local"),
		HTTPPort:             getEnvInt("HTTP_PORT", 8085),
		ReadTimeout:          getEnvDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:         getEnvDuration("WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:          getEnvDuration("IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:      getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		HandlerTimeout:       getEnvInt("HANDLER_TIMEOUT_SECS", 25),
		LogLevel:             getEnv("LOG_LEVEL", "info"),
		LogFormat:            getEnv("LOG_FORMAT", "json"),
		DBHost:               getEnv("DB_HOST", ""),
		DBPort:               getEnvInt("DB_PORT", 5432),
		DBName:               getEnv("DB_NAME", "banking_payments"),
		DBUser:               getEnv("DB_USER", ""),
		DBPassword:           getEnv("DB_PASSWORD", ""),
		DBSSLMode:            getEnv("DB_SSLMODE", "disable"),
		DBMaxConns:           getEnvInt("DB_MAX_CONNS", 25),
		DBMinConns:           getEnvInt("DB_MIN_CONNS", 5),
		DBLogLevel:           getEnv("DB_LOG_LEVEL", "silent"),
		RedisAddr:            getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:        getEnv("REDIS_PASSWORD", ""),
		IdempotencyTTLHours:  getEnvInt("IDEMPOTENCY_TTL_HOURS", 24),
		NATSUrl:              getEnv("NATS_URL", "nats://localhost:9053"),
		NATSConsumer:         getEnv("NATS_CONSUMER", "payment-svc-consumer"),
		AccountSvcURL:        getEnv("ACCOUNT_SVC_URL", "http://localhost:8081"),
		AccountSvcAPIKey:     getEnv("ACCOUNT_SVC_API_KEY", ""),
		JWTPublicKeyB64:      getEnv("JWT_PUBLIC_KEY_B64", ""),
		JWTIssuer:            getEnv("JWT_ISSUER", "banking-platform"),
		JWTSubjectKeyB64:     getEnv("JWT_SUBJECT_ENCRYPTION_KEY", ""),
		RateLimitRPS:         getEnvInt("RATE_LIMIT_RPS", 500),
		RateLimitBurst:       getEnvInt("RATE_LIMIT_BURST", 1000),
		QRISAcquirerGUID:     getEnv("QRIS_ACQUIRER_GUID", "ID.CO.QRIS.WWW"),
		QRISDefaultMCC:       getEnv("QRIS_DEFAULT_MCC", "5411"),
		QRISCurrency:         getEnv("QRIS_CURRENCY", "IDR"),
		QRISChargeTTLSeconds: getEnvInt("QRIS_CHARGE_TTL_SECONDS", 1800),
		OTelEnabled:          getEnvBool("OTEL_ENABLED", false),
		OTelLogsEnabled:      getEnvBool("OTEL_LOGS_ENABLED", false),
		OTelEndpoint:         getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTelSamplingRate:     getEnvFloat("OTEL_SAMPLING_RATE", 1.0),
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
