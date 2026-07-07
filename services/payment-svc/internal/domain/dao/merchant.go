package dao

import "time"

// Merchant is the GORM model for the merchants table (QRIS merchant registry).
type Merchant struct {
	ID         string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	NMID       string    `gorm:"type:text;not null;uniqueIndex"`
	Name       string    `gorm:"type:text;not null"`
	City       string    `gorm:"type:text;not null"`
	PostalCode *string   `gorm:"type:text"`
	MCC        string    `gorm:"type:char(4);not null"`
	Country    string    `gorm:"type:char(2);not null;default:ID"`
	AccountID  string    `gorm:"type:uuid;not null"`
	Currency   string    `gorm:"type:char(3);not null;default:IDR"`
	Status     string    `gorm:"type:text;not null;default:ACTIVE"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

func (Merchant) TableName() string { return "merchants" }
