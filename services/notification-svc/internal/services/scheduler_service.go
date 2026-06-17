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
	"github.com/robfig/cron/v3"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/notification-svc/internal/repository"
)

// SchedulerService manages notification schedules.
type SchedulerService interface {
	Create(ctx context.Context, req *dto.CreateScheduleRequest) (*dto.ScheduleResponse, error)
	Update(ctx context.Context, id string, req *dto.UpdateScheduleRequest) (*dto.ScheduleResponse, error)
	Delete(ctx context.Context, id string) error
	Enable(ctx context.Context, id string) (*dto.ScheduleResponse, error)
	Disable(ctx context.Context, id string) (*dto.ScheduleResponse, error)
	GetByID(ctx context.Context, id string) (*dto.ScheduleResponse, error)
	List(ctx context.Context, filter dto.ListSchedulesFilter) (*dto.PaginatedSchedulesResponse, error)
}

// cronParser is a shared standard 5-field cron parser.
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

type schedulerService struct {
	tr   *observability.ServiceTracer
	repo repository.ScheduleRepository
}

// NewSchedulerService creates a SchedulerService.
func NewSchedulerService(repo repository.ScheduleRepository) SchedulerService {
	return &schedulerService{
		tr:   observability.NewServiceTracer("SchedulerService"),
		repo: repo,
	}
}

func (s *schedulerService) Create(ctx context.Context, req *dto.CreateScheduleRequest) (res *dto.ScheduleResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Create", attribute.String("schedule.name", req.Name))
	defer s.tr.Finish(span, &err)

	if req.Recurring && req.CronExpr == "" {
		return nil, pkgerrors.Validation("cron_expr", "required for recurring schedules")
	}
	if !req.Recurring && req.ScheduledAt == nil {
		return nil, pkgerrors.Validation("scheduled_at", "required for one-time schedules")
	}

	var nextRunAt *time.Time
	if req.Recurring {
		t, err := computeNextRun(req.CronExpr, time.Now().UTC())
		if err != nil {
			return nil, pkgerrors.Validation("cron_expr", fmt.Sprintf("invalid: %s", err))
		}
		nextRunAt = &t
	} else {
		nextRunAt = req.ScheduledAt
	}

	varsJSON, _ := json.Marshal(req.TemplateVars)
	schedule := &dao.Schedule{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Description:  req.Description,
		Channel:      req.Channel,
		TemplateCode: req.TemplateCode,
		Recipient:    req.Recipient,
		TemplateVars: varsJSON,
		CronExpr:     repository.NullString(req.CronExpr),
		ScheduledAt:  req.ScheduledAt,
		Recurring:    req.Recurring,
		Enabled:      true,
		NextRunAt:    nextRunAt,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	if err = s.repo.Create(ctx, schedule); err != nil {
		return nil, fmt.Errorf("scheduler_service.Create: %w", err)
	}

	slog.InfoContext(ctx, "schedule created",
		slog.String("schedule_id", schedule.ID),
		slog.String("name", schedule.Name),
		slog.Bool("recurring", schedule.Recurring),
	)
	return toScheduleResponse(schedule), nil
}

func (s *schedulerService) Update(ctx context.Context, id string, req *dto.UpdateScheduleRequest) (res *dto.ScheduleResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Update", attribute.String("schedule.id", id))
	defer s.tr.Finish(span, &err)

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrScheduleNotFound) {
			return nil, pkgerrors.NotFound("schedule", id)
		}
		return nil, fmt.Errorf("scheduler_service.Update: get: %w", err)
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.TemplateCode != "" {
		existing.TemplateCode = req.TemplateCode
	}
	if req.Recipient != "" {
		existing.Recipient = req.Recipient
	}
	if req.TemplateVars != nil {
		existing.TemplateVars, _ = json.Marshal(req.TemplateVars)
	}
	if req.CronExpr != "" {
		t, parseErr := computeNextRun(req.CronExpr, time.Now().UTC())
		if parseErr != nil {
			return nil, pkgerrors.Validation("cron_expr", fmt.Sprintf("invalid: %s", parseErr))
		}
		existing.CronExpr = repository.NullString(req.CronExpr)
		existing.NextRunAt = &t
	}
	if req.ScheduledAt != nil {
		existing.ScheduledAt = req.ScheduledAt
		existing.NextRunAt = req.ScheduledAt
	}

	if err = s.repo.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("scheduler_service.Update: %w", err)
	}
	return toScheduleResponse(existing), nil
}

