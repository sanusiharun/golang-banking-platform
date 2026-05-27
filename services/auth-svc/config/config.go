// Package config loads and validates auth-svc configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for auth-svc.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string

	HTTPPort        int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration

	LogLevel  string
	LogFormat string

	// Database — auth-svc owns its own DB (database-per-service pattern).
	DBHost     string
	DBPort     int
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string
	DBMaxConns int
	DBMinConns int
	DBLogLevel string

	// JWT — auth-svc holds the PRIVATE key for signing.
	// Other services receive only the PUBLIC key.
	JWTPrivateKeyB64 string        // base64-encoded PKCS#8 PEM private key
	JWTIssuer        string
	JWTTokenTTL      time.Duration

	// Observability
	OTelEnabled      bool
	OTelLogsEnabled  bool
	OTelEndpoint     string
	OTelSamplingRate float64
}

func Load() (*Config, error) {
	_ = loadDotEnv(".env")
	environment := getEnv("ENVIRONMENT", "local")

	cfg := &Config{
		ServiceName:      getEnv("SERVICE_NAME", "auth-svc"),
		ServiceVersion:   getEnv("SERVICE_VERSION", "dev"),
		Environment:      environment,
		HTTPPort:         getEnvInt("HTTP_PORT", 8082),
		ReadTimeout:      getEnvDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:     getEnvDuration("WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:      getEnvDuration("IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:  getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		LogFormat:        getEnv("LOG_FORMAT", "json"),
		DBHost:           getEnv("DB_HOST", ""),
		DBPort:           getEnvInt("DB_PORT", 5432),
		DBName:           getEnv("DB_NAME", "authdb"),
		DBUser:           getEnv("DB_USER", ""),
		DBPassword:       getEnv("DB_PASSWORD", ""),
		DBSSLMode:        getEnv("DB_SSLMODE", "disable"),
		DBMaxConns:       getEnvInt("DB_MAX_CONNS", 10),
		DBMinConns:       getEnvInt("DB_MIN_CONNS", 2),
		DBLogLevel:       getEnv("DB_LOG_LEVEL", "silent"),
		JWTPrivateKeyB64: getEnv("JWT_PRIVATE_KEY_B64", ""),
		JWTIssuer:        getEnv("JWT_ISSUER", "banking-platform"),
		JWTTokenTTL:      getEnvDuration("JWT_TOKEN_TTL", 24*time.Hour),
		OTelEnabled:      getEnvBool("OTEL_ENABLED", false),
		OTelLogsEnabled:  getEnvBool("OTEL_LOGS_ENABLED", false),
		OTelEndpoint:     getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTelSamplingRate: getEnvFloat("OTEL_SAMPLING_RATE", 1.0),
	}

	return cfg, cfg.validate()
}

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
	if c.JWTPrivateKeyB64 == "" {
		errs = append(errs, errors.New("JWT_PRIVATE_KEY_B64 is required"))
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
			_ = os.Setenv(key, val)
		}
	}
	return nil
}
