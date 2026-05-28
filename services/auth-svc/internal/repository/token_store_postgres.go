package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
)

type postgresTokenStore struct {
	db *gorm.DB
}

// NewPostgresTokenStore returns a TokenStore backed by PostgreSQL.
func NewPostgresTokenStore(db *gorm.DB) TokenStore {
	return &postgresTokenStore{db: db}
}

func (s *postgresTokenStore) Save(ctx context.Context, rt *dao.RefreshToken) error {
	if err := s.db.WithContext(ctx).Create(rt).Error; err != nil {
		return fmt.Errorf("token_store(postgres): save: %w", err)
	}
	return nil
}

func (s *postgresTokenStore) FindByHash(ctx context.Context, hash string) (*dao.RefreshToken, error) {
	var rt dao.RefreshToken
	err := s.db.WithContext(ctx).
		Where("token_hash = ?", hash).
		First(&rt).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("token_store(postgres): find: %w", err)
	}

	if rt.Revoked {
		slog.WarnContext(ctx, "token_store: revoked token used", slog.String("user_id", rt.UserID))
		return nil, ErrTokenRevoked
	}
	if time.Now().UTC().After(rt.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	return &rt, nil
}

func (s *postgresTokenStore) Revoke(ctx context.Context, hash string) error {
	result := s.db.WithContext(ctx).
		Model(&dao.RefreshToken{}).
		Where("token_hash = ?", hash).
		Update("revoked", true)

	if result.Error != nil {
		return fmt.Errorf("token_store(postgres): revoke: %w", result.Error)
	}
	return nil
}

func (s *postgresTokenStore) RevokeAllForUser(ctx context.Context, userID string) error {
	result := s.db.WithContext(ctx).
		Model(&dao.RefreshToken{}).
		Where("user_id = ? AND revoked = false", userID).
		Update("revoked", true)

	if result.Error != nil {
		return fmt.Errorf("token_store(postgres): revoke all for user: %w", result.Error)
	}

	slog.InfoContext(ctx, "token_store: all tokens revoked for user",
		slog.String("user_id", userID),
		slog.Int64("count", result.RowsAffected),
	)
	return nil
}
