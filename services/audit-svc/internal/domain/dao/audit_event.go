// Package dao contains GORM models (Data Access Objects) for audit-svc.
package dao

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuditEvent is the GORM model for the audit_events table.
// The table is append-only: no Update or Delete methods exist on the repository.
type AuditEvent struct {
	ID          string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	ActorType   string `gorm:"not null"`
	ActorID     string `gorm:"not null;index:idx_audit_actor,priority:1"`
	ActorEmail  string
	Action      string `gorm:"not null;index:idx_audit_action,priority:1"`
	Status      string `gorm:"not null;default:'success'"`
	Resource    string
	ResourceID  string
	ServiceName string `gorm:"not null;index:idx_audit_service_time,priority:1"`
	TraceID     string
	IPAddress   string
	UserAgent   string
	Metadata    []byte    `gorm:"type:jsonb"` // stored as raw JSON
	CreatedAt   time.Time `gorm:"not null;default:now();index:idx_audit_actor,priority:2;index:idx_audit_action,priority:2;index:idx_audit_service_time,priority:2"`
}

// TableName sets the Postgres table name.
func (AuditEvent) TableName() string { return "audit_events" }

// BeforeCreate assigns a UUID if one hasn't been set.
func (a *AuditEvent) BeforeCreate(_ *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}
