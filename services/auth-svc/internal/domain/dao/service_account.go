package dao

import "time"

// ServiceAccount is the GORM model for the service_accounts table.
// Service accounts are non-human identities — internal services or external partners.
// They authenticate using API keys rather than passwords.
type ServiceAccount struct {
	ID          string      `gorm:"primaryKey;type:text"`
	Name        string      `gorm:"type:text;not null"`
	Description string      `gorm:"type:text"`
	TenantID    string      `gorm:"type:text;not null;default:default"`
	Roles       StringArray `gorm:"type:text[];not null;default:'{}'"`
	IsActive    bool        `gorm:"not null;default:true"`
	CreatedBy   string      `gorm:"type:text;not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (ServiceAccount) TableName() string { return "service_accounts" }
