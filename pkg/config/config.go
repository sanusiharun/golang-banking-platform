// Package config provides an environment-variable-based configuration loader
// with validation. Services embed this base config and extend it with their
// own fields.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Base holds common configuration shared by all services.
type Base struct {
	// Server
	HTTPPort    int           `env:"HTTP_PORT"    default:"8080"`
	MetricsPort int           `env:"METRICS_PORT" default:"9090"`
	ReadTimeout  time.Duration `env:"READ_TIMEOUT"  default:"30s"`
	WriteTimeout time.Duration `env:"WRITE_TIMEOUT" default:"30s"`
	IdleTimeout  time.Duration `env:"IDLE_TIMEOUT"  default:"120s"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" default:"30s"`

	// Logging
	LogLevel  string `env:"LOG_LEVEL"  default:"info"`
	LogFormat string `env:"LOG_FORMAT" default:"json"` // json | text | pretty

	// Postgres
	DBHost     string `env:"DB_HOST"     default:"localhost"`
	DBPort     int    `env:"DB_PORT"     default:"5432"`
	DBName     string `env:"DB_NAME"     required:"true"`
	DBUser     string `env:"DB_USER"     required:"true"`
	DBPassword string `env:"DB_PASSWORD" required:"true"`
	DBSSLMode  string `env:"DB_SSL_MODE" default:"disable"`
	DBMaxConns int    `env:"DB_MAX_CONNS" default:"25"`
	DBMinConns int    `env:"DB_MIN_CONNS" default:"5"`

	// OTel
	OTelEnabled      bool   `env:"OTEL_ENABLED"       default:"false"`
	OTelEndpoint     string `env:"OTEL_ENDPOINT"      default:"localhost:4317"`
	OTelServiceName  string `env:"OTEL_SERVICE_NAME"  default:"banking-service"`
	OTelSamplingRate float64 `env:"OTEL_SAMPLING_RATE" default:"1.0"`

	// JWT
	JWTSecret string `env:"JWT_SECRET" required:"false"`
	JWTIssuer string `env:"JWT_ISSUER" default:"banking-platform"`

	// Environment
	Environment string `env:"ENVIRONMENT" default:"development"`
}

// DSN returns the PostgreSQL connection string.
func (b *Base) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s pool_max_conns=%d pool_min_conns=%d",
		b.DBHost, b.DBPort, b.DBName, b.DBUser, b.DBPassword, b.DBSSLMode, b.DBMaxConns, b.DBMinConns,
	)
}

// IsDevelopment reports whether the environment is development.
func (b *Base) IsDevelopment() bool {
	return strings.EqualFold(b.Environment, "development") || strings.EqualFold(b.Environment, "dev")
}

// IsProduction reports whether the environment is production.
func (b *Base) IsProduction() bool {
	return strings.EqualFold(b.Environment, "production") || strings.EqualFold(b.Environment, "prod")
}

// LoadBase reads the Base config from environment variables.
func LoadBase() (*Base, error) {
	cfg := &Base{}

	cfg.HTTPPort = envInt("HTTP_PORT", 8080)
	cfg.MetricsPort = envInt("METRICS_PORT", 9090)
	cfg.ReadTimeout = envDuration("READ_TIMEOUT", 30*time.Second)
	cfg.WriteTimeout = envDuration("WRITE_TIMEOUT", 30*time.Second)
	cfg.IdleTimeout = envDuration("IDLE_TIMEOUT", 120*time.Second)
	cfg.ShutdownTimeout = envDuration("SHUTDOWN_TIMEOUT", 30*time.Second)

	cfg.LogLevel = envString("LOG_LEVEL", "info")
	cfg.LogFormat = envString("LOG_FORMAT", "json")

	cfg.DBHost = envString("DB_HOST", "localhost")
	cfg.DBPort = envInt("DB_PORT", 5432)
	cfg.DBSSLMode = envString("DB_SSL_MODE", "disable")
	cfg.DBMaxConns = envInt("DB_MAX_CONNS", 25)
	cfg.DBMinConns = envInt("DB_MIN_CONNS", 5)

	var missing []string

	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.DBName = v
	} else {
		missing = append(missing, "DB_NAME")
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.DBUser = v
	} else {
		missing = append(missing, "DB_USER")
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.DBPassword = v
	} else {
		missing = append(missing, "DB_PASSWORD")
	}

	cfg.OTelEnabled = envBool("OTEL_ENABLED", false)
	cfg.OTelEndpoint = envString("OTEL_ENDPOINT", "localhost:4317")
	cfg.OTelServiceName = envString("OTEL_SERVICE_NAME", "banking-service")
	cfg.OTelSamplingRate = envFloat64("OTEL_SAMPLING_RATE", 1.0)

	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	cfg.JWTIssuer = envString("JWT_ISSUER", "banking-platform")

	cfg.Environment = envString("ENVIRONMENT", "development")

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

// Helpers

func envString(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func envBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultVal
	}
	return b
}

func envDuration(key string, defaultVal time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultVal
	}
	return d
}

func envFloat64(key string, defaultVal float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultVal
	}
	return f
}
