package repository

import (
	"context"
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

var ErrScheduleNotFound = errors.New("schedule not found")

// ScheduleRepository defines data access for notification schedules.
type ScheduleRepository interface {
	Create(ctx context.Context, s *dao.Schedule) error
	GetByID(ctx context.Context, id string) (*dao.Schedule, error)
	Update(ctx context.Context, s *dao.Schedule) error
	Delete(ctx context.Context, id string) error
	SetEnabled(ctx context.Context, id string, enabled bool) error
	List(ctx context.Context, filter dto.ListSchedulesFilter) ([]*dao.Schedule, int64, error)
	// ClaimDue atomically claims enabled schedules with next_run_at <= now.
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]*dao.Schedule, error)
	// UpdateAfterRun updates last_run_at and next_run_at after a successful execution.
	UpdateAfterRun(ctx context.Context, id string, lastRunAt, nextRunAt *time.Time) error
}

type scheduleRepository struct {
	tr *observability.ServiceTracer
	db *gorm.DB
}

// NewScheduleRepository creates a Postgres-backed ScheduleRepository.
func NewScheduleRepository(db *gorm.DB) ScheduleRepository {
	return &scheduleRepository{
		tr: observability.NewServiceTracer("ScheduleRepository"),
		db: db,
	}
}

func (r *scheduleRepository) Create(ctx context.Context, s *dao.Schedule) (err error) {
	ctx, span := r.tr.Start(ctx, "Create", attribute.String("schedule.name", s.Name))
	defer r.tr.Finish(span, &err)

	if err = r.db.WithContext(ctx).Create(s).Error; err != nil {
		slog.ErrorContext(ctx, "schedule_repository: create", slog.String("error", err.Error()))
		return fmt.Errorf("schedule_repository.Create: %w", err)
	}
	return nil
}

func (r *scheduleRepository) GetByID(ctx context.Context, id string) (res *dao.Schedule, err error) {
	ctx, span := r.tr.Start(ctx, "GetByID", attribute.String("schedule.id", id))
	defer r.tr.Finish(span, &err)

	var s dao.Schedule
	if err = r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrScheduleNotFound
		}
		return nil, fmt.Errorf("schedule_repository.GetByID: %w", err)
	}
	return &s, nil
}

func (r *scheduleRepository) Update(ctx context.Context, s *dao.Schedule) (err error) {
	ctx, span := r.tr.Start(ctx, "Update", attribute.String("schedule.id", s.ID))
	defer r.tr.Finish(span, &err)

	result := r.db.WithContext(ctx).Model(&dao.Schedule{}).Where("id = ?", s.ID).Updates(map[string]any{
		"name":          s.Name,
		"description":   s.Description,
		"template_code": s.TemplateCode,
		"recipient":     s.Recipient,
		"template_vars": s.TemplateVars,
		"cron_expr":     s.CronExpr,
		"scheduled_at":  s.ScheduledAt,
		"next_run_at":   s.NextRunAt,
		"updated_at":    time.Now().UTC(),
	})
	if result.Error != nil {
		return fmt.Errorf("schedule_repository.Update: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrScheduleNotFound
	}
	return nil
}

func (r *scheduleRepository) Delete(ctx context.Context, id string) (err error) {
	ctx, span := r.tr.Start(ctx, "Delete", attribute.String("schedule.id", id))
	defer r.tr.Finish(span, &err)

	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&dao.Schedule{})
	if result.Error != nil {
		return fmt.Errorf("schedule_repository.Delete: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrScheduleNotFound
	}
	return nil
}

func (r *scheduleRepository) SetEnabled(ctx context.Context, id string, enabled bool) (err error) {
	ctx, span := r.tr.Start(ctx, "SetEnabled",
		attribute.String("schedule.id", id),
		attribute.Bool("enabled", enabled),
	)
	defer r.tr.Finish(span, &err)

	result := r.db.WithContext(ctx).Model(&dao.Schedule{}).Where("id = ?", id).Updates(map[string]any{
		"enabled":    enabled,
		"updated_at": time.Now().UTC(),
	})
	if result.Error != nil {
		return fmt.Errorf("schedule_repository.SetEnabled: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrScheduleNotFound
	}
	return nil
}

func (r *scheduleRepository) List(ctx context.Context, filter dto.ListSchedulesFilter) (items []*dao.Schedule, total int64, err error) {
	ctx, span := r.tr.Start(ctx, "List")
	defer r.tr.Finish(span, &err)

	q := r.db.WithContext(ctx).Model(&dao.Schedule{})
	if filter.Channel != "" {
		q = q.Where("channel = ?", filter.Channel)
	}
	if filter.Enabled != nil {
		q = q.Where("enabled = ?", *filter.Enabled)
	}
	if filter.Recurring != nil {
		q = q.Where("recurring = ?", *filter.Recurring)
	}

	if err = q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("schedule_repository.List count: %w", err)
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	if err = q.Offset((page-1)*pageSize).Limit(pageSize).Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("schedule_repository.List: %w", err)
	}
	return items, total, nil
}

func (r *scheduleRepository) ClaimDue(ctx context.Context, now time.Time, limit int) (res []*dao.Schedule, err error) {
	ctx, span := r.tr.Start(ctx, "ClaimDue")
	defer r.tr.Finish(span, &err)

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("enabled = true AND next_run_at <= ?", now).
			Limit(limit).
			Find(&res).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("schedule_repository.ClaimDue: %w", err)
	}
	return res, nil
}

func (r *scheduleRepository) UpdateAfterRun(ctx context.Context, id string, lastRunAt, nextRunAt *time.Time) (err error) {
	ctx, span := r.tr.Start(ctx, "UpdateAfterRun", attribute.String("schedule.id", id))
	defer r.tr.Finish(span, &err)

	updates := map[string]any{
		"last_run_at": lastRunAt,
		"next_run_at": nextRunAt,
		"updated_at":  time.Now().UTC(),
	}
	if nextRunAt == nil {
		// One-time schedule fired — disable it.
		updates["enabled"] = false
	}

	result := r.db.WithContext(ctx).Model(&dao.Schedule{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("schedule_repository.UpdateAfterRun: %w", result.Error)
	}
	return nil
}
