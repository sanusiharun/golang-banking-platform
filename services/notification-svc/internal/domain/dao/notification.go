package dao

import (
	"database/sql"
	"time"
)

// Notification status constants.
const (
	StatusPending    = "PENDING"
	StatusProcessing = "PROCESSING"
	StatusSent       = "SENT"
	StatusDelivered  = "DELIVERED"
	StatusFailed     = "FAILED"
	StatusRetrying   = "RETRYING"
	StatusCancelled  = "CANCELLED"
)

// Channel type constants.
const (
	ChannelEmail    = "EMAIL"
	ChannelSMS      = "SMS"
	ChannelPush     = "PUSH"
	ChannelWhatsApp = "WHATSAPP"
	ChannelWebhook  = "WEBHOOK"
)

// Notification is the GORM model for the notifications table.
type Notification struct {
	ID             string         `gorm:"primaryKey;type:text"`
	Channel        string         `gorm:"not null;type:text;index"`
	Recipient      string         `gorm:"not null;type:text;index"`
	TemplateID     sql.NullString `gorm:"type:text"`
	TemplateCode   sql.NullString `gorm:"type:text;index"`
	TemplateVars   []byte         `gorm:"type:jsonb"`
	Payload        []byte         `gorm:"type:jsonb"`
	Status         string         `gorm:"not null;type:text;default:'PENDING';index"`
	ProviderRef    sql.NullString `gorm:"type:text"`
	ProviderResp   []byte         `gorm:"type:jsonb"`
	ErrorMessage   sql.NullString `gorm:"type:text"`
	RetryCount     int            `gorm:"not null;default:0"`
	MaxRetries     int            `gorm:"not null;default:3"`
	IdempotencyKey sql.NullString `gorm:"type:text;uniqueIndex:idx_notifications_idempotency_key,where:idempotency_key IS NOT NULL"`
	ScheduleID     sql.NullString `gorm:"type:text;index"`
	ScheduledAt    *time.Time     `gorm:"type:timestamptz"`
	SentAt         *time.Time     `gorm:"type:timestamptz"`
	DeliveredAt    *time.Time     `gorm:"type:timestamptz"`
	CreatedAt      time.Time      `gorm:"not null;autoCreateTime;index:idx_notifications_created_at,sort:desc"`
	UpdatedAt      time.Time      `gorm:"not null;autoUpdateTime"`
}

func (Notification) TableName() string { return "notifications" }
