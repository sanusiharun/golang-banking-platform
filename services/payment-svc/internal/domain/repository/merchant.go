package repository

import (
	"context"

	"github.com/sanusi/banking/services/payment-svc/internal/domain/dao"
)

// MerchantRepository defines persistence operations for QRIS merchants.
type MerchantRepository interface {
	// Create persists a new merchant.
	Create(ctx context.Context, m *dao.Merchant) error

	// GetByID retrieves a merchant by its primary key.
	GetByID(ctx context.Context, id string) (*dao.Merchant, error)

	// GetByNMID retrieves a merchant by its National Merchant ID.
	GetByNMID(ctx context.Context, nmid string) (*dao.Merchant, error)
}
