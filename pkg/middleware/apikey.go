package middleware

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ── Types ─────────────────────────────────────────────────────────────────────

// ServiceAccountIdentity is the resolved identity of an API key caller.
// It is produced by the auth-svc repository and consumed by AuthenticateAPIKey
// to build a *Claims that downstream middleware (RequireRole, UserIDFromContext)
// can use identically to a JWT-authenticated request.
type ServiceAccountIdentity struct {
	ServiceAccountID string     `json:"service_account_id"`
	TenantID         string     `json:"tenant_id"`
	Roles            []string   `json:"roles"`
	KeyID            string     `json:"key_id"` // api_keys.id — used for logging, not the hash
	ExpiresAt        *time.Time `json:"expires_at,omitempty"` // nil = non-expiring
}

// APIKeyLookup is the minimal interface AuthenticateAPIKey requires.
// Implemented by auth-svc's repository layer (redisAPIKeyStore wrapping postgresAPIKeyStore).
// Using an interface here keeps pkg/middleware free of any auth-svc dependency.
type APIKeyLookup interface {
	FindActiveByHash(ctx context.Context, hash string) (*ServiceAccountIdentity, error)
	UpdateLastUsed(ctx context.Context, keyID string) error
}

// APIKeyConfig configures the API key middleware.
type APIKeyConfig struct {
	Lookup      APIKeyLookup
	Environment string // "local" | "staging" | "production" — used for prefix validation
}

// ── Key format ────────────────────────────────────────────────────────────────
//
//	bp_live_<32 base62 chars>  — production
//	bp_test_<32 base62 chars>  — non-production
//
// The prefix "bp_live_" / "bp_test_" is scannable by secret detection tools.

const (
	keyPrefixProd = "bp_live_"
	keyPrefixTest = "bp_test_"
	keyTotalLen   = len(keyPrefixProd) + 32 // 8 prefix + 32 random = 40 chars
)

// GenerateAPIKey generates a cryptographically random API key for the given environment.
// env should be "live" for production and "test" for all other environments.
// Returns the raw key (shown once) and its SHA-256 hex hash (stored in DB).
func GenerateAPIKey(env string) (rawKey, hash string, err error) {
	prefix := keyPrefixTest
	if env == "live" {
		prefix = keyPrefixProd
	}

	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate api key: %w", err)
	}
	secret := make([]byte, 32)
	for i, v := range b {
		secret[i] = alphabet[int(v)%62]
	}

	rawKey = prefix + string(secret)
	h := sha256.Sum256([]byte(rawKey))
	hash = hex.EncodeToString(h[:])
	return rawKey, hash, nil
}

// HashAPIKey returns the SHA-256 hex hash of a raw API key.
// Used to look up a presented key in the database without storing the raw value.
func HashAPIKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// ── Middleware ────────────────────────────────────────────────────────────────

// AuthenticateAPIKey validates an API key from the request and injects *Claims
// into the context — identical to what the JWT Authenticate middleware produces.
// The Subject field is set to "sa:<service_account_id>" so downstream code can
// distinguish service account callers from human users when needed.
//
// Header priority:
//  1. Authorization: ApiKey <key>
//  2. X-API-Key: <key>  (fallback for partners that cannot set Authorization)
func AuthenticateAPIKey(cfg APIKeyConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawKey := extractAPIKey(r)
			if rawKey == "" {
				writeAuthError(w, "UNAUTHORIZED", "missing api key", http.StatusUnauthorized)
				return
			}

			// Reject test keys in production and vice versa.
			if !validKeyPrefix(rawKey, cfg.Environment) {
				writeAuthError(w, "UNAUTHORIZED", "invalid api key prefix for this environment", http.StatusUnauthorized)
				return
			}

			hash := HashAPIKey(rawKey)
			identity, err := cfg.Lookup.FindActiveByHash(r.Context(), hash)
			if err != nil {
				slog.WarnContext(r.Context(), "api_key auth: lookup failed",
					slog.String("error", err.Error()),
					slog.String("request_id", RequestIDFromContext(r.Context())),
				)
				writeAuthError(w, "UNAUTHORIZED", "invalid or expired api key", http.StatusUnauthorized)
				return
			}

			// Fire-and-forget last_used_at update — does not block the request.
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if err := cfg.Lookup.UpdateLastUsed(ctx, identity.KeyID); err != nil {
					slog.Warn("api_key auth: failed to update last_used_at",
						slog.String("key_id", identity.KeyID),
						slog.String("error", err.Error()),
					)
				}
			}()

			// Build Claims — identical shape to JWT claims so all downstream
			// middleware (RequireRole, UserIDFromContext, audit, idempotency) is unaware
			// of which auth method was used.
			claims := &Claims{
				UserID:   identity.ServiceAccountID,
				TenantID: identity.TenantID,
				Roles:    identity.Roles,
			}
			// Use RegisteredClaims.Subject = "sa:<id>" to mark as service account.
			claims.Subject = "sa:" + identity.ServiceAccountID

			ctx := context.WithValue(r.Context(), claimsKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthenticateAny tries JWT first (no I/O), then API key (requires store lookup).
// Use on endpoints that accept both human users and service accounts.
//
// Decision path:
//   - Header starts with "Bearer " → JWT path (Authenticate)
//   - Header starts with "ApiKey " or X-API-Key present → API key path
//   - Neither → 401
func AuthenticateAny(jwtCfg JWTConfig, apiKeyCfg APIKeyConfig) func(http.Handler) http.Handler {
	jwtMiddleware := Authenticate(jwtCfg)
	apiKeyMiddleware := AuthenticateAPIKey(apiKeyCfg)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")

			if strings.HasPrefix(authHeader, "Bearer ") || strings.HasPrefix(authHeader, "bearer ") {
				jwtMiddleware(next).ServeHTTP(w, r)
				return
			}

			if strings.HasPrefix(strings.ToLower(authHeader), "apikey ") || r.Header.Get("X-API-Key") != "" {
				apiKeyMiddleware(next).ServeHTTP(w, r)
				return
			}

			writeAuthError(w, "UNAUTHORIZED", "missing authorization header", http.StatusUnauthorized)
		})
	}
}

// IsServiceAccount reports whether the claims belong to a service account caller.
// Use this in middleware that needs to distinguish service accounts from human users
// (e.g. idempotency enforcement).
func IsServiceAccount(claims *Claims) bool {
	return strings.HasPrefix(claims.Subject, "sa:")
}

// ── helpers ───────────────────────────────────────────────────────────────────

// extractAPIKey reads the raw API key from:
//  1. Authorization: ApiKey <key>
//  2. X-API-Key: <key>
func extractAPIKey(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if after, ok := strings.CutPrefix(auth, "ApiKey "); ok {
			return strings.TrimSpace(after)
		}
		if after, ok := strings.CutPrefix(auth, "apikey "); ok {
			return strings.TrimSpace(after)
		}
	}
	return strings.TrimSpace(r.Header.Get("X-API-Key"))
}

// validKeyPrefix checks that the key matches the expected prefix for the environment.
// Prevents test keys from being used in production and vice versa.
func validKeyPrefix(key, env string) bool {
	if env == "production" || env == "prod" {
		return strings.HasPrefix(key, keyPrefixProd)
	}
	// All non-production environments accept test keys.
	return strings.HasPrefix(key, keyPrefixTest) || strings.HasPrefix(key, keyPrefixProd)
}
