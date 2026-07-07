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

// PostgresQRISChargeRepository implements repository.QRISChargeRepository.
type PostgresQRISChargeRepository struct {
	db *gorm.DB
}

// NewQRISChargeRepository creates a PostgresQRISChargeRepository.
func NewQRISChargeRepository(db *gorm.DB) repository.QRISChargeRepository {
	return &PostgresQRISChargeRepository{db: db}
}

func (r *PostgresQRISChargeRepository) Create(ctx context.Context, c *dao.QRISCharge) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("qris_charge_repo.Create: %w", err)
	}
	return nil
}

func (r *PostgresQRISChargeRepository) GetByID(ctx context.Context, id string) (*dao.QRISCharge, error) {
	var c dao.QRISCharge
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkgerrors.NotFound("qris_charge", id)
		}
		return nil, fmt.Errorf("qris_charge_repo.GetByID: %w", err)
	}
	return &c, nil
}

// MarkPaid transitions PENDING → PAID and records the settling transaction.
// The WHERE clause guards on PENDING so a replay or concurrent payer that lost
// the race gets RowsAffected == 0 (surfaced as a conflict).
func (r *PostgresQRISChargeRepository) MarkPaid(ctx context.Context, id, txnID string) error {
	result := r.db.WithContext(ctx).
		Model(&dao.QRISCharge{}).
		Where("id = ? AND status = ?", id, dto.QRISChargePending).
		Updates(map[string]any{
			"status":      dto.QRISChargePaid,
			"paid_txn_id": txnID,
			"updated_at":  time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("qris_charge_repo.MarkPaid: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return pkgerrors.Conflict("qris_charge", "status", "not PENDING")
	}
	return nil
}

func (r *PostgresQRISChargeRepository) UpdateStatus(ctx context.Context, id, status string) error {
	result := r.db.WithContext(ctx).
		Model(&dao.QRISCharge{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     status,
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("qris_charge_repo.UpdateStatus: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return pkgerrors.NotFound("qris_charge", id)
	}
	return nil
}
