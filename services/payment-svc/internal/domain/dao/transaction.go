// Package dao contains GORM model definitions for payment-svc.
// No business logic lives here — only table mappings.
package dao

import "time"

// Transaction is the GORM model for the transactions table.
// Amount is stored in minor currency units (kobo, cents) as int64.
type Transaction struct {
	ID                    string     `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	IdempotencyKey        string     `gorm:"type:text;not null;uniqueIndex"`
	PaymentType           string     `gorm:"type:text;not null"`
	Channel               string     `gorm:"type:text;not null"`
	SourceAccountID       string     `gorm:"type:text;not null;index:idx_txn_source_created,priority:1"`
	DestinationAccountID  string     `gorm:"type:text;not null;index:idx_txn_dest_created,priority:1"`
	Amount                int64      `gorm:"not null"`
	Currency              string     `gorm:"type:char(3);not null"`
	Status                string     `gorm:"type:text;not null;default:PENDING"`
	FailureReason         *string    `gorm:"type:text"`
	RetryCount            int        `gorm:"not null;default:0"`
	MaxRetries            int        `gorm:"not null;default:3"`
	ExternalReference     *string    `gorm:"type:text"`
	CorrelationID         *string    `gorm:"type:uuid"`
	TraceID               *string    `gorm:"type:text"`
	Description           *string    `gorm:"type:text"`
	Metadata              []byte     `gorm:"type:jsonb"`
	InitiatedBy           string     `gorm:"type:text;not null"`
	CreatedAt             time.Time  `gorm:"autoCreateTime"`
	UpdatedAt             time.Time  `gorm:"autoUpdateTime"`
	CompletedAt           *time.Time `gorm:"type:timestamptz"`
	ReversedAt            *time.Time `gorm:"type:timestamptz"`
}

func (Transaction) TableName() string { return "transactions" }
