package logger

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"sync"

	"go.opentelemetry.io/otel/trace"
)

// ─── traceContextHandler ──────────────────────────────────────────────────────
//
// Wraps any handler and automatically injects trace_id + span_id from the
// active OpenTelemetry span stored in the log call's context.
//
// When no span is active (or OTel is not initialised) the fields are silently
// omitted — no panics, no zero-value noise.
//
// Why this matters: the tracing middleware (pkg/middleware/tracing.go) starts
// a span and stores it in the request context. Every downstream call to
// slog.InfoContext(ctx, "…") automatically gets the IDs without any extra code.
// In Grafana / Jaeger you can click a log line and jump straight to
// the matching distributed trace.

type traceContextHandler struct{ slog.Handler }

func (h *traceContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		sc := span.SpanContext()
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

func (h *traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}
func (h *traceContextHandler) WithGroup(name string) slog.Handler {
	return &traceContextHandler{Handler: h.Handler.WithGroup(name)}
}

// ─── contextExtractorHandler ──────────────────────────────────────────────────
//
// Enriches every record with fields the caller pulls from the log context.
// The extractor functions are provided at Setup() time so this handler has
// zero knowledge of httpx, JWT claims, or any domain-specific package.
//
// Execution happens after traceContextHandler so OTel IDs always appear first.

type contextExtractorHandler struct {
	slog.Handler
	extractors []ContextExtractor
}

func (h *contextExtractorHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, ex := range h.extractors {
		if v := ex.Extract(ctx); v != "" {
			r.AddAttrs(slog.String(ex.Key, v))
		}
	}
	return h.Handler.Handle(ctx, r)
}

func (h *contextExtractorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextExtractorHandler{
		Handler:    h.Handler.WithAttrs(attrs),
		extractors: h.extractors,
	}
}
func (h *contextExtractorHandler) WithGroup(name string) slog.Handler {
	return &contextExtractorHandler{
		Handler:    h.Handler.WithGroup(name),
		extractors: h.extractors,
	}
}

// ─── MultiHandler ─────────────────────────────────────────────────────────────
//
// Fans a single record out to multiple independent handlers — for example:
//
//	console handler  →  human-readable output locally
//	OTel log bridge  →  OTLP log pipeline (Jaeger / Grafana Loki)
//
// Handlers that are not enabled for a given level are skipped.
// Records are cloned before each dispatch so handlers cannot interfere.
//
// Adding an OTel log bridge later requires no changes to the rest of the code:
//
//	import "go.opentelemetry.io/contrib/bridges/otelslog"
//	bridge := otelslog.NewHandler("account-svc",
//	    otelslog.WithLoggerProvider(logProvider))
//	base = logger.NewMultiHandler(consoleHandler, bridge)

type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a fan-out handler that dispatches to all provided handlers.
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

func (m *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			_ = h.Handle(ctx, r.Clone())
		}
	}
	return nil
}

func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hs := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		hs[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: hs}
}

func (m *MultiHandler) WithGroup(name string) slog.Handler {
	hs := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		hs[i] = h.WithGroup(name)
	}
	return &MultiHandler{handlers: hs}
}

// ─── prettyHandler ────────────────────────────────────────────────────────────
//
// Colour-coded, human-readable output designed for local development.
//
// Example output:
//
//	15:04:05.123  INF  request completed   trace_id=a1b2c3d4  span_id=e5f6a7b8  request_id=abc-123  status=200
//	15:04:05.456  ERR  database error      trace_id=a1b2c3d4  span_id=e5f6a7b8  error=connection refused
//
// ⚠  Never use in production — ANSI escape codes break log parsers and shippers.

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiCyan   = "\033[36m"
)

type prettyHandler struct {
	mu          sync.Mutex
	w           io.Writer
	level       slog.Level
	presetAttrs []slog.Attr // accumulated via WithAttrs
}

func newPrettyHandler(w io.Writer, level slog.Level) *prettyHandler {
	return &prettyHandler{w: w, level: level}
}

func (h *prettyHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= h.level
}

func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	var buf bytes.Buffer

	// ── timestamp — dim/gray ──────────────────────────────────────────────────
	buf.WriteString(ansiDim)
	buf.WriteString(r.Time.Format("15:04:05.000"))
	buf.WriteString(ansiReset)

	// ── level — 3-char, bold, coloured ───────────────────────────────────────
	buf.WriteString("  ")
	buf.WriteString(levelStyle(r.Level))
	buf.WriteString(levelAbbr(r.Level))
	buf.WriteString(ansiReset)

	// ── message — bold white ──────────────────────────────────────────────────
	buf.WriteString("  ")
	buf.WriteString(ansiBold)
	buf.WriteString(r.Message)
	buf.WriteString(ansiReset)

	// ── preset attrs (from slog.With / logger.With) ───────────────────────────
	for _, a := range h.presetAttrs {
		writeKV(&buf, a)
	}

	// ── record-level attrs ────────────────────────────────────────────────────
	r.Attrs(func(a slog.Attr) bool {
		writeKV(&buf, a)
		return true
	})

	buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf.Bytes())
	return err
}

// WithAttrs returns a new handler with the given attrs merged into presetAttrs.
// This is called when the logger is created with slog.With(…) global fields.
func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, len(h.presetAttrs)+len(attrs))
	copy(merged, h.presetAttrs)
	copy(merged[len(h.presetAttrs):], attrs)
	return &prettyHandler{w: h.w, level: h.level, presetAttrs: merged}
}

func (h *prettyHandler) WithGroup(_ string) slog.Handler { return h }

// ── pretty helpers ────────────────────────────────────────────────────────────

func levelStyle(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return ansiRed + ansiBold
	case l >= slog.LevelWarn:
		return ansiYellow + ansiBold
	case l >= slog.LevelInfo:
		return ansiGreen + ansiBold
	default:
		return ansiCyan + ansiBold
	}
}

func levelAbbr(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERR"
	case l >= slog.LevelWarn:
		return "WRN"
	case l >= slog.LevelInfo:
		return "INF"
	default:
		return "DBG"
	}
}

// writeKV writes a single attr as "  key=value" with dimmed key names.
// Groups are flattened to "group.key=value" notation.
func writeKV(buf *bytes.Buffer, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		for _, ga := range a.Value.Group() {
			if a.Key != "" {
				ga.Key = a.Key + "." + ga.Key
			}
			writeKV(buf, ga)
		}
		return
	}
	buf.WriteString("  ")
	buf.WriteString(ansiDim)
	buf.WriteString(a.Key)
	buf.WriteString("=")
	buf.WriteString(ansiReset)
	buf.WriteString(a.Value.String())
}
