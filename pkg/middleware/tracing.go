package middleware

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Tracing is an OTel trace propagation middleware. It extracts incoming trace
// context from request headers (W3C TraceContext / B3), starts a server span,
// and injects the span into the request context for downstream handlers.
func Tracing(serviceName string) func(http.Handler) http.Handler {
	tracer := otel.Tracer(serviceName)
	propagator := otel.GetTextMapPropagator()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract incoming trace context from headers.
			ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			spanName := r.Method + " " + r.URL.Path
			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.method", r.Method),
					attribute.String("http.url", r.URL.String()),
					attribute.String("http.scheme", scheme(r)),
					attribute.String("http.host", r.Host),
					attribute.String("http.user_agent", r.UserAgent()),
				),
			)
			defer span.End()

			// Inject updated trace context into response headers.
			propagator.Inject(ctx, propagation.HeaderCarrier(w.Header()))

			wrapped := &tracingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			span.SetAttributes(attribute.Int("http.status_code", wrapped.statusCode))
		})
	}
}

type tracingResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (tw *tracingResponseWriter) WriteHeader(code int) {
	if !tw.wroteHeader {
		tw.statusCode = code
		tw.wroteHeader = true
		tw.ResponseWriter.WriteHeader(code)
	}
}

func (tw *tracingResponseWriter) Write(b []byte) (int, error) {
	if !tw.wroteHeader {
		tw.WriteHeader(http.StatusOK)
	}
	return tw.ResponseWriter.Write(b)
}

// scheme returns "https" or "http" based on TLS state and forwarded headers.
func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if fwdProto := r.Header.Get("X-Forwarded-Proto"); fwdProto != "" {
		return fwdProto
	}
	return "http"
}
