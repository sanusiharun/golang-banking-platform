package dao

import "time"

// Transaction is the GORM model for the transactions table.
// Rows are immutable after creation — never update a transaction record.
type Transaction struct {
	ID            string    `gorm:"primaryKey;type:text"`
	AccountID     string    `gorm:"type:text;not null;index:idx_transactions_account_id"`
	Type          string    `gorm:"type:text;not null"` // CREDIT | DEBIT
	Amount        int64     `gorm:"not null;check:amount > 0"`
	BalanceBefore int64     `gorm:"not null"`
	BalanceAfter  int64     `gorm:"not null;check:balance_after >= 0"`
	Reference     string    `gorm:"type:text"`
	CreatedAt     time.Time `gorm:"autoCreateTime"`
}

func (Transaction) TableName() string { return "transactions" }
