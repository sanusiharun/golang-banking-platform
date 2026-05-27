// Package logger configures the global slog logger once at startup.
// After Setup() is called in main.go every package calls slog.Info/Error
// directly — no logger struct, no constructor injection needed.
package logger

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Format controls how log records are encoded.
type Format string

const (
	// FormatJSON writes structured JSON — the production default.
	// Output: {"time":"…","level":"INFO","msg":"…","trace_id":"…","service":"account-svc"}
	FormatJSON Format = "json"

	// FormatText writes logfmt key=value lines — good for tailing in CI pipelines.
	// Output: time=… level=INFO msg=… trace_id=… service=account-svc
	FormatText Format = "text"

	// FormatPretty writes colour-coded, human-readable lines for local development.
	// Output: 15:04:05.000  INF  request completed   trace_id=abc123  status=200
	//
	// ⚠  Never use in production — ANSI codes break log shippers and parsers.
	FormatPretty Format = "pretty"
)

// ContextExtractor pulls a string value out of a log call's context and maps
// it to a log field. Return "" to suppress the field on that record.
//
// The logger package stays dependency-free: callers provide the extractor
// functions that know about httpx, JWT claims, or any other context value.
//
//	logger.ContextExtractor{
//	    Key:     "request_id",
//	    Extract: pkgmiddleware.RequestIDFromContext,
//	}
type ContextExtractor struct {
	Key     string
	Extract func(context.Context) string
}

// Config is the complete logger configuration.
// All fields have sensible defaults; only ServiceName is typically required.
type Config struct {
	// Level filters records below this threshold.
	// Accepted values: debug | info | warn | error  (default: info)
	Level string

	// Format controls output encoding. Defaults to FormatJSON.
	Format Format

	// ServiceName is emitted on every record as "service".
	ServiceName string

	// Version is emitted on every record as "version".
	Version string

	// Environment is emitted on every record as "env".
	Environment string

	// AddSource includes the caller file:line on every record.
	// Enabled automatically when Level == "debug".
	AddSource bool

	// FilePath writes logs to this file instead of stdout when set.
	FilePath string

	// OTelTraceContext, when true, reads the active OpenTelemetry span from
	// the log call's context and emits trace_id + span_id automatically.
	//
	// This is the core of the slog ↔ OTel integration: any call to
	// slog.InfoContext(ctx, "…") where ctx carries an active span gets its
	// trace and span IDs in the record — zero extra code at each call site.
	//
	// The OTel SDK must be initialised before the first log call.
	OTelTraceContext bool

	// Extractors pull additional fields (e.g. request_id, user_id, tenant_id)
	// from the log context on every record.
	// Nil or empty means no extras.
	Extractors []ContextExtractor
}

// LoadFromEnv builds a Config from environment variables using the given prefix.
//
//	LoadFromEnv("LOG") reads:
//	  LOG_LEVEL, LOG_FORMAT, LOG_ADD_SOURCE, LOG_FILE_PATH,
//	  LOG_OTEL_TRACE_CONTEXT, LOG_SERVICE_NAME, LOG_VERSION, LOG_ENVIRONMENT
//
// Falls back to the bare SERVICE_NAME / SERVICE_VERSION / ENVIRONMENT variables
// when the prefixed variants are absent.
func LoadFromEnv(prefix string) Config {
	env := func(suffix, def string) string {
		if v := os.Getenv(fmt.Sprintf("%s_%s", prefix, suffix)); v != "" {
			return v
		}
		return def
	}
	flag := func(suffix string) bool {
		return strings.EqualFold(env(suffix, "false"), "true")
	}

	return Config{
		Level:            env("LEVEL", "info"),
		Format:           Format(strings.ToLower(env("FORMAT", "json"))),
		ServiceName:      env("SERVICE_NAME", os.Getenv("SERVICE_NAME")),
		Version:          env("VERSION", os.Getenv("SERVICE_VERSION")),
		Environment:      env("ENVIRONMENT", os.Getenv("ENVIRONMENT")),
		AddSource:        flag("ADD_SOURCE"),
		FilePath:         os.Getenv(fmt.Sprintf("%s_FILE_PATH", prefix)),
		OTelTraceContext: flag("OTEL_TRACE_CONTEXT"),
	}
}
