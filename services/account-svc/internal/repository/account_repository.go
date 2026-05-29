// Package repository contains the data access layer for account-svc.
package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"

	"github.com/sanusi/banking/pkg/observability"
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
	tr *observability.ServiceTracer
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) AccountRepository {
	return &accountRepository{
		tr: observability.NewServiceTracer("AccountRepository"),
		db: db,
	}
}

func (r *accountRepository) Create(ctx context.Context, account *dao.Account) (err error) {
	ctx, span := r.tr.Start(ctx, "Create",
		attribute.String("account.id", account.ID),
		attribute.String("account.customer_id", account.CustomerID),
	)
	defer r.tr.Finish(span, &err)

	if err = r.db.WithContext(ctx).Create(account).Error; err != nil {
		slog.ErrorContext(ctx, "repository: create account", slog.String("error", err.Error()))
		return fmt.Errorf("create account: %w", err)
	}
	return nil
}

func (r *accountRepository) GetByID(ctx context.Context, id string) (res *dao.Account, err error) {
	ctx, span := r.tr.Start(ctx, "GetByID",
		attribute.String("account.id", id),
	)
	defer r.tr.Finish(span, &err)

	var account dao.Account
	if err = r.db.WithContext(ctx).Where("id = ?", id).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = ErrNotFound
			return nil, err
		}
		slog.ErrorContext(ctx, "repository: get account by id",
			slog.String("account_id", id),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("get account: %w", err)
	}
	return &account, nil
}

// Update uses optimistic locking via WHERE version = ? to prevent lost updates.
// Never use db.Save() — it overwrites every column including balance.
func (r *accountRepository) Update(ctx context.Context, account *dao.Account) (err error) {
	ctx, span := r.tr.Start(ctx, "Update",
		attribute.String("account.id", account.ID),
		attribute.Int64("account.version", int64(account.Version)),
	)
	defer r.tr.Finish(span, &err)

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
		err = fmt.Errorf("update account: %w", result.Error)
		return err
	}
	if result.RowsAffected == 0 {
		err = ErrConflict
		return err
	}

	account.Version++
	return nil
}

func (r *accountRepository) List(ctx context.Context, customerID string, page, pageSize int) (accounts []*dao.Account, total int64, err error) {
	ctx, span := r.tr.Start(ctx, "List",
		attribute.String("account.customer_id", customerID),
		attribute.Int("pagination.page", page),
		attribute.Int("pagination.page_size", pageSize),
	)
	defer r.tr.Finish(span, &err)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	q := r.db.WithContext(ctx).Model(&dao.Account{})
	if customerID != "" {
		q = q.Where("customer_id = ?", customerID)
	}

	if err = q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count accounts: %w", err)
	}
	if err = q.Offset((page-1)*pageSize).Limit(pageSize).Order("created_at DESC").Find(&accounts).Error; err != nil {
		return nil, 0, fmt.Errorf("list accounts: %w", err)
	}

	return accounts, total, nil
}
