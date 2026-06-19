// Package postgres provides PostgreSQL implementations of payment-svc repositories.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/repository"
)

// PostgresTransactionRepository implements repository.TransactionRepository.
type PostgresTransactionRepository struct {
	db *gorm.DB
}

// NewTransactionRepository creates a PostgresTransactionRepository.
func NewTransactionRepository(db *gorm.DB) repository.TransactionRepository {
	return &PostgresTransactionRepository{db: db}
}

func (r *PostgresTransactionRepository) Create(ctx context.Context, txn *dao.Transaction) error {
	if err := r.db.WithContext(ctx).Create(txn).Error; err != nil {
		return fmt.Errorf("transaction_repo.Create: %w", err)
	}
	return nil
}

func (r *PostgresTransactionRepository) UpdateStatus(ctx context.Context, id, status string, failureReason *string) error {
	updates := map[string]any{
		"status":     status,
		"updated_at": time.Now().UTC(),
	}
	if failureReason != nil {
		updates["failure_reason"] = failureReason
	}
	if status == dto.StatusSuccess {
		now := time.Now().UTC()
		updates["completed_at"] = now
	}
	if status == dto.StatusReversed {
		now := time.Now().UTC()
		updates["reversed_at"] = now
	}
	result := r.db.WithContext(ctx).
		Model(&dao.Transaction{}).
		Where("id = ?", id).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("transaction_repo.UpdateStatus: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return pkgerrors.NotFound("transaction", id)
	}
	return nil
}

func (r *PostgresTransactionRepository) GetByID(ctx context.Context, id string) (*dao.Transaction, error) {
	var txn dao.Transaction
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&txn).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkgerrors.NotFound("transaction", id)
		}
		return nil, fmt.Errorf("transaction_repo.GetByID: %w", err)
	}
	return &txn, nil
}

func (r *PostgresTransactionRepository) GetByIdempotencyKey(ctx context.Context, key string) (*dao.Transaction, error) {
	var txn dao.Transaction
	if err := r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&txn).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkgerrors.NotFound("transaction", key)
		}
		return nil, fmt.Errorf("transaction_repo.GetByIdempotencyKey: %w", err)
	}
	return &txn, nil
}

func (r *PostgresTransactionRepository) ListByAccount(ctx context.Context, filter dto.ListFilter) ([]*dao.Transaction, int64, error) {
	q := r.db.WithContext(ctx).Model(&dao.Transaction{})

	if filter.AccountID != "" {
		q = q.Where("source_account_id = ? OR destination_account_id = ?", filter.AccountID, filter.AccountID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.From != nil {
		q = q.Where("created_at >= ?", filter.From)
	}
	if filter.To != nil {
		q = q.Where("created_at <= ?", filter.To)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("transaction_repo.ListByAccount count: %w", err)
	}

	var items []*dao.Transaction
	if err := q.Order("created_at DESC").
		Limit(filter.Limit).
		Offset(filter.Offset).
		Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("transaction_repo.ListByAccount: %w", err)
	}
	return items, total, nil
}

func (r *PostgresTransactionRepository) IncrementRetryCount(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Model(&dao.Transaction{}).
		Where("id = ?", id).
		UpdateColumn("retry_count", gorm.Expr("retry_count + 1"))
	if result.Error != nil {
		return fmt.Errorf("transaction_repo.IncrementRetryCount: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return pkgerrors.NotFound("transaction", id)
	}
	return nil
}

func (r *PostgresTransactionRepository) GetReversal(ctx context.Context, originalTxnID string) (*dao.Reversal, error) {
	var rev dao.Reversal
	if err := r.db.WithContext(ctx).Where("original_txn_id = ?", originalTxnID).First(&rev).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkgerrors.NotFound("reversal", originalTxnID)
		}
		return nil, fmt.Errorf("transaction_repo.GetReversal: %w", err)
	}
	return &rev, nil
}

func (r *PostgresTransactionRepository) CreateReversal(ctx context.Context, rev *dao.Reversal) error {
	if err := r.db.WithContext(ctx).Create(rev).Error; err != nil {
		return fmt.Errorf("transaction_repo.CreateReversal: %w", err)
	}
	return nil
}

func (r *PostgresTransactionRepository) UpdateReversalStatus(ctx context.Context, id, status string, failureReason *string) error {
	updates := map[string]any{"status": status}
	if failureReason != nil {
		updates["failure_reason"] = failureReason
	}
	if status == dto.StatusSuccess {
		now := time.Now().UTC()
		updates["completed_at"] = now
	}
	result := r.db.WithContext(ctx).
		Model(&dao.Reversal{}).
		Where("id = ?", id).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("transaction_repo.UpdateReversalStatus: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return pkgerrors.NotFound("reversal", id)
	}
	return nil
}
