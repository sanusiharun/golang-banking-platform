package dao

import "time"

// RefreshToken is the GORM model for the refresh_tokens table.
// The raw token is NEVER stored — only its SHA-256 hex hash.
type RefreshToken struct {
	ID        string    `gorm:"primaryKey;type:text"`
	UserID    string    `gorm:"type:text;not null;index"`
	TokenHash string    `gorm:"type:text;not null;uniqueIndex"`
	ExpiresAt time.Time `gorm:"not null"`
	Revoked   bool      `gorm:"not null;default:false"`
	CreatedAt time.Time
}

func (RefreshToken) TableName() string { return "refresh_tokens" }