func (s *schedulerService) Delete(ctx context.Context, id string) (err error) {
	ctx, span := s.tr.Start(ctx, "Delete", attribute.String("schedule.id", id))
	defer s.tr.Finish(span, &err)

	if err = s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrScheduleNotFound) {
			return pkgerrors.NotFound("schedule", id)
		}
		return fmt.Errorf("scheduler_service.Delete: %w", err)
	}
	return nil
}

func (s *schedulerService) Enable(ctx context.Context, id string) (res *dto.ScheduleResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Enable", attribute.String("schedule.id", id))
	defer s.tr.Finish(span, &err)

	if err = s.repo.SetEnabled(ctx, id, true); err != nil {
		if errors.Is(err, repository.ErrScheduleNotFound) {
			return nil, pkgerrors.NotFound("schedule", id)
		}
		return nil, fmt.Errorf("scheduler_service.Enable: %w", err)
	}
	return s.GetByID(ctx, id)
}

func (s *schedulerService) Disable(ctx context.Context, id string) (res *dto.ScheduleResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Disable", attribute.String("schedule.id", id))
	defer s.tr.Finish(span, &err)

	if err = s.repo.SetEnabled(ctx, id, false); err != nil {
		if errors.Is(err, repository.ErrScheduleNotFound) {
			return nil, pkgerrors.NotFound("schedule", id)
		}
		return nil, fmt.Errorf("scheduler_service.Disable: %w", err)
	}
	return s.GetByID(ctx, id)
}

func (s *schedulerService) GetByID(ctx context.Context, id string) (res *dto.ScheduleResponse, err error) {
	ctx, span := s.tr.Start(ctx, "GetByID", attribute.String("schedule.id", id))
	defer s.tr.Finish(span, &err)

	schedule, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrScheduleNotFound) {
			return nil, pkgerrors.NotFound("schedule", id)
		}
		return nil, fmt.Errorf("scheduler_service.GetByID: %w", err)
	}
	return toScheduleResponse(schedule), nil
}

func (s *schedulerService) List(ctx context.Context, filter dto.ListSchedulesFilter) (res *dto.PaginatedSchedulesResponse, err error) {
	ctx, span := s.tr.Start(ctx, "List")
	defer s.tr.Finish(span, &err)

	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("scheduler_service.List: %w", err)
	}

	responses := make([]*dto.ScheduleResponse, len(items))
	for i, s := range items {
		responses[i] = toScheduleResponse(s)
	}

	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	return &dto.PaginatedSchedulesResponse{
		Items:      responses,
		Page:       filter.Page,
		PageSize:   pageSize,
		TotalCount: total,
		TotalPages: totalPages,
	}, nil
}

// computeNextRun parses a 5-field cron expression and returns the next fire time after from.
func computeNextRun(expr string, from time.Time) (time.Time, error) {
	schedule, err := cronParser.Parse(expr)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse cron expression %q: %w", expr, err)
	}
	return schedule.Next(from), nil
}

// ComputeNextRun is exported for use by the scheduler worker.
func ComputeNextRun(expr string, from time.Time) (time.Time, error) {
	return computeNextRun(expr, from)
}

// ── mapping helpers ───────────────────────────────────────────────────────────

func toScheduleResponse(s *dao.Schedule) *dto.ScheduleResponse {
	r := &dto.ScheduleResponse{
		ID:           s.ID,
		Name:         s.Name,
		Description:  s.Description,
		Channel:      s.Channel,
		TemplateCode: s.TemplateCode,
		Recipient:    s.Recipient,
		Recurring:    s.Recurring,
		Enabled:      s.Enabled,
		ScheduledAt:  s.ScheduledAt,
		LastRunAt:    s.LastRunAt,
		NextRunAt:    s.NextRunAt,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
	}
	if s.CronExpr.Valid {
		r.CronExpr = s.CronExpr.String
	}
	if len(s.TemplateVars) > 0 {
		_ = json.Unmarshal(s.TemplateVars, &r.TemplateVars)
	}
	return r
}
