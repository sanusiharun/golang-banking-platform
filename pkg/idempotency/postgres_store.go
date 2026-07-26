package idempotency

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// idempotencyRow maps to the idempotency_requests table.
type idempotencyRow struct {
	ID              string `gorm:"primaryKey;type:text"`
	ScopeKey        string `gorm:"type:text;not null;uniqueIndex"`
	IdempotencyKey  string `gorm:"type:text;not null"`
	CallerID        string `gorm:"type:text;not null"`
	HTTPMethod      string `gorm:"type:text;not null"`
	URLPath         string `gorm:"type:text;not null"`
	Status          string `gorm:"type:text;not null;default:processing"`
	StatusCode      *int   `gorm:"type:int"`
	ResponseHeaders []byte `gorm:"type:jsonb"`
	ResponseBody    []byte `gorm:"type:bytea"`
	CreatedAt       time.Time
	CompletedAt     *time.Time
	ExpiresAt       time.Time `gorm:"not null"`
}

func (idempotencyRow) TableName() string { return "idempotency_requests" }

// PostgresStore is an idempotency Store backed by Postgres.
// Used as the durable fallback in DualStore when Redis is unavailable.
type PostgresStore struct {
	db  *gorm.DB
	ttl time.Duration
}

// NewPostgresStore returns a PostgresStore with the given TTL (default 24h if zero).
func NewPostgresStore(db *gorm.DB, ttl time.Duration) *PostgresStore {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &PostgresStore{db: db, ttl: ttl}
}

// Acquire atomically inserts a PROCESSING row using ON CONFLICT DO NOTHING,
// then fetches the existing row if the insert was a no-op.
// The Postgres unique constraint on scope_key is the true mutex —
// concurrent requests cannot both win the INSERT.
func (s *PostgresStore) Acquire(ctx context.Context, scopeKey string, meta Meta) (*Record, error) {
	now := time.Now()
	row := idempotencyRow{
		ScopeKey:       scopeKey,
		IdempotencyKey: meta.IdempotencyKey,
		CallerID:       meta.CallerID,
		HTTPMethod:     meta.Method,
		URLPath:        meta.Path,
		Status:         string(StatusProcessing),
		CreatedAt:      now,
		ExpiresAt:      now.Add(s.ttl),
	}

	// INSERT ... ON CONFLICT (scope_key) DO NOTHING
	result := s.db.WithContext(ctx).
		Where("scope_key = ?", scopeKey).
		FirstOrCreate(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("idempotency(postgres): acquire: %w", result.Error)
	}

	// If RowsAffected > 0, we inserted → we own this execution.
	if result.RowsAffected > 0 {
		return nil, nil
	}

	// Row already existed — return its state.
	if row.Status == string(StatusProcessing) {
		return nil, ErrInFlight
	}

	return rowToRecord(row)
}

func (s *PostgresStore) Complete(ctx context.Context, scopeKey string, rec *Record) error {
	now := time.Now()
	headersJSON, _ := json.Marshal(rec.Headers) //nolint:errcheck // marshaling a map[string]string of response headers cannot fail

	updates := map[string]any{
		"status":           string(rec.Status),
		"status_code":      rec.StatusCode,
		"response_headers": headersJSON,
		"response_body":    rec.Body,
		"completed_at":     now,
	}

	if err := s.db.WithContext(ctx).
		Model(&idempotencyRow{}).
		Where("scope_key = ?", scopeKey).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("idempotency(postgres): complete: %w", err)
	}
	return nil
}

// DeleteExpired removes idempotency records older than their expires_at.
// Called by the cleanup goroutine in auth-svc every hour.
func (s *PostgresStore) DeleteExpired(ctx context.Context) (int64, error) {
	result := s.db.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&idempotencyRow{})
	if result.Error != nil {
		return 0, fmt.Errorf("idempotency(postgres): delete expired: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func rowToRecord(row idempotencyRow) (*Record, error) {
	rec := &Record{
		Status:    Status(row.Status),
		ScopeKey:  row.ScopeKey,
		CreatedAt: row.CreatedAt.Unix(),
	}
	if row.StatusCode != nil {
		rec.StatusCode = *row.StatusCode
	}
	if row.CompletedAt != nil {
		ts := row.CompletedAt.Unix()
		rec.CompletedAt = &ts
	}
	if len(row.ResponseBody) > 0 {
		rec.Body = row.ResponseBody
	}
	if len(row.ResponseHeaders) > 0 {
		if err := json.Unmarshal(row.ResponseHeaders, &rec.Headers); err != nil {
			return nil, fmt.Errorf("idempotency(postgres): unmarshal headers: %w", err)
		}
	}
	return rec, nil
}
