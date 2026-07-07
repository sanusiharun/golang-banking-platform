package repository

import (
	"context"

	"github.com/sanusi/banking/services/payment-svc/internal/domain/dao"
)

// QRISChargeRepository defines persistence operations for QRIS charges.
type QRISChargeRepository interface {
	// Create persists a new QR charge.
	Create(ctx context.Context, c *dao.QRISCharge) error

	// GetByID retrieves a charge by its primary key.
	GetByID(ctx context.Context, id string) (*dao.QRISCharge, error)

	// MarkPaid transitions a charge to PAID and links the settling transaction.
	// It only affects a charge still in PENDING, so a concurrent second payer
	// cannot re-mark an already-paid charge.
	MarkPaid(ctx context.Context, id, txnID string) error

	// UpdateStatus sets a charge's status (e.g. EXPIRED, CANCELLED).
	UpdateStatus(ctx context.Context, id, status string) error
}
