package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/repository"
)

// PostgresMerchantRepository implements repository.MerchantRepository.
type PostgresMerchantRepository struct {
	db *gorm.DB
}

// NewMerchantRepository creates a PostgresMerchantRepository.
func NewMerchantRepository(db *gorm.DB) repository.MerchantRepository {
	return &PostgresMerchantRepository{db: db}
}

func (r *PostgresMerchantRepository) Create(ctx context.Context, m *dao.Merchant) error {
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("merchant_repo.Create: %w", err)
	}
	return nil
}

func (r *PostgresMerchantRepository) GetByID(ctx context.Context, id string) (*dao.Merchant, error) {
	var m dao.Merchant
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkgerrors.NotFound("merchant", id)
		}
		return nil, fmt.Errorf("merchant_repo.GetByID: %w", err)
	}
	return &m, nil
}

func (r *PostgresMerchantRepository) GetByNMID(ctx context.Context, nmid string) (*dao.Merchant, error) {
	var m dao.Merchant
	if err := r.db.WithContext(ctx).Where("nmid = ?", nmid).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkgerrors.NotFound("merchant", nmid)
		}
		return nil, fmt.Errorf("merchant_repo.GetByNMID: %w", err)
	}
	return &m, nil
}
