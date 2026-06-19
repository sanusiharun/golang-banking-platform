package dao

import "time"

// Reversal is the GORM model for the reversals table.
// Each reversal is a 1:1 compensating record for a successful transaction.
type Reversal struct {
	ID            string     `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	OriginalTxnID string     `gorm:"type:uuid;not null;uniqueIndex"`
	Status        string     `gorm:"type:text;not null;default:PENDING"`
	FailureReason *string    `gorm:"type:text"`
	InitiatedBy   string     `gorm:"type:text;not null"`
	CreatedAt     time.Time  `gorm:"autoCreateTime"`
	CompletedAt   *time.Time `gorm:"type:timestamptz"`
}

func (Reversal) TableName() string { return "reversals" }
