package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/sanusi/banking/pkg/httpx"
)

// HeaderServiceSecret carries the shared secret for internal service-to-service calls.
const HeaderServiceSecret = "X-Service-Secret"

// RequireServiceSecret rejects requests whose X-Service-Secret header does not match
// secret using a constant-time comparison. Intended for internal endpoints (e.g.
// auth-svc's /auth/apikey/introspect) that Docker network isolation alone doesn't
// protect against a caller on the same network.
//
// If secret is empty, the middleware is a no-op — ponytail: dev/local convenience,
// operators must set the secret env var before exposing the service beyond banking-net.
func RequireServiceSecret(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if secret == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get(HeaderServiceSecret)
			if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
				httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing service secret"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
