// Package repository contains data-access logic for auth-svc.
package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"

	"github.com/sanusi/banking/pkg/observability"
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
	tr *observability.ServiceTracer
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{
		tr: observability.NewServiceTracer("UserRepository"),
		db: db,
	}
}

func (r *userRepository) FindByID(ctx context.Context, id string) (res *dao.User, err error) {
	ctx, span := r.tr.Start(ctx, "FindByID",
		attribute.String("user.id", id),
	)
	defer r.tr.Finish(span, &err)

	var user dao.User
	if err = r.db.WithContext(ctx).
		Where("id = ? AND is_active = true", id).
		First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = ErrUserNotFound
			return nil, err
		}
		slog.ErrorContext(ctx, "repository: find user by id",
			slog.String("user_id", id),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return &user, nil
}

func (r *userRepository) FindByUsername(ctx context.Context, username string) (res *dao.User, err error) {
	ctx, span := r.tr.Start(ctx, "FindByUsername",
		attribute.String("user.username", username),
	)
	defer r.tr.Finish(span, &err)

	var user dao.User
	if err = r.db.WithContext(ctx).
		Where("username = ? AND is_active = true", username).
		First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = ErrUserNotFound
			return nil, err
		}
		slog.ErrorContext(ctx, "repository: find user by username",
			slog.String("username", username),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("find user: %w", err)
	}
	return &user, nil
}
