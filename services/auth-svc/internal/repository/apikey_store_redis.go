package repository

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
)

const (
	apiKeyCachePrefix = "apikey:" // apikey:{sha256_hash} → ServiceAccountIdentity JSON
	apiKeyCacheTTL    = 5 * time.Minute
)

// redisAPIKeyStore wraps a postgres APIKeyStore and adds a Redis read-through cache
// for FindActiveByHash — the only hot-path operation. All write operations (Save,
// Revoke, etc.) delegate directly to postgres; Revoke additionally flushes Redis.
type redisAPIKeyStore struct {
	postgres APIKeyStore
	redis    *redis.Client
}

// NewRedisAPIKeyStore returns an APIKeyStore backed by Redis (cache) + Postgres (source of truth).
func NewRedisAPIKeyStore(postgres APIKeyStore, redisClient *redis.Client) APIKeyStore {
	return &redisAPIKeyStore{postgres: postgres, redis: redisClient}
}

// cachePayload is what we store in Redis — a compact version of ServiceAccountIdentity.
type cachePayload struct {
	ServiceAccountID string     `json:"sa_id"`
	TenantID         string     `json:"tenant_id"`
	Roles            []string   `json:"roles"`
	KeyID            string     `json:"key_id"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
}

func (s *redisAPIKeyStore) FindActiveByHash(ctx context.Context, hash string) (*pkgmiddleware.ServiceAccountIdentity, error) {
	cacheKey := apiKeyCachePrefix + hash

	// ── Cache read ────────────────────────────────────────────────────────────
	data, err := s.redis.Get(ctx, cacheKey).Bytes()
	if err != nil && !errors.Is(err, redis.Nil) {
		// Redis unavailable — log and fall through to Postgres.
		slog.WarnContext(ctx, "api_key cache: redis unavailable, falling back to postgres",
			slog.String("error", err.Error()),
		)
	}

	if err == nil {
		// Cache HIT — deserialise and return.
		var p cachePayload
		if jsonErr := json.Unmarshal(data, &p); jsonErr == nil {
			// Check expiry even from cache (key may have expired since last lookup).
			if p.ExpiresAt != nil && time.Now().UTC().After(*p.ExpiresAt) {
				// Stale cache entry — delete and fall through.
				_ = s.redis.Del(ctx, cacheKey)
			} else {
				return &pkgmiddleware.ServiceAccountIdentity{
					ServiceAccountID: p.ServiceAccountID,
					TenantID:         p.TenantID,
					Roles:            p.Roles,
					KeyID:            p.KeyID,
					ExpiresAt:        p.ExpiresAt,
				}, nil
			}
		}
	}

	// ── Cache MISS — query Postgres ───────────────────────────────────────────
	identity, err := s.postgres.FindActiveByHash(ctx, hash)
	if err != nil {
		return nil, err // ErrAPIKeyNotFound, ErrAPIKeyExpired, or DB error
	}

	// ── Populate cache ────────────────────────────────────────────────────────
	payload := cachePayload{
		ServiceAccountID: identity.ServiceAccountID,
		TenantID:         identity.TenantID,
		Roles:            identity.Roles,
		KeyID:            identity.KeyID,
		ExpiresAt:        identity.ExpiresAt,
	}
	if b, jsonErr := json.Marshal(payload); jsonErr == nil {
		ttl := apiKeyCacheTTL
		if identity.ExpiresAt != nil {
			remaining := time.Until(*identity.ExpiresAt)
			if remaining < ttl {
				ttl = remaining
			}
		}
		if setErr := s.redis.Set(ctx, cacheKey, b, ttl).Err(); setErr != nil {
			slog.WarnContext(ctx, "api_key cache: failed to write to redis",
				slog.String("error", setErr.Error()),
			)
			// Non-fatal — Postgres is authoritative.
		}
	}

	return identity, nil
}

// Revoke delegates to Postgres AND immediately flushes the Redis cache entry,
// ensuring revoked keys are rejected within milliseconds.
func (s *redisAPIKeyStore) Revoke(ctx context.Context, keyID, hash string) error {
	if err := s.postgres.Revoke(ctx, keyID, hash); err != nil {
		return err
	}
	if hash != "" {
		if err := s.redis.Del(ctx, apiKeyCachePrefix+hash).Err(); err != nil {
			slog.WarnContext(ctx, "api_key cache: failed to invalidate redis on revoke",
				slog.String("key_id", keyID),
				slog.String("error", err.Error()),
			)
			// Non-fatal — key will expire from cache within apiKeyCacheTTL.
		}
	}
	return nil
}

// Remaining methods delegate directly to postgres — they are management operations.

func (s *redisAPIKeyStore) Save(ctx context.Context, key *dao.APIKey) error {
	return s.postgres.Save(ctx, key)
}

func (s *redisAPIKeyStore) ListByServiceAccount(ctx context.Context, serviceAccountID string) ([]*dao.APIKey, error) {
	return s.postgres.ListByServiceAccount(ctx, serviceAccountID)
}

func (s *redisAPIKeyStore) UpdateLastUsed(ctx context.Context, keyID string) error {
	return s.postgres.UpdateLastUsed(ctx, keyID)
}

// InvalidateAll removes Redis cache entries for all hashes belonging to a service account.
// Called when is_active is set to false on the service account.
func (s *redisAPIKeyStore) InvalidateAll(ctx context.Context, hashes []string) {
	if len(hashes) == 0 {
		return
	}
	keys := make([]string, len(hashes))
	for i, h := range hashes {
		keys[i] = apiKeyCachePrefix + h
	}
	if err := s.redis.Del(ctx, keys...).Err(); err != nil {
		slog.WarnContext(ctx, "api_key cache: failed to invalidate all keys for service account",
			slog.String("error", err.Error()),
		)
	}
}

// Ensure interface compliance at compile time.
var _ APIKeyStore = (*redisAPIKeyStore)(nil)

// redisInvalidator exposes the extra InvalidateAll method without breaking the interface.
type redisInvalidator interface {
	InvalidateAll(ctx context.Context, hashes []string)
}

// CacheInvalidator returns the InvalidateAll capability if the store supports it.
// Returns nil if the store is Postgres-only.
func CacheInvalidator(store APIKeyStore) redisInvalidator {
	if ri, ok := store.(redisInvalidator); ok {
		return ri
	}
	return nil
}
