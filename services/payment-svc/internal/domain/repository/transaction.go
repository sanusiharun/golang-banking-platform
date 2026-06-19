// Package repository defines data access interfaces for payment-svc.
// Concrete implementations live in internal/infra/postgres/.
package repository

import (
	"context"

	"github.com/sanusi/banking/services/payment-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dto"
)

// TransactionRepository defines all persistence operations for transactions and reversals.
type TransactionRepository interface {
	// Create persists a new transaction record.
	Create(ctx context.Context, txn *dao.Transaction) error

	// UpdateStatus updates the status and optional failure reason of a transaction.
	UpdateStatus(ctx context.Context, id, status string, failureReason *string) error

	// GetByID retrieves a transaction by its primary key.
	GetByID(ctx context.Context, id string) (*dao.Transaction, error)

	// GetByIdempotencyKey retrieves a transaction by its idempotency key.
	GetByIdempotencyKey(ctx context.Context, key string) (*dao.Transaction, error)

	// ListByAccount returns paginated transactions matching the filter.
	// Returns the items and the total count for pagination.
	ListByAccount(ctx context.Context, filter dto.ListFilter) ([]*dao.Transaction, int64, error)

	// IncrementRetryCount atomically increments the retry_count for a transaction.
	IncrementRetryCount(ctx context.Context, id string) error

	// GetReversal retrieves the reversal record for a given original transaction ID.
	GetReversal(ctx context.Context, originalTxnID string) (*dao.Reversal, error)

	// CreateReversal persists a new reversal record.
	CreateReversal(ctx context.Context, rev *dao.Reversal) error

	// UpdateReversalStatus updates the status and optional failure reason of a reversal.
	UpdateReversalStatus(ctx context.Context, id, status string, failureReason *string) error
}
