package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

	defaultIdempotencyTTL      = 24 * time.Hour
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
			if isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			// ── Skip JWT (human) callers ──────────────────────────────────────
			claims, isServiceCaller := serviceAccountClaims(r.Context())
			if !isServiceCaller {
				next.ServeHTTP(w, r)
				return
			}

			// ── Require Idempotency-Key header ────────────────────────────────
			idempKey, keyErr := validateIdempotencyKeyHeader(r)
			if keyErr != nil {
				httpx.WriteHTTPError(w, r, keyErr)
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
			acquired := acquire(r, cfg.Store, scope, idempKey, meta)
			if acquired.respondOrDegrade(w, r, next) {
				return
			}

			// ── Replay existing completed/failed response ──────────────────────
			if acquired.existing != nil {
				replayExisting(w, r, idempKey, scope, acquired.existing, expiresAt)
				return
			}

			// ── Execute handler and persist the result ──────────────────────────
			executeAndPersist(w, r, next, scope, cfg, expiresAt)
		})
	}
}

// isSafeMethod reports whether method never needs idempotency protection.
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// replayExisting writes a previously completed/failed response back to the
// client without re-executing the handler, recording the outcome on the span and log.
func replayExisting(w http.ResponseWriter, r *http.Request, idempKey, scope string, existing *idempotency.Record, expiresAt time.Time) {
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
}

// executeAndPersist runs the handler and stores its response for future replay.
func executeAndPersist(w http.ResponseWriter, r *http.Request, next http.Handler, scope string, cfg IdempotencyConfig, expiresAt time.Time) {
	rec := captureAndExecute(w, r, next, scope, cfg, expiresAt)
	if rec == nil {
		return // large body — response already written, no storage
	}
	if err := cfg.Store.Complete(r.Context(), scope, rec); err != nil {
		slog.ErrorContext(r.Context(), "idempotency: store complete failed",
			slog.String("error", err.Error()),
		)
		// Non-fatal — response already sent to client.
	}
}

// validateIdempotencyKeyHeader extracts and validates the Idempotency-Key header,
// returning an HTTPError describing the problem when the key is missing or too long.
func validateIdempotencyKeyHeader(r *http.Request) (string, *httpx.HTTPError) {
	idempKey := strings.TrimSpace(r.Header.Get(headerIdempotencyKey))
	if idempKey == "" {
		return "", &httpx.HTTPError{
			StatusCode: http.StatusBadRequest,
			Code:       "MISSING_IDEMPOTENCY_KEY",
			Message:    "Idempotency-Key header is required for this operation",
		}
	}
	if len(idempKey) > 255 {
		return "", &httpx.HTTPError{
			StatusCode: http.StatusBadRequest,
			Code:       "INVALID_IDEMPOTENCY_KEY",
			Message:    "Idempotency-Key must not exceed 255 characters",
		}
	}
	return idempKey, nil
}

// serviceAccountClaims returns the request's claims and whether the caller is
// a service account (the only caller type idempotency protection applies to).
func serviceAccountClaims(ctx context.Context) (*Claims, bool) {
	claims, ok := ClaimsFromContext(ctx)
	if !ok || !IsServiceAccount(claims) {
		return claims, false
	}
	return claims, true
}

// acquireResult is the outcome of attempting to acquire the idempotency lock
// for one request: exactly one of httpErr, degrade, or existing (possibly nil,
// meaning "first request, proceed to execute the handler") applies.
type acquireResult struct {
	httpErr  *httpx.HTTPError    // set on IN_FLIGHT conflict — write and return
	degrade  bool                // set on a store error — proceed without idempotency protection
	existing *idempotency.Record // set (non-nil) when replaying a completed/failed prior response
}

// respondOrDegrade writes the appropriate response for a terminal acquire
// outcome (IN_FLIGHT conflict or store-error degrade) and reports whether the
// caller has already been fully handled and should stop.
func (a acquireResult) respondOrDegrade(w http.ResponseWriter, r *http.Request, next http.Handler) bool {
	if a.httpErr != nil {
		httpx.WriteHTTPError(w, r, a.httpErr)
		return true
	}
	if a.degrade {
		next.ServeHTTP(w, r)
		return true
	}
	return false
}

// acquire calls the idempotency store once and classifies the result into
// exactly one of: in-flight conflict, degrade-on-error, or proceed (with or
// without an existing record to replay).
func acquire(r *http.Request, store idempotency.Store, scope, idempKey string, meta idempotency.Meta) acquireResult {
	existing, err := store.Acquire(r.Context(), scope, meta)
	if errors.Is(err, idempotency.ErrInFlight) {
		slog.InfoContext(r.Context(), "idempotency: request in flight",
			slog.String("scope_prefix", scope[:8]),
			slog.String("idempotency_key", idempKey),
		)
		return acquireResult{httpErr: &httpx.HTTPError{
			StatusCode: http.StatusConflict,
			Code:       "IDEMPOTENCY_IN_FLIGHT",
			Message:    "a request with this idempotency key is already being processed",
		}}
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "idempotency: store acquire failed",
			slog.String("error", err.Error()),
		)
		return acquireResult{degrade: true}
	}
	return acquireResult{existing: existing}
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
	_, _ = w.Write(rec.Body) //nolint:errcheck // headers already sent; nothing to do if the client disconnected mid-write
}

// writeRecorderToWriter flushes the captured response to the real ResponseWriter.
func writeRecorderToWriter(w http.ResponseWriter, rec *responseRecorder) {
	for k, v := range rec.headers {
		for _, vv := range v {
			w.Header().Set(k, vv)
		}
	}
	w.WriteHeader(rec.statusCode)
	_, _ = w.Write(rec.body.Bytes()) //nolint:errcheck // headers already sent; nothing to do if the client disconnected mid-write
}

// scopeKey computes the idempotency scope as SHA-256(callerID|method|path|idempotencyKey).
// Including callerID prevents cross-caller key collisions.
func scopeKey(callerID, method, path, idempotencyKey string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s", callerID, method, path, idempotencyKey) //nolint:errcheck // hash.Hash.Write never returns an error, per the io.Writer contract in the standard library's hash package
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
