package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/sanusi/banking/pkg/httpx"
	"github.com/sanusi/banking/pkg/idempotency"
)

const (
	headerIdempotencyKey     = "Idempotency-Key"
	headerIdempotencyReplay  = "Idempotency-Replay"
	headerIdempotencyExpires = "Idempotency-Key-Expires"

	defaultIdempotencyTTL     = 24 * time.Hour
	defaultMaxResponseBodySize = 1 << 20 // 1 MB
)

// IdempotencyConfig configures the idempotency middleware.
type IdempotencyConfig struct {
	Store           idempotency.Store
	TTL             time.Duration // default 24h
	MaxResponseSize int64         // default 1MB; responses larger than this are not stored
}

// Idempotency returns a middleware that enforces idempotency for API key callers.
//
// JWT callers (human users) are passed through immediately — idempotency is only
// required for external channels and service accounts authenticating via API key.
//
// For API key callers on mutating methods (POST, PUT, PATCH):
//   - Missing Idempotency-Key header → 400 Bad Request
//   - First request → execute handler, store response
//   - Duplicate request (completed/failed) → replay stored response
//   - Concurrent duplicate (in-flight) → 409 Conflict
//
// Safe methods (GET, DELETE, HEAD, OPTIONS) are always passed through.
func Idempotency(cfg IdempotencyConfig) func(http.Handler) http.Handler {
	if cfg.TTL <= 0 {
		cfg.TTL = defaultIdempotencyTTL
	}
	if cfg.MaxResponseSize <= 0 {
		cfg.MaxResponseSize = defaultMaxResponseBodySize
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// ── Skip safe methods unconditionally ─────────────────────────────
			switch r.Method {
			case http.MethodGet, http.MethodDelete, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}

			// ── Skip JWT (human) callers ──────────────────────────────────────
			claims, ok := ClaimsFromContext(r.Context())
			if !ok || !IsServiceAccount(claims) {
				next.ServeHTTP(w, r)
				return
			}

			// ── Require Idempotency-Key header ────────────────────────────────
			idempKey := strings.TrimSpace(r.Header.Get(headerIdempotencyKey))
			if idempKey == "" {
				httpx.WriteHTTPError(w, r, &httpx.HTTPError{
					StatusCode: http.StatusBadRequest,
					Code:       "MISSING_IDEMPOTENCY_KEY",
					Message:    "Idempotency-Key header is required for this operation",
				})
				return
			}
			if len(idempKey) > 255 {
				httpx.WriteHTTPError(w, r, &httpx.HTTPError{
					StatusCode: http.StatusBadRequest,
					Code:       "INVALID_IDEMPOTENCY_KEY",
					Message:    "Idempotency-Key must not exceed 255 characters",
				})
				return
			}

			// ── Compute scope key ─────────────────────────────────────────────
			scope := scopeKey(claims.UserID, r.Method, r.URL.Path, idempKey)
			expiresAt := time.Now().Add(cfg.TTL)

			meta := idempotency.Meta{
				IdempotencyKey: idempKey,
				CallerID:       claims.UserID,
				Method:         r.Method,
				Path:           r.URL.Path,
				ExpiresAt:      expiresAt,
			}

			// ── Acquire ───────────────────────────────────────────────────────
			existing, err := cfg.Store.Acquire(r.Context(), scope, meta)
			if err == idempotency.ErrInFlight {
				slog.InfoContext(r.Context(), "idempotency: request in flight",
					slog.String("scope_prefix", scope[:8]),
					slog.String("idempotency_key", idempKey),
				)
				httpx.WriteHTTPError(w, r, &httpx.HTTPError{
					StatusCode: http.StatusConflict,
					Code:       "IDEMPOTENCY_IN_FLIGHT",
					Message:    "a request with this idempotency key is already being processed",
				})
				return
			}
			if err != nil {
				slog.ErrorContext(r.Context(), "idempotency: store acquire failed",
					slog.String("error", err.Error()),
				)
				// Degrade gracefully — proceed without idempotency protection.
				next.ServeHTTP(w, r)
				return
			}

			// ── Replay existing completed/failed response ──────────────────────
			if existing != nil {
				span := trace.SpanFromContext(r.Context())
				span.SetAttributes(
					attribute.String("idempotency.outcome", "replayed"),
					attribute.String("idempotency.key_prefix", idempKey[:min(8, len(idempKey))]),
				)
				slog.InfoContext(r.Context(), "idempotency: replaying response",
					slog.String("scope_prefix", scope[:8]),
					slog.Int("status_code", existing.StatusCode),
				)
				replayResponse(w, existing, expiresAt)
				return
			}

			// ── Execute handler ────────────────────────────────────────────────
			rec := captureAndExecute(w, r, next, scope, cfg, expiresAt)
			if rec == nil {
				return // large body — response already written, no storage
			}

			// ── Persist completed record ───────────────────────────────────────
			if err := cfg.Store.Complete(r.Context(), scope, rec); err != nil {
				slog.ErrorContext(r.Context(), "idempotency: store complete failed",
					slog.String("error", err.Error()),
				)
				// Non-fatal — response already sent to client.
			}
		})
	}
}

