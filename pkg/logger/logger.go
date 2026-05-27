package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"go.opentelemetry.io/contrib/bridges/otelslog"
)

// Setup configures slog.SetDefault from cfg.
//
// Call once — at the very start of main(), before any other initialisation.
// All slog.Info / slog.Error / slog.Warn / slog.Debug calls across every
// package in the process will automatically pick up this handler.
//
// Handler chain (outermost → innermost → base encoder):
//
//	traceContextHandler   — injects trace_id + span_id from active OTel span
//	contextExtractorHandler — injects request_id, user_id, … from context
//	base handler          — JSON | text | pretty
//
// Static service fields (service, version, env) are attached last via
// slog.With so they appear consistently on every record.
func Setup(cfg Config) {
	level := parseLevel(cfg.Level)
	addSource := cfg.AddSource || level == slog.LevelDebug

	dest := openOutput(cfg.FilePath)

	opts := &slog.HandlerOptions{
		Level:       level,
		AddSource:   addSource,
		ReplaceAttr: trimSourcePath,
	}

	// ── 1. Base encoding handler ──────────────────────────────────────────────
	var base slog.Handler
	switch cfg.Format {
	case FormatPretty:
		base = newPrettyHandler(dest, level)
	case FormatText:
		base = slog.NewTextHandler(dest, opts)
	default: // FormatJSON — safe production default
		base = slog.NewJSONHandler(dest, opts)
	}

	// ── 2. Context enrichment layers ──────────────────────────────────────────
	// Wrap from innermost out — the first wrapper's Handle() runs last,
	// meaning it adds its fields just before the base handler encodes them.
	// Field order in the final record:
	//   [OTel: trace_id, span_id] → [extractors: request_id, user_id, …] → base fields
	h := base

	if len(cfg.Extractors) > 0 {
		h = &contextExtractorHandler{Handler: h, extractors: cfg.Extractors}
	}
	if cfg.OTelTraceContext {
		h = &traceContextHandler{Handler: h}
	}

	// ── 3. Static service identity fields ─────────────────────────────────────
	// slog.With returns a new logger that prepends these attrs to every record.
	// They are applied after the enrichment chain so they always appear last
	// and are never accidentally overwritten by context-extracted values.
	logger := slog.New(h)
	if attrs := serviceAttrs(cfg); len(attrs) > 0 {
		logger = logger.With(attrs...)
	}

	slog.SetDefault(logger)
}

// AttachOTelBridge wires the OpenTelemetry log bridge into the already-configured
// global slog logger. Call this AFTER observability.Bootstrap() returns so that
// the global OTel log provider is set before the bridge tries to use it.
//
// After this call every slog.InfoContext / slog.ErrorContext / etc. will:
//  1. Write to the existing console handler (pretty / JSON / text)
//  2. Also send the record through the OTel log pipeline via OTLP gRPC.
func AttachOTelBridge(serviceName string) {
	current := slog.Default().Handler()
	bridge := otelslog.NewHandler(serviceName)
	slog.SetDefault(slog.New(NewMultiHandler(current, bridge)))
}

// ── helpers ───────────────────────────────────────────────────────────────────

func serviceAttrs(cfg Config) []any {
	var attrs []any
	if cfg.ServiceName != "" {
		attrs = append(attrs, slog.String("service", cfg.ServiceName))
	}
	if cfg.Version != "" {
		attrs = append(attrs, slog.String("version", cfg.Version))
	}
	if cfg.Environment != "" {
		attrs = append(attrs, slog.String("env", cfg.Environment))
	}
	return attrs
}

// openOutput returns the io.Writer for log output.
// Opens FilePath if set, otherwise returns os.Stdout.
// A file-open error is printed to stderr and falls back to stdout — the
// process must not die because of a logging misconfiguration.
func openOutput(path string) io.Writer {
	if path == "" {
		return os.Stdout
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"logger: cannot open %q (%v) — falling back to stdout\n", path, err)
		return os.Stdout
	}
	return f
}

// trimSourcePath replaces the full absolute file path with basename:line.
//
//	"/home/runner/go/pkg/middleware/logger.go:42" → "logger.go:42"
func trimSourcePath(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.SourceKey {
		if src, ok := a.Value.Any().(*slog.Source); ok && src != nil {
			a.Value = slog.StringValue(
				fmt.Sprintf("%s:%d", filepath.Base(src.File), src.Line),
			)
		}
	}
	return a
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
