// Package repository contains the data access layer for account-svc.
// Each file defines one repository: interface and GORM implementation together.
// Uses the global slog logger — no constructor injection required.
package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/sanusi/banking/services/account-svc/internal/domain/dao"
)

// ── Sentinel errors ───────────────────────────────────────────────────────────

var (
	ErrNotFound          = errors.New("account not found")
	ErrConflict          = errors.New("account was modified concurrently, please retry")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrAccountNotActive  = errors.New("account is not active")
)

// ── Interface ─────────────────────────────────────────────────────────────────

type AccountRepository interface {
	Create(ctx context.Context, account *dao.Account) error
	GetByID(ctx context.Context, id string) (*dao.Account, error)
	Update(ctx context.Context, account *dao.Account) error
	List(ctx context.Context, customerID string, page, pageSize int) ([]*dao.Account, int64, error)
}

// ── Implementation ────────────────────────────────────────────────────────────

type accountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) AccountRepository {
	return &accountRepository{db: db}
}

func (r *accountRepository) Create(ctx context.Context, account *dao.Account) error {
	if err := r.db.WithContext(ctx).Create(account).Error; err != nil {
		slog.ErrorContext(ctx, "repository: create account", slog.String("error", err.Error()))
		return fmt.Errorf("create account: %w", err)
	}
	return nil
}

func (r *accountRepository) GetByID(ctx context.Context, id string) (*dao.Account, error) {
	var account dao.Account
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&account).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		slog.ErrorContext(ctx, "repository: get account by id",
			slog.String("account_id", id),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("get account: %w", err)
	}
	return &account, nil
}

// Update uses explicit field map + WHERE version = ? for optimistic locking.
// Never use db.Save() — it would overwrite every column including balance.
func (r *accountRepository) Update(ctx context.Context, account *dao.Account) error {
	result := r.db.WithContext(ctx).
		Model(&dao.Account{}).
		Where("id = ? AND version = ?", account.ID, account.Version).
		Updates(map[string]any{
			"balance":    account.Balance,
			"status":     account.Status,
			"version":    gorm.Expr("version + 1"),
			"updated_at": time.Now().UTC(),
		})

	if result.Error != nil {
		slog.ErrorContext(ctx, "repository: update account",
			slog.String("account_id", account.ID),
			slog.String("error", result.Error.Error()),
		)
		return fmt.Errorf("update account: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrConflict
	}

	account.Version++
	return nil
}

func (r *accountRepository) List(ctx context.Context, customerID string, page, pageSize int) ([]*dao.Account, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var (
		accounts []*dao.Account
		total    int64
	)

	q := r.db.WithContext(ctx).Model(&dao.Account{})
	if customerID != "" {
		q = q.Where("customer_id = ?", customerID)
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count accounts: %w", err)
	}
	if err := q.Offset((page - 1) * pageSize).Limit(pageSize).Order("created_at DESC").Find(&accounts).Error; err != nil {
		return nil, 0, fmt.Errorf("list accounts: %w", err)
	}

	return accounts, total, nil
}
