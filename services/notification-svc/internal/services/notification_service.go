// Package services contains the business logic for notification-svc.
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/notification-svc/internal/repository"
)

// NotificationService manages notification lifecycle.
type NotificationService interface {
	Send(ctx context.Context, req *dto.SendNotificationRequest) (*dto.NotificationResponse, error)
	Retry(ctx context.Context, id string) (*dto.NotificationResponse, error)
	Cancel(ctx context.Context, id string) (*dto.NotificationResponse, error)
	GetByID(ctx context.Context, id string) (*dto.NotificationResponse, error)
	List(ctx context.Context, filter dto.ListNotificationsFilter) (*dto.PaginatedNotificationsResponse, error)
}

type notificationService struct {
	tr   *observability.ServiceTracer
	repo repository.NotificationRepository
}

// NewNotificationService creates a NotificationService.
func NewNotificationService(repo repository.NotificationRepository) NotificationService {
	return &notificationService{
		tr:   observability.NewServiceTracer("NotificationService"),
		repo: repo,
	}
}

func (s *notificationService) Send(ctx context.Context, req *dto.SendNotificationRequest) (res *dto.NotificationResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Send",
		attribute.String("notification.channel", req.Channel),
		attribute.String("notification.recipient", req.Recipient),
	)
	defer s.tr.Finish(span, &err)

	// Idempotency check.
	if req.IdempotencyKey != "" {
		existing, lookupErr := s.repo.GetByIdempotencyKey(ctx, req.IdempotencyKey)
		if lookupErr == nil {
			slog.InfoContext(ctx, "notification: idempotency hit",
				slog.String("idempotency_key", req.IdempotencyKey),
				slog.String("notification_id", existing.ID),
			)
			return toNotificationResponse(existing), nil
		}
		if !errors.Is(lookupErr, repository.ErrNotificationNotFound) {
			return nil, fmt.Errorf("send: check idempotency: %w", lookupErr)
		}
	}

	maxRetries := req.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	varsJSON, _ := json.Marshal(req.TemplateVars)
	payloadJSON, _ := json.Marshal(req.Payload)

	n := &dao.Notification{
		ID:             uuid.New().String(),
		Channel:        req.Channel,
		Recipient:      req.Recipient,
		TemplateCode:   repository.NullString(req.TemplateCode),
		TemplateVars:   varsJSON,
		Payload:        payloadJSON,
		Status:         dao.StatusPending,
		RetryCount:     0,
		MaxRetries:     maxRetries,
		IdempotencyKey: repository.NullString(req.IdempotencyKey),
		ScheduleID:     repository.NullString(req.ScheduleID),
		ScheduledAt:    req.ScheduledAt,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	if err = s.repo.Create(ctx, n); err != nil {
		return nil, fmt.Errorf("send: create notification: %w", err)
	}

	slog.InfoContext(ctx, "notification created",
		slog.String("notification_id", n.ID),
		slog.String("channel", n.Channel),
		slog.String("status", n.Status),
	)
	return toNotificationResponse(n), nil
}

func (s *notificationService) Retry(ctx context.Context, id string) (res *dto.NotificationResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Retry", attribute.String("notification.id", id))
	defer s.tr.Finish(span, &err)

	n, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotificationNotFound) {
			return nil, pkgerrors.NotFound("notification", id)
		}
		return nil, fmt.Errorf("retry: get notification: %w", err)
	}

	if n.Status == dao.StatusCancelled {
		return nil, pkgerrors.Conflict("notification", "status", "CANCELLED")
	}

	extras := map[string]any{
		"status":      dao.StatusPending,
		"retry_count": 0,
		"error_message": nil,
	}
	if err = s.repo.UpdateStatus(ctx, id, dao.StatusPending, extras); err != nil {
		return nil, fmt.Errorf("retry: update status: %w", err)
	}

	n.Status = dao.StatusPending
	n.RetryCount = 0
	slog.InfoContext(ctx, "notification queued for retry", slog.String("notification_id", id))
	return toNotificationResponse(n), nil
}

func (s *notificationService) Cancel(ctx context.Context, id string) (res *dto.NotificationResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Cancel", attribute.String("notification.id", id))
	defer s.tr.Finish(span, &err)

	n, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotificationNotFound) {
			return nil, pkgerrors.NotFound("notification", id)
		}
		return nil, fmt.Errorf("cancel: get notification: %w", err)
	}

	if n.Status != dao.StatusPending && n.Status != dao.StatusRetrying {
		return nil, pkgerrors.Conflict("notification", "status", n.Status)
	}

	if err = s.repo.UpdateStatus(ctx, id, dao.StatusCancelled, nil); err != nil {
		return nil, fmt.Errorf("cancel: update status: %w", err)
	}

	n.Status = dao.StatusCancelled
	slog.InfoContext(ctx, "notification cancelled", slog.String("notification_id", id))
	return toNotificationResponse(n), nil
}

func (s *notificationService) GetByID(ctx context.Context, id string) (res *dto.NotificationResponse, err error) {
	ctx, span := s.tr.Start(ctx, "GetByID", attribute.String("notification.id", id))
	defer s.tr.Finish(span, &err)

	n, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotificationNotFound) {
			return nil, pkgerrors.NotFound("notification", id)
		}
		return nil, fmt.Errorf("get_by_id: %w", err)
	}
	return toNotificationResponse(n), nil
}

func (s *notificationService) List(ctx context.Context, filter dto.ListNotificationsFilter) (res *dto.PaginatedNotificationsResponse, err error) {
	ctx, span := s.tr.Start(ctx, "List")
	defer s.tr.Finish(span, &err)

	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}

	responses := make([]*dto.NotificationResponse, len(items))
	for i, n := range items {
		responses[i] = toNotificationResponse(n)
	}

	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	return &dto.PaginatedNotificationsResponse{
		Items:      responses,
		Page:       filter.Page,
		PageSize:   pageSize,
		TotalCount: total,
		TotalPages: totalPages,
	}, nil
}

// ── mapping helpers ───────────────────────────────────────────────────────────

func toNotificationResponse(n *dao.Notification) *dto.NotificationResponse {
	r := &dto.NotificationResponse{
		ID:          n.ID,
		Channel:     n.Channel,
		Recipient:   n.Recipient,
		Status:      n.Status,
		RetryCount:  n.RetryCount,
		MaxRetries:  n.MaxRetries,
		ScheduledAt: n.ScheduledAt,
		SentAt:      n.SentAt,
		DeliveredAt: n.DeliveredAt,
		CreatedAt:   n.CreatedAt,
		UpdatedAt:   n.UpdatedAt,
	}
	if n.TemplateID.Valid {
		r.TemplateID = n.TemplateID.String
	}
	if n.TemplateCode.Valid {
		r.TemplateCode = n.TemplateCode.String
	}
	if n.IdempotencyKey.Valid {
		r.IdempotencyKey = n.IdempotencyKey.String
	}
	if n.ScheduleID.Valid {
		r.ScheduleID = n.ScheduleID.String
	}
	if n.ProviderRef.Valid {
		r.ProviderRef = n.ProviderRef.String
	}
	if n.ErrorMessage.Valid {
		r.ErrorMessage = n.ErrorMessage.String
	}
	if len(n.TemplateVars) > 0 {
		_ = json.Unmarshal(n.TemplateVars, &r.TemplateVars)
	}
	if len(n.Payload) > 0 {
		_ = json.Unmarshal(n.Payload, &r.Payload)
	}
	return r
}
