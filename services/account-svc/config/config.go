// Package config loads and validates account-svc configuration from environment
// variables at startup. The returned struct is immutable — it is never mutated
// after Load() returns. Pass it through constructors, never store it as a global.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for account-svc.
type Config struct {
	// Service identity
	ServiceName    string
	ServiceVersion string
	Environment    string // local | staging | production

	// HTTP server
	HTTPPort        int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	HandlerTimeout  int // seconds, passed to timeout middleware

	// Logging
	LogLevel  string // debug | info | warn | error
	LogFormat string // json | text | pretty

	// Database
	DBHost     string
	DBPort     int
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string
	DBMaxConns int
	DBMinConns int
	DBLogLevel string // silent | error | warn | info

	// Auth — account-svc holds ONLY the public key for RS256 verification.
	// Token issuance (and the private key) lives exclusively in auth-svc.
	JWTPublicKeyB64  string // base64-encoded PKIX PEM public key
	JWTIssuer        string
	JWTSubjectKeyB64 string // base64-encoded AES-256 key for decrypting Subject claim

	// Rate limiting
	RateLimitRPS   int
	RateLimitBurst int

	// Observability
	OTelEnabled      bool
	OTelLogsEnabled  bool   // false for Jaeger; true only for backends that support OTLP logs
	OTelEndpoint     string
	OTelSamplingRate float64
}

// Load reads config from environment variables and validates required fields.
// If a .env file exists in the working directory it is loaded first; values
// already present in the environment always take precedence.
// Returns an error immediately if any required field is missing.
func Load() (*Config, error) {
	// Auto-load .env for local development. Silently ignored if file is absent.
	_ = loadDotEnv(".env")
	// Resolve environment first so LOG_FORMAT default can depend on it.
	environment := getEnv("ENVIRONMENT", "local")

	// LOG_FORMAT defaults to "pretty" in local dev, "json" everywhere else.
	// An explicit LOG_FORMAT env var always wins regardless of environment.
	logFormatDefault := "json"
	if environment == "local" {
		logFormatDefault = "json"
	}

	cfg := &Config{
		ServiceName:      getEnv("SERVICE_NAME", "account-svc"),
		ServiceVersion:   getEnv("SERVICE_VERSION", "dev"),
		Environment:      environment,
		HTTPPort:         getEnvInt("HTTP_PORT", 8080),
		ReadTimeout:      getEnvDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:     getEnvDuration("WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:      getEnvDuration("IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:  getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		HandlerTimeout:   getEnvInt("HANDLER_TIMEOUT_SECS", 25),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		LogFormat:        getEnv("LOG_FORMAT", logFormatDefault),
		DBHost:           getEnv("DB_HOST", ""),
		DBPort:           getEnvInt("DB_PORT", 5432),
		DBName:           getEnv("DB_NAME", ""),
		DBUser:           getEnv("DB_USER", ""),
		DBPassword:       getEnv("DB_PASSWORD", ""),
		DBSSLMode:        getEnv("DB_SSLMODE", "disable"),
		DBMaxConns:       getEnvInt("DB_MAX_CONNS", 25),
		DBMinConns:       getEnvInt("DB_MIN_CONNS", 5),
		DBLogLevel:       getEnv("DB_LOG_LEVEL", "silent"),
		JWTPublicKeyB64:  getEnv("JWT_PUBLIC_KEY_B64", ""),
		JWTIssuer:        getEnv("JWT_ISSUER", "banking-platform"),
		JWTSubjectKeyB64: getEnv("JWT_SUBJECT_ENCRYPTION_KEY", ""),
		RateLimitRPS:     getEnvInt("RATE_LIMIT_RPS", 1000),
		RateLimitBurst:   getEnvInt("RATE_LIMIT_BURST", 2000),
		OTelEnabled:      getEnvBool("OTEL_ENABLED", false),
		OTelLogsEnabled:  getEnvBool("OTEL_LOGS_ENABLED", false),
		OTelEndpoint:     getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTelSamplingRate: getEnvFloat("OTEL_SAMPLING_RATE", 1.0),
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

// loadDotEnv parses KEY=VALUE pairs from filename and calls os.Setenv for
// each key that is not already set in the process environment.
// Lines beginning with # and blank lines are ignored.
// Surrounding single or double quotes on values are stripped.
// No external dependency — pure stdlib.
func loadDotEnv(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err // caller silently ignores missing file
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
		// Strip inline comment (anything after unquoted #)
		if idx := strings.Index(val, " #"); idx >= 0 {
			val = strings.TrimSpace(val[:idx])
		}
		// Strip surrounding quotes
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
