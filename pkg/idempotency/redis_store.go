package idempotency

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	redisKeyPrefix = "idempkey:"
	defaultTTL     = 24 * time.Hour
)

// luaAcquire atomically checks and sets an idempotency record.
// Returns "acquired" if the key was newly created (PROCESSING state set).
// Returns the existing JSON record if the key already exists.
// This eliminates the TOCTOU race between GET and SET NX.
var luaAcquire = redis.NewScript(`
local existing = redis.call('GET', KEYS[1])
if existing ~= false then
    return existing
end
redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
return "acquired"
`)

// RedisStore is an idempotency Store backed by Redis.
// It is intended to be used as the primary layer in DualStore.
type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisStore returns a RedisStore with the given TTL (default 24h if zero).
func NewRedisStore(client *redis.Client, ttl time.Duration) *RedisStore {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &RedisStore{client: client, ttl: ttl}
}

func (s *RedisStore) Acquire(ctx context.Context, scopeKey string, meta Meta) (*Record, error) {
	processing := &Record{
		Status:    StatusProcessing,
		CreatedAt: time.Now().Unix(),
		ScopeKey:  scopeKey,
	}
	processingJSON, err := json.Marshal(processing)
	if err != nil {
		return nil, fmt.Errorf("idempotency(redis): marshal processing record: %w", err)
	}

	result, err := luaAcquire.Run(ctx, s.client,
		[]string{redisKeyPrefix + scopeKey},
		string(processingJSON),
		int(s.ttl.Seconds()),
	).Text()
	if err != nil {
		return nil, fmt.Errorf("idempotency(redis): lua acquire: %w", err)
	}

	if result == "acquired" {
		return nil, nil // caller owns this execution
	}

	// Existing record — deserialise and return.
	var rec Record
	if err := json.Unmarshal([]byte(result), &rec); err != nil {
		// Corrupt entry — treat as acquired so the request can proceed.
		_ = s.client.Del(ctx, redisKeyPrefix+scopeKey)
		return nil, nil
	}

	if rec.Status == StatusProcessing {
		return nil, ErrInFlight
	}

	return &rec, nil
}

func (s *RedisStore) Complete(ctx context.Context, scopeKey string, rec *Record) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("idempotency(redis): marshal completed record: %w", err)
	}
	// Overwrite the PROCESSING entry with the final record, preserving the TTL
	// from the original acquire (or resetting to full TTL — simpler and safe).
	if err := s.client.Set(ctx, redisKeyPrefix+scopeKey, data, s.ttl).Err(); err != nil {
		return fmt.Errorf("idempotency(redis): complete: %w", err)
	}
	return nil
}
