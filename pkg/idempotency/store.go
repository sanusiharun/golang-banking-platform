// Package idempotency provides the IdempotencyStore interface and shared types
// used by the idempotency middleware. Concrete implementations (Redis, Postgres,
// dual-store) live alongside this file.
package idempotency

import (
	"context"
	"errors"
	"time"
)

// ── Sentinel errors ───────────────────────────────────────────────────────────

var (
	// ErrInFlight is returned by Acquire when a PROCESSING record already exists
	// for the given scope key (concurrent duplicate request).
	ErrInFlight = errors.New("idempotency: request is already in flight")
)

// ── Types ─────────────────────────────────────────────────────────────────────

// Status represents the lifecycle state of an idempotency record.
type Status string

const (
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// Record holds the stored state of an idempotent request.
type Record struct {
	Status      Status            `json:"status"`
	StatusCode  int               `json:"status_code"`  // 0 while processing
	Headers     map[string]string `json:"headers"`      // response headers to replay
	Body        []byte            `json:"body"`         // raw response body
	CreatedAt   int64             `json:"created_at"`   // unix seconds
	CompletedAt *int64            `json:"completed_at"` // nil while processing
	ScopeKey    string            `json:"scope_key"`    // for debugging (truncate in logs)
}

// Meta carries request metadata stored alongside the record for debugging.
type Meta struct {
	IdempotencyKey string // original caller-supplied key
	CallerID       string // service account ID
	Method         string
	Path           string
	ExpiresAt      time.Time
}

// ── Interface ─────────────────────────────────────────────────────────────────

// Store is the pluggable backend for idempotency state.
// Use DualStore (redis_store + postgres_store) in production.
type Store interface {
	// Acquire atomically creates a PROCESSING record for scopeKey.
	//
	// Returns:
	//   (nil, nil)         — acquired; caller must execute the handler then call Complete.
	//   (*Record, nil)     — existing COMPLETED or FAILED record; caller must replay it.
	//   (nil, ErrInFlight) — another request is currently processing this key.
	//   (nil, err)         — store error.
	Acquire(ctx context.Context, scopeKey string, meta Meta) (*Record, error)

	// Complete stores the final response for a PROCESSING record.
	// Called after the handler executes, regardless of success or failure.
	Complete(ctx context.Context, scopeKey string, rec *Record) error
}
