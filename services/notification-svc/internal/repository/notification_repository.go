// Package repository contains data access interfaces and GORM implementations
// for notification-svc.
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dto"
)

// Sentinel errors for notification repository.
var (
	ErrNotificationNotFound = errors.New("notification not found")
	ErrDuplicateIdempotency = errors.New("notification with this idempotency key already exists")
)

// NotificationRepository defines data access for notifications.
type NotificationRepository interface {
	Create(ctx context.Context, n *dao.Notification) error
	GetByID(ctx context.Context, id string) (*dao.Notification, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*dao.Notification, error)
	Update(ctx context.Context, n *dao.Notification) error
	UpdateStatus(ctx context.Context, id, status string, extras map[string]any) error
	List(ctx context.Context, filter dto.ListNotificationsFilter) ([]*dao.Notification, int64, error)
	// ClaimPending atomically claims up to batchSize PENDING (or RETRYING) notifications
	// whose scheduled_at is null or in the past. Returns the claimed records.
	ClaimPending(ctx context.Context, batchSize int, now time.Time) ([]*dao.Notification, error)
}

type notificationRepository struct {
	tr *observability.ServiceTracer
	db *gorm.DB
}

// NewNotificationRepository creates a Postgres-backed NotificationRepository.
func NewNotificationRepository(db *gorm.DB) NotificationRepository {
	return &notificationRepository{
		tr: observability.NewServiceTracer("NotificationRepository"),
		db: db,
	}
}

func (r *notificationRepository) Create(ctx context.Context, n *dao.Notification) (err error) {
	ctx, span := r.tr.Start(ctx, "Create",
		attribute.String("notification.id", n.ID),
		attribute.String("notification.channel", n.Channel),
	)
	defer r.tr.Finish(span, &err)

	if err = r.db.WithContext(ctx).Create(n).Error; err != nil {
		slog.ErrorContext(ctx, "notification_repository: create", slog.String("error", err.Error()))
		return fmt.Errorf("notification_repository.Create: %w", err)
	}
	return nil
}

func (r *notificationRepository) GetByID(ctx context.Context, id string) (res *dao.Notification, err error) {
	ctx, span := r.tr.Start(ctx, "GetByID", attribute.String("notification.id", id))
	defer r.tr.Finish(span, &err)

	var n dao.Notification
	if err = r.db.WithContext(ctx).Where("id = ?", id).First(&n).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotificationNotFound
		}
		return nil, fmt.Errorf("notification_repository.GetByID: %w", err)
	}
	return &n, nil
}

func (r *notificationRepository) GetByIdempotencyKey(ctx context.Context, key string) (res *dao.Notification, err error) {
	ctx, span := r.tr.Start(ctx, "GetByIdempotencyKey", attribute.String("idempotency_key", key))
	defer r.tr.Finish(span, &err)

	var n dao.Notification
	if err = r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&n).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotificationNotFound
		}
		return nil, fmt.Errorf("notification_repository.GetByIdempotencyKey: %w", err)
	}
	return &n, nil
}

func (r *notificationRepository) Update(ctx context.Context, n *dao.Notification) (err error) {
	ctx, span := r.tr.Start(ctx, "Update", attribute.String("notification.id", n.ID))
	defer r.tr.Finish(span, &err)

	if err = r.db.WithContext(ctx).Save(n).Error; err != nil {
		return fmt.Errorf("notification_repository.Update: %w", err)
	}
	return nil
}

func (r *notificationRepository) UpdateStatus(ctx context.Context, id, status string, extras map[string]any) (err error) {
	ctx, span := r.tr.Start(ctx, "UpdateStatus",
		attribute.String("notification.id", id),
		attribute.String("notification.status", status),
	)
	defer r.tr.Finish(span, &err)

	updates := map[string]any{
		"status":     status,
		"updated_at": time.Now().UTC(),
	}
	for k, v := range extras {
		updates[k] = v
	}

	result := r.db.WithContext(ctx).Model(&dao.Notification{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("notification_repository.UpdateStatus: %w", result.Error)
	}
	return nil
}

func (r *notificationRepository) List(ctx context.Context, filter dto.ListNotificationsFilter) (items []*dao.Notification, total int64, err error) {
	ctx, span := r.tr.Start(ctx, "List")
	defer r.tr.Finish(span, &err)

	q := r.db.WithContext(ctx).Model(&dao.Notification{})
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.Channel != "" {
		q = q.Where("channel = ?", filter.Channel)
	}
	if filter.Recipient != "" {
		q = q.Where("recipient = ?", filter.Recipient)
	}
	if filter.TemplateCode != "" {
		q = q.Where("template_code = ?", filter.TemplateCode)
	}
	if filter.ScheduleID != "" {
		q = q.Where("schedule_id = ?", filter.ScheduleID)
	}
	if filter.From != nil {
		q = q.Where("created_at >= ?", filter.From)
	}
	if filter.To != nil {
		q = q.Where("created_at <= ?", filter.To)
	}

	if err = q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("notification_repository.List count: %w", err)
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	if err = q.Offset((page - 1) * pageSize).Limit(pageSize).Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("notification_repository.List: %w", err)
	}
	return items, total, nil
}

// ClaimPending uses SELECT ... FOR UPDATE SKIP LOCKED to atomically claim
// PENDING or RETRYING notifications whose delivery window has arrived.
// This is safe for concurrent workers across multiple instances.
func (r *notificationRepository) ClaimPending(ctx context.Context, batchSize int, now time.Time) (res []*dao.Notification, err error) {
	ctx, span := r.tr.Start(ctx, "ClaimPending", attribute.Int("batch_size", batchSize))
	defer r.tr.Finish(span, &err)

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if findErr := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND (scheduled_at IS NULL OR scheduled_at <= ?)",
				[]string{dao.StatusPending, dao.StatusRetrying}, now).
			Limit(batchSize).
			Find(&res).Error; findErr != nil {
			return findErr
		}
		if len(res) == 0 {
			return nil
		}
		ids := make([]string, len(res))
		for i, n := range res {
			ids[i] = n.ID
		}
		return tx.Model(&dao.Notification{}).
			Where("id IN ?", ids).
			Updates(map[string]any{
				"status":     dao.StatusProcessing,
				"updated_at": time.Now().UTC(),
			}).Error
	})
	if err != nil {
		return nil, fmt.Errorf("notification_repository.ClaimPending: %w", err)
	}
	return res, nil
}

// ── JSON helpers used by the service layer to marshal/unmarshal JSONB columns ─

func MarshalJSON(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

func UnmarshalJSON(data []byte, v any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}

func NullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
