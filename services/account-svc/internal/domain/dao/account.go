// Package dao contains GORM model definitions (Data Access Objects).
// One file per entity. No business logic lives here.
package dao

import "time"

// Account is the GORM model for the accounts table.
// Balance is stored in minor currency units (kobo, cents) as int64
// to avoid all floating-point precision issues in financial calculations.
type Account struct {
	ID         string    `gorm:"primaryKey;type:text"`
	CustomerID string    `gorm:"type:text;not null;index:idx_accounts_customer_id"`
	IBAN       string    `gorm:"type:text;not null;uniqueIndex:idx_accounts_iban"`
	Currency   string    `gorm:"type:char(3);not null"`
	Balance    int64     `gorm:"not null;default:0;check:balance >= 0"`
	Status     string    `gorm:"type:text;not null;default:'PENDING'"`
	Version    int       `gorm:"not null;default:1"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

func (Account) TableName() string { return "accounts" }
