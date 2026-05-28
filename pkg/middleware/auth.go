package middleware

import (
	"context"
	"crypto/rsa"
	"log/slog"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	pkgcrypto "github.com/sanusi/banking/pkg/crypto"
)

type claimsKey struct{}

// Claims holds the parsed JWT payload used for RBAC decisions.
//
// Layout:
//   - jwt.RegisteredClaims is embedded so all standard fields (sub, iss, exp, iat)
//     appear inline in the JWT JSON — no nesting.
//   - UserID is NOT serialised into the JWT (json:"-"). It is populated by the
//     Authenticate middleware after decrypting RegisteredClaims.Subject.
//     Callers should always read UserID, never Subject directly.
//   - TenantID and Roles travel as custom claims inside the token.
type Claims struct {
	jwt.RegisteredClaims            // Subject = AES-256-GCM encrypted user ID
	UserID   string `json:"-"`      // decrypted user ID — set by middleware, not in JWT
	TenantID string `json:"tenant_id"`
	Roles    []string `json:"roles"`
}

// JWTConfig holds JWT validation parameters for RS256.
type JWTConfig struct {
	PublicKey  *rsa.PublicKey
	Issuer     string
	SubjectKey []byte // AES-256 key (32 bytes) for decrypting Subject claim.
	            	   // If empty, Subject is used as-is (no encryption).
}

// Authenticate validates a Bearer token using RS256 and injects Claims into context.
// If JWTConfig.SubjectKey is set, the Subject claim is decrypted and stored in
// Claims.UserID — all downstream handlers receive the plaintext user ID transparently.
func Authenticate(cfg JWTConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractBearer(r)
			if tokenStr == "" {
				writeAuthError(w, "UNAUTHORIZED", "missing authorization header", http.StatusUnauthorized)
				return
			}

			claims := &Claims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
				// Reject any token not signed with RS256.
				// Prevents algorithm-confusion attacks (alg=none, HS256 downgrade, etc.).
				if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return cfg.PublicKey, nil
			}, jwt.WithIssuer(cfg.Issuer), jwt.WithExpirationRequired())

			if err != nil || !token.Valid {
				slog.WarnContext(r.Context(), "invalid token",
					slog.String("error", err.Error()),
					slog.String("request_id", RequestIDFromContext(r.Context())),
				)
				writeAuthError(w, "UNAUTHORIZED", "invalid or expired token", http.StatusUnauthorized)
				return
			}

			// Decrypt Subject → UserID.
			// If no key is configured, treat Subject as plaintext (backwards compatible).
			if len(cfg.SubjectKey) > 0 {
				userID, err := pkgcrypto.DecryptSubject(cfg.SubjectKey, claims.Subject)
				if err != nil {
					slog.WarnContext(r.Context(), "failed to decrypt token subject",
						slog.String("error", err.Error()),
					)
					writeAuthError(w, "UNAUTHORIZED", "invalid token subject", http.StatusUnauthorized)
					return
				}
				claims.UserID = userID
			} else {
				claims.UserID = claims.Subject
			}

			ctx := context.WithValue(r.Context(), claimsKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole allows the request only if the authenticated user holds at least
// one of the given roles. Must be used after Authenticate.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				writeAuthError(w, "UNAUTHORIZED", "no auth context", http.StatusUnauthorized)
				return
			}
			for _, role := range claims.Roles {
				if _, ok := allowed[role]; ok {
					next.ServeHTTP(w, r)
					return
				}
			}
			slog.WarnContext(r.Context(), "access denied",
				slog.String("user_id", claims.UserID),
				slog.Any("user_roles", claims.Roles),
				slog.Any("required_roles", roles),
			)
			writeAuthError(w, "FORBIDDEN", "insufficient role", http.StatusForbidden)
		})
	}
}

// ClaimsFromContext retrieves claims stored by Authenticate.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey{}).(*Claims)
	return c, ok && c != nil
}

// UserIDFromContext returns the decrypted user ID from the request context.
// The Authenticate middleware decrypts the JWT Subject transparently, so this
// always returns the plaintext user ID — callers never deal with encryption.
// Returns an empty string if no auth context is present.
func UserIDFromContext(ctx context.Context) string {
	if c, ok := ClaimsFromContext(ctx); ok {
		return c.UserID
	}
	return ""
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func writeAuthError(w http.ResponseWriter, code, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"success":false,"error":{"code":"` + code + `","message":"` + msg + `"}}`))
}
