package idempotency

import (
	"context"
	"log/slog"
)

// DualStore composes a RedisStore (primary) and a PostgresStore (fallback).
//
// Acquire flow:
//  1. Try Redis (Lua atomic acquire).
//  2. On Redis error — fall back to Postgres.
//  3. On Redis success — skip Postgres on the hot path.
//
// Complete flow:
//  1. Write to Redis (fast, best-effort).
//  2. Write to Postgres asynchronously (durable).
//
// Redis outage CANNOT cause double-execution because the Postgres unique constraint
// is always the authoritative mutex when Redis is unavailable.
type DualStore struct {
	redis    *RedisStore
	postgres *PostgresStore
}

// NewDualStore returns a Store that uses Redis as the primary and Postgres as fallback.
func NewDualStore(redis *RedisStore, postgres *PostgresStore) Store {
	return &DualStore{redis: redis, postgres: postgres}
}

func (d *DualStore) Acquire(ctx context.Context, scopeKey string, meta Meta) (*Record, error) {
	// ── Try Redis first ───────────────────────────────────────────────────────
	rec, err := d.redis.Acquire(ctx, scopeKey, meta)
	if err == nil {
		// Redis succeeded — rec is nil (acquired) or an existing record (replay/in-flight).
		return rec, nil
	}
	if err == ErrInFlight {
		return nil, ErrInFlight
	}

	// ── Redis unavailable — fall back to Postgres ─────────────────────────────
	slog.WarnContext(ctx, "idempotency: redis unavailable, falling back to postgres",
		slog.String("error", err.Error()),
		slog.String("scope_key_prefix", scopeKey[:8]),
	)
	return d.postgres.Acquire(ctx, scopeKey, meta)
}

func (d *DualStore) Complete(ctx context.Context, scopeKey string, rec *Record) error {
	// Write to Redis synchronously (fast path for future cache hits).
	if err := d.redis.Complete(ctx, scopeKey, rec); err != nil {
		slog.WarnContext(ctx, "idempotency: redis complete failed",
			slog.String("error", err.Error()),
		)
		// Non-fatal — Postgres write below is authoritative.
	}

	// Write to Postgres asynchronously — durable but not on the critical path.
	go func() {
		// Use a background context so a cancelled request context doesn't abort the write.
		bgCtx := context.Background()
		if err := d.postgres.Complete(bgCtx, scopeKey, rec); err != nil {
			slog.Error("idempotency: postgres complete failed (async)",
				slog.String("error", err.Error()),
				slog.String("scope_key_prefix", scopeKey[:8]),
			)
		}
	}()

	return nil
}

// Postgres returns the underlying PostgresStore, exposing DeleteExpired for the cleanup goroutine.
func (d *DualStore) Postgres() *PostgresStore {
	return d.postgres
}
