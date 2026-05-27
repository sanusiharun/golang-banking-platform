package middleware

import (
	"context"
	"crypto/rsa"
	"log/slog"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type claimsKey struct{}

// Claims holds the parsed JWT payload used for RBAC decisions.
// This struct is populated by auth-svc (signing) and read by every other
// service (verification). The shape must stay in sync across both sides.
type Claims struct {
	UserID   string   `json:"sub"`
	TenantID string   `json:"tenant_id"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

// JWTConfig holds JWT validation parameters for RS256.
// PublicKey is the RSA public key issued by auth-svc.
// All services receive only the public key — the private key never leaves auth-svc.
type JWTConfig struct {
	PublicKey *rsa.PublicKey
	Issuer    string
}

// Authenticate validates a Bearer token using RS256 and injects Claims into context.
// The token must have been signed by auth-svc using its RSA private key.
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
