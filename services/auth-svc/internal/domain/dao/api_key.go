package dao

import "time"

// APIKey is the GORM model for the api_keys table.
// The raw key is NEVER stored — only its SHA-256 hex hash.
type APIKey struct {
	ID               string     `gorm:"primaryKey;type:text"`
	ServiceAccountID string     `gorm:"type:text;not null;index"`
	Name             string     `gorm:"type:text;not null"`
	KeyHash          string     `gorm:"type:text;not null;uniqueIndex"`
	KeyPrefix        string     `gorm:"type:text;not null"` // first 10 chars, for human identification
	ExpiresAt        *time.Time `gorm:"index"`
	RevokedAt        *time.Time
	LastUsedAt       *time.Time
	CreatedBy        string `gorm:"type:text;not null"`
	CreatedAt        time.Time

	// Preloaded association — not a DB column.
	ServiceAccount *ServiceAccount `gorm:"foreignKey:ServiceAccountID"`
}

func (APIKey) TableName() string { return "api_keys" }
