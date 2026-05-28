package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
)

const (
	redisTokenPrefix      = "rt:"           // rt:{hash}
	redisUserRevokedKey   = "user_revoked:" // user_revoked:{user_id} = unix timestamp
)

type redisTokenStore struct {
	client         *redis.Client
	refreshTokenTTL time.Duration
}

// NewRedisTokenStore returns a TokenStore backed by Redis.
// refreshTokenTTL is the maximum TTL for refresh tokens, used when setting
// the user_revoked key so it naturally expires after no token can use it.
func NewRedisTokenStore(client *redis.Client, refreshTokenTTL time.Duration) TokenStore {
	return &redisTokenStore{client: client, refreshTokenTTL: refreshTokenTTL}
}

type redisTokenPayload struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *redisTokenStore) Save(ctx context.Context, rt *dao.RefreshToken) error {
	payload := redisTokenPayload{
		ID:        rt.ID,
		UserID:    rt.UserID,
		ExpiresAt: rt.ExpiresAt,
		Revoked:   rt.Revoked,
		CreatedAt: rt.CreatedAt,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("token_store(redis): marshal: %w", err)
	}

	ttl := time.Until(rt.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("token_store(redis): token already expired")
	}

	key := redisTokenPrefix + rt.TokenHash
	if err := s.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("token_store(redis): save: %w", err)
	}
	return nil
}

func (s *redisTokenStore) FindByHash(ctx context.Context, hash string) (*dao.RefreshToken, error) {
	key := redisTokenPrefix + hash
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("token_store(redis): find: %w", err)
	}

	var payload redisTokenPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("token_store(redis): unmarshal: %w", err)
	}

	if payload.Revoked {
		slog.WarnContext(ctx, "token_store: revoked token used", slog.String("user_id", payload.UserID))
		return nil, ErrTokenRevoked
	}
	if time.Now().UTC().After(payload.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	// Check if RevokeAllForUser was called after this token was issued.
	revokedBefore, err := s.client.Get(ctx, redisUserRevokedKey+payload.UserID).Int64()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("token_store(redis): check user revocation: %w", err)
	}
	if revokedBefore > 0 && payload.CreatedAt.Unix() < revokedBefore {
		return nil, ErrTokenRevoked
	}

	return &dao.RefreshToken{
		ID:        payload.ID,
		UserID:    payload.UserID,
		TokenHash: hash,
		ExpiresAt: payload.ExpiresAt,
		Revoked:   payload.Revoked,
		CreatedAt: payload.CreatedAt,
	}, nil
}

func (s *redisTokenStore) Revoke(ctx context.Context, hash string) error {
	// Re-fetch, mark revoked, re-save with remaining TTL.
	key := redisTokenPrefix + hash
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil // already gone — treat as success
		}
		return fmt.Errorf("token_store(redis): revoke fetch: %w", err)
	}

	var payload redisTokenPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("token_store(redis): revoke unmarshal: %w", err)
	}
	payload.Revoked = true

	updated, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("token_store(redis): revoke marshal: %w", err)
	}

	ttl := time.Until(payload.ExpiresAt)
	if ttl <= 0 {
		_ = s.client.Del(ctx, key)
		return nil
	}

	return s.client.Set(ctx, key, updated, ttl).Err()
}

func (s *redisTokenStore) RevokeAllForUser(ctx context.Context, userID string) error {
	// Store a timestamp — any token created before this time is considered revoked.
	// TTL matches the max refresh token lifetime so the key auto-expires.
	err := s.client.Set(ctx, redisUserRevokedKey+userID, time.Now().Unix(), s.refreshTokenTTL).Err()
	if err != nil {
		return fmt.Errorf("token_store(redis): revoke all for user: %w", err)
	}
	slog.InfoContext(ctx, "token_store: all tokens revoked for user", slog.String("user_id", userID))
	return nil
}