// captureAndExecute runs the handler with a response recorder, stores the result,
// and writes it to the real ResponseWriter. Returns nil if the response body is
// too large to store (response is written directly in that case).
func captureAndExecute(
	w http.ResponseWriter,
	r *http.Request,
	next http.Handler,
	scopeKey string,
	cfg IdempotencyConfig,
	expiresAt time.Time,
) *idempotency.Record {
	rec := newResponseRecorder(w)
	next.ServeHTTP(rec, r)

	// Body too large — do not store; write directly and return nil.
	if int64(rec.body.Len()) > cfg.MaxResponseSize {
		slog.WarnContext(r.Context(), "idempotency: response too large to store, skipping",
			slog.Int("size", rec.body.Len()),
			slog.Int64("max", cfg.MaxResponseSize),
		)
		writeRecorderToWriter(w, rec)
		return nil
	}

	now := time.Now().Unix()
	status := idempotency.StatusCompleted
	if rec.statusCode >= 400 {
		status = idempotency.StatusFailed
	}

	headers := make(map[string]string)
	for k, v := range rec.headers {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	result := &idempotency.Record{
		Status:      status,
		StatusCode:  rec.statusCode,
		Headers:     headers,
		Body:        rec.body.Bytes(),
		CreatedAt:   now,
		CompletedAt: &now,
		ScopeKey:    scopeKey,
	}

	// Write idempotency expiry header before flushing.
	w.Header().Set(headerIdempotencyExpires, expiresAt.UTC().Format(time.RFC3339))
	writeRecorderToWriter(w, rec)
	return result
}

// replayResponse writes a stored idempotency record to the ResponseWriter.
func replayResponse(w http.ResponseWriter, rec *idempotency.Record, expiresAt time.Time) {
	for k, v := range rec.Headers {
		w.Header().Set(k, v)
	}
	w.Header().Set(headerIdempotencyReplay, "true")
	w.Header().Set(headerIdempotencyExpires, expiresAt.UTC().Format(time.RFC3339))
	w.WriteHeader(rec.StatusCode)
	_, _ = w.Write(rec.Body)
}

// writeRecorderToWriter flushes the captured response to the real ResponseWriter.
func writeRecorderToWriter(w http.ResponseWriter, rec *responseRecorder) {
	for k, v := range rec.headers {
		for _, vv := range v {
			w.Header().Set(k, vv)
		}
	}
	w.WriteHeader(rec.statusCode)
	_, _ = w.Write(rec.body.Bytes())
}

// scopeKey computes the idempotency scope as SHA-256(callerID|method|path|idempotencyKey).
// Including callerID prevents cross-caller key collisions.
func scopeKey(callerID, method, path, idempotencyKey string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s", callerID, method, path, idempotencyKey)
	return hex.EncodeToString(h.Sum(nil))
}

// ── responseRecorder ──────────────────────────────────────────────────────────

// responseRecorder buffers the handler's response so it can be stored and replayed.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
	headers    http.Header
	written    bool
}

func newResponseRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		headers:        make(http.Header),
	}
}

func (r *responseRecorder) Header() http.Header {
	return r.headers
}

func (r *responseRecorder) WriteHeader(code int) {
	if !r.written {
		r.statusCode = code
		r.written = true
	}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.written {
		r.statusCode = http.StatusOK
		r.written = true
	}
	return r.body.Write(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
