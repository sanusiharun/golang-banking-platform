// Package middleware provides HTTP middleware components for the banking platform.
package middleware

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"

	"github.com/sanusi/banking/pkg/httpx"
)

const (
	// HeaderRequestID is the HTTP header name for correlation IDs.
	HeaderRequestID = "X-Request-ID"
)

// RequestID middleware injects a unique request ID into each request's context
// and response headers. If the incoming request already carries X-Request-ID,
// that value is propagated; otherwise a new UUID v4 is generated.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(HeaderRequestID)
		if requestID == "" {
			requestID = newUUID()
		}

		ctx := context.WithValue(r.Context(), httpx.RequestIDKey, requestID)
		w.Header().Set(HeaderRequestID, requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// newUUID generates a cryptographically random UUID v4.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// RequestIDFromContext retrieves the request ID injected by the RequestID middleware.
// Returns an empty string if none is present.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(httpx.RequestIDKey).(string)
	return id
}
