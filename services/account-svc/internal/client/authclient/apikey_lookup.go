package authclient

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
)

const (
	apiKeyCachePrefix = "apikey:"
	apiKeyCacheTTL    = 5 * time.Minute
)

// cachePayload mirrors the JSON shape written by auth-svc's redisAPIKeyStore.
// Must stay in sync with services/auth-svc/internal/repository/apikey_store_redis.go.
type cachePayload struct {
	ServiceAccountID string     `json:"sa_id"`
	TenantID         string     `json:"tenant_id"`
	Roles            []string   `json:"roles"`
	KeyID            string     `json:"key_id"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
}

// APIKeyLookupAdapter implements pkg/middleware.APIKeyLookup for account-svc.
//
// Lookup order on every API-key-authenticated request:
//  1. Redis GET apikey:{hash}               — HIT returns identity (~0.5 ms)
//  2. HTTP POST /auth/apikey/introspect     — miss or Redis down (~5 ms)
//     on success → writes identity back to Redis with TTL
//  3. 401 if both fail
//
// redis may be nil — in that case every lookup goes directly to auth-svc.
type APIKeyLookupAdapter struct {
	client *Client
	redis  *redis.Client
}

// NewAPIKeyLookup returns a pkgmiddleware.APIKeyLookup backed by Redis + auth-svc.
// redisClient may be nil; in that case all lookups go straight to HTTP introspect.
func NewAPIKeyLookup(c *Client, redisClient *redis.Client) pkgmiddleware.APIKeyLookup {
	return &APIKeyLookupAdapter{client: c, redis: redisClient}
}

// FindActiveByHash resolves a SHA-256 hash to a ServiceAccountIdentity.
// Called by AuthenticateAPIKey on every API-key-authenticated request.
func (a *APIKeyLookupAdapter) FindActiveByHash(ctx context.Context, hash string) (*pkgmiddleware.ServiceAccountIdentity, error) {
	// ── 1. Redis ──────────────────────────────────────────────────────────────
	if a.redis != nil {
		if identity, ok := a.redisGet(ctx, hash); ok {
			return identity, nil
		}
	}

	// ── 2. HTTP introspect → auth-svc ─────────────────────────────────────────
	identity, err := a.client.IntrospectAPIKey(ctx, hash)
	if err != nil {
		return nil, err
	}

	// ── 3. Write result back to Redis for future requests ─────────────────────
	if a.redis != nil {
		a.redisSet(ctx, hash, identity)
	}

	return identity, nil
}

// UpdateLastUsed is a no-op: auth-svc fires the last_used_at update
// asynchronously as part of every IntrospectAPIKey call.
func (a *APIKeyLookupAdapter) UpdateLastUsed(_ context.Context, _ string) error {
	return nil
}

// redisGet reads the identity from Redis.
// Returns (identity, true) on a valid cache hit, (nil, false) on miss or error.
func (a *APIKeyLookupAdapter) redisGet(ctx context.Context, hash string) (*pkgmiddleware.ServiceAccountIdentity, bool) {
	data, err := a.redis.Get(ctx, apiKeyCachePrefix+hash).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false // clean miss
	}
	if err != nil {
		slog.WarnContext(ctx, "apikey cache: redis unavailable, falling back to auth-svc",
			slog.String("error", err.Error()),
		)
		return nil, false
	}

	var p cachePayload
	if err := json.Unmarshal(data, &p); err != nil {
		slog.WarnContext(ctx, "apikey cache: corrupt payload, falling back to auth-svc",
			slog.String("error", err.Error()),
		)
		return nil, false
	}

	// Guard against stale entries where the key expired since it was cached.
	if p.ExpiresAt != nil && time.Now().UTC().After(*p.ExpiresAt) {
		_ = a.redis.Del(ctx, apiKeyCachePrefix+hash)
		return nil, false
	}

	return &pkgmiddleware.ServiceAccountIdentity{
		ServiceAccountID: p.ServiceAccountID,
		TenantID:         p.TenantID,
		Roles:            p.Roles,
		KeyID:            p.KeyID,
		ExpiresAt:        p.ExpiresAt,
	}, true
}

// redisSet writes the identity to Redis with a 5-minute TTL (capped to key
// expiry if the key expires sooner). Non-fatal — errors are logged only.
func (a *APIKeyLookupAdapter) redisSet(ctx context.Context, hash string, identity *pkgmiddleware.ServiceAccountIdentity) {
	p := cachePayload{
		ServiceAccountID: identity.ServiceAccountID,
		TenantID:         identity.TenantID,
		Roles:            identity.Roles,
		KeyID:            identity.KeyID,
		ExpiresAt:        identity.ExpiresAt,
	}
	b, err := json.Marshal(p)
	if err != nil {
		return
	}

	ttl := apiKeyCacheTTL
	if identity.ExpiresAt != nil {
		if remaining := time.Until(*identity.ExpiresAt); remaining < ttl {
			ttl = remaining
		}
	}

	if err := a.redis.Set(ctx, apiKeyCachePrefix+hash, b, ttl).Err(); err != nil {
		slog.WarnContext(ctx, "apikey cache: failed to write to redis",
			slog.String("key_id", identity.KeyID),
			slog.String("error", err.Error()),
		)
	}
}
