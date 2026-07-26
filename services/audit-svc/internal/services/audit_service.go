// Package services contains the business logic for audit-svc.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"

	pkgaudit "github.com/sanusi/banking/pkg/audit"
	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/audit-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/audit-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/audit-svc/internal/repository"
)

// AuditService defines the business operations for audit-svc.
type AuditService interface {
	// Ingest validates and persists an audit event from any source (NATS or HTTP).
	Ingest(ctx context.Context, event pkgaudit.AuditEvent) error

	// GetByID returns a single event. Returns repository.ErrNotFound if absent.
	GetByID(ctx context.Context, id string) (*dto.EventResponse, error)

	// List returns a filtered, paginated list of events.
	List(ctx context.Context, params dto.QueryParams) (*dto.EventListResponse, error)
}

// ── Implementation ────────────────────────────────────────────────────────────

type auditService struct {
	tr   *observability.ServiceTracer
	repo repository.AuditRepository
}

// New creates a new AuditService backed by the given repository.
func New(repo repository.AuditRepository) AuditService {
	return &auditService{
		tr:   observability.NewServiceTracer("AuditService"),
		repo: repo,
	}
}

// Ingest validates the event, assigns a server-side timestamp, and persists it.
// Called from both the NATS consumer and the HTTP ingest handler.
func (s *auditService) Ingest(ctx context.Context, event pkgaudit.AuditEvent) (err error) {
	ctx, span := s.tr.Start(ctx, "Ingest",
		attribute.String("audit.action", event.Action),
		attribute.String("audit.actor_id", event.ActorID),
		attribute.String("audit.service", event.ServiceName),
	)
	defer s.tr.Finish(span, &err)

	if event.Action == "" || event.ActorID == "" || event.ServiceName == "" {
		return fmt.Errorf("ingest: action, actor_id, and service_name are required")
	}

	// Marshal Metadata to JSON for the JSONB column.
	var metaBytes []byte
	if len(event.Metadata) > 0 {
		metaBytes, err = json.Marshal(event.Metadata)
		if err != nil {
			slog.WarnContext(ctx, "audit: failed to marshal metadata, storing empty",
				slog.String("action", event.Action),
				slog.String("error", err.Error()),
			)
			metaBytes = nil
		}
	}

	row := &dao.AuditEvent{
		ActorType:   event.ActorType,
		ActorID:     event.ActorID,
		ActorEmail:  event.ActorEmail,
		Action:      event.Action,
		Status:      orDefault(event.Status, pkgaudit.StatusSuccess),
		Resource:    event.Resource,
		ResourceID:  event.ResourceID,
		ServiceName: event.ServiceName,
		TraceID:     event.TraceID,
		IPAddress:   event.IPAddress,
		UserAgent:   event.UserAgent,
		Metadata:    metaBytes,
		CreatedAt:   time.Now().UTC(),
	}

	if err = s.repo.Create(ctx, row); err != nil {
		return fmt.Errorf("ingest audit event: %w", err)
	}
	return nil
}

// GetByID fetches a single event and maps it to the API response shape.
func (s *auditService) GetByID(ctx context.Context, id string) (res *dto.EventResponse, err error) {
	ctx, span := s.tr.Start(ctx, "GetByID", attribute.String("audit.id", id))
	defer s.tr.Finish(span, &err)

	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toEventResponse(row), nil
}

// List returns a paginated list of events matching the query parameters.
func (s *auditService) List(ctx context.Context, params dto.QueryParams) (res *dto.EventListResponse, err error) {
	ctx, span := s.tr.Start(ctx, "List")
	defer s.tr.Finish(span, &err)

	rows, nextCursor, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}

	events := make([]*dto.EventResponse, len(rows))
	for i, row := range rows {
		events[i] = toEventResponse(row)
	}

	return &dto.EventListResponse{
		Events:     events,
		NextCursor: nextCursor,
		Total:      len(events),
	}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func toEventResponse(row *dao.AuditEvent) *dto.EventResponse {
	resp := &dto.EventResponse{
		ID:          row.ID,
		ActorType:   row.ActorType,
		ActorID:     row.ActorID,
		ActorEmail:  row.ActorEmail,
		Action:      row.Action,
		Status:      row.Status,
		Resource:    row.Resource,
		ResourceID:  row.ResourceID,
		ServiceName: row.ServiceName,
		TraceID:     row.TraceID,
		IPAddress:   row.IPAddress,
		UserAgent:   row.UserAgent,
		CreatedAt:   row.CreatedAt,
	}
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &resp.Metadata); err != nil {
			slog.Warn("audit_service: failed to unmarshal event metadata",
				slog.String("event_id", row.ID), slog.String("error", err.Error()))
		}
	}
	return resp
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
