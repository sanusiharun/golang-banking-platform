package dao

import "time"

// QRISCharge is the GORM model for the qris_charges table.
// Amount is stored in minor currency units; nil for static QR codes.
type QRISCharge struct {
	ID             string     `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	MerchantID     string     `gorm:"type:uuid;not null;index:idx_qris_charges_merchant,priority:1"`
	QRType         string     `gorm:"type:text;not null"`
	QRString       string     `gorm:"type:text;not null"`
	Amount         *int64     `gorm:"type:bigint"`
	Currency       string     `gorm:"type:char(3);not null;default:IDR"`
	ReferenceLabel *string    `gorm:"type:text"`
	BillNumber     *string    `gorm:"type:text"`
	Status         string     `gorm:"type:text;not null;default:PENDING"`
	PaidTxnID      *string    `gorm:"type:uuid"`
	ExpiresAt      *time.Time `gorm:"type:timestamptz"`
	CreatedAt      time.Time  `gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (QRISCharge) TableName() string { return "qris_charges" }
