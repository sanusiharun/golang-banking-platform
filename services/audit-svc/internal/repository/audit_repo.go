// Package repository defines the data access interface for audit-svc.
package repository

import (
	"context"

	"github.com/sanusi/banking/services/audit-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/audit-svc/internal/domain/dto"
)

// AuditRepository is the data access interface for audit events.
// Deliberately omits Update and Delete — the audit log is append-only.
type AuditRepository interface {
	// Create persists a new audit event and populates event.ID and event.CreatedAt.
	Create(ctx context.Context, event *dao.AuditEvent) error

	// GetByID retrieves a single event. Returns ErrNotFound if absent.
	GetByID(ctx context.Context, id string) (*dao.AuditEvent, error)

	// List returns a filtered, cursor-paginated slice of events.
	// nextCursor is empty when there are no more results.
	List(ctx context.Context, params dto.QueryParams) (events []*dao.AuditEvent, nextCursor string, err error)
}
