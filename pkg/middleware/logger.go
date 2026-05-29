package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// skipLogPaths contains paths that are polled frequently and would flood logs
// with noise. Prometheus scrapes /metrics every 10s; health checks hit /healthz.
var skipLogPaths = map[string]struct{}{
	"/metrics": {},
	"/healthz": {},
	"/readyz":  {},
	"/livez":   {},
}

// RequestLogger logs each incoming request using the global slog default logger.
//
// Design notes:
//   - /metrics, /healthz, /readyz, /livez are silently skipped — they are
//     scraped every few seconds and would drown out real application traffic.
//   - request_id is NOT added here — contextExtractorHandler injects it automatically
//     from context on every slog call, adding it here would produce duplicates.
//   - trace_id / span_id are captured explicitly using trace.SpanFromContext BEFORE
//     the tracing middleware ends the span on return. When the outer tracing span is
//     still valid we store the IDs; after the span ends IsRecording() is false but
//     SpanContext().IsValid() still returns the final IDs, so we always get them.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip noisy infrastructure endpoints entirely.
		if _, skip := skipLogPaths[r.URL.Path]; skip {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		ctx := r.Context()
		ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(ww, r)

		// Build base log attrs.
		attrs := []any{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote_addr", r.RemoteAddr),
			slog.Int("status", ww.status),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		}

		// Explicitly attach trace/span IDs so they always appear on the request
		// log line regardless of whether the tracing span is still recording.
		// (The outer tracing middleware may have already called span.End() by
		// the time this logger fires, making IsRecording() == false, which
		// causes the automatic traceContextHandler to skip them.)
		if sc := trace.SpanFromContext(ctx).SpanContext(); sc.IsValid() {
			attrs = append(attrs,
				slog.String("trace_id", sc.TraceID().String()),
				slog.String("span_id", sc.SpanID().String()),
			)
		}

		slog.InfoContext(ctx, "request", attrs...)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}
