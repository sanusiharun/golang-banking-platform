// Package repository contains data-access logic for auth-svc.
package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
)

// ── Sentinel errors ───────────────────────────────────────────────────────────

var ErrUserNotFound = errors.New("user not found")

// ── Interface ─────────────────────────────────────────────────────────────────

type UserRepository interface {
	FindByUsername(ctx context.Context, username string) (*dao.User, error)
	FindByID(ctx context.Context, id string) (*dao.User, error)
}

// ── Implementation ────────────────────────────────────────────────────────────

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

// FindByID loads a single active user by their primary key.
func (r *userRepository) FindByID(ctx context.Context, id string) (*dao.User, error) {
	var user dao.User
	err := r.db.WithContext(ctx).
		Where("id = ? AND is_active = true", id).
		First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		slog.ErrorContext(ctx, "repository: find user by id",
			slog.String("user_id", id),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return &user, nil
}

// FindByUsername loads a single active user by username.
// Inactive users are treated as not found to prevent login.
func (r *userRepository) FindByUsername(ctx context.Context, username string) (*dao.User, error) {
	var user dao.User
	err := r.db.WithContext(ctx).
		Where("username = ? AND is_active = true", username).
		First(&user).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		slog.ErrorContext(ctx, "repository: find user by username",
			slog.String("username", username),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("find user: %w", err)
	}
	return &user, nil
}
