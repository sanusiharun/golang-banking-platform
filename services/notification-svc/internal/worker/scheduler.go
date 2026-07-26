package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/sanusi/banking/services/notification-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/notification-svc/internal/repository"
	"github.com/sanusi/banking/services/notification-svc/internal/services"
)

var (
	scheduledJobsExecuted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "scheduled_jobs_executed_total",
		Help: "Total number of scheduled notification jobs fired.",
	})

	scheduledJobsFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "scheduled_jobs_failed_total",
		Help: "Total number of scheduled notification jobs that failed to create a notification.",
	})
)

// SchedulerWorker polls the schedules table and creates notification records
// for due schedules. Runs as a single goroutine (one per instance).
type SchedulerWorker struct {
	repo        repository.ScheduleRepository
	notifRepo   repository.NotificationRepository
	cronCompute func(expr string, from time.Time) (time.Time, error)
	tickEvery   time.Duration
}

// NewSchedulerWorker creates a SchedulerWorker.
func NewSchedulerWorker(
	repo repository.ScheduleRepository,
	notifRepo repository.NotificationRepository,
) *SchedulerWorker {
	return &SchedulerWorker{
		repo:        repo,
		notifRepo:   notifRepo,
		cronCompute: services.ComputeNextRun,
		tickEvery:   60 * time.Second,
	}
}

// Start runs the scheduler loop until ctx is done.
func (w *SchedulerWorker) Start(ctx context.Context) {
	slog.Info("scheduler: starting", slog.Duration("tick_every", w.tickEvery))

	ticker := time.NewTicker(w.tickEvery)
	defer ticker.Stop()

	// Fire immediately on start to catch any overdue schedules.
	w.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler: stopped")
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *SchedulerWorker) tick(ctx context.Context) {
	schedules, err := w.repo.ClaimDue(ctx, time.Now().UTC(), 50)
	if err != nil {
		slog.ErrorContext(ctx, "scheduler: claim due failed", slog.String("error", err.Error()))
		return
	}
	for _, s := range schedules {
		w.fire(ctx, s)
	}
}

func (w *SchedulerWorker) fire(ctx context.Context, s *dao.Schedule) {
	now := time.Now().UTC()

	// Build idempotency key from schedule ID + tick time (minute precision).
	tick := now.Truncate(time.Minute).Format(time.RFC3339)
	idempotencyKey := "schedule:" + s.ID + ":" + tick

	var vars map[string]any
	if len(s.TemplateVars) > 0 {
		_ = json.Unmarshal(s.TemplateVars, &vars) //nolint:errcheck // best-effort decode of a stored jsonb column; a decode failure just leaves vars nil
	}
	varsJSON, _ := json.Marshal(vars) //nolint:errcheck // re-marshaling data just decoded above cannot fail

	n := &dao.Notification{
		ID:             uuid.New().String(),
		Channel:        s.Channel,
		Recipient:      s.Recipient,
		TemplateCode:   repository.NullString(s.TemplateCode),
		TemplateVars:   varsJSON,
		Status:         dao.StatusPending,
		MaxRetries:     3,
		IdempotencyKey: repository.NullString(idempotencyKey),
		ScheduleID:     repository.NullString(s.ID),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := w.notifRepo.Create(ctx, n); err != nil {
		slog.ErrorContext(ctx, "scheduler: create notification failed",
			slog.String("schedule_id", s.ID),
			slog.String("error", err.Error()),
		)
		scheduledJobsFailed.Inc()
		return
	}

	// Compute next_run_at for recurring schedules.
	var nextRunAt *time.Time
	if s.Recurring && s.CronExpr.Valid && s.CronExpr.String != "" {
		next, err := w.cronCompute(s.CronExpr.String, now)
		if err != nil {
			slog.ErrorContext(ctx, "scheduler: compute next run failed",
				slog.String("schedule_id", s.ID),
				slog.String("cron_expr", s.CronExpr.String),
				slog.String("error", err.Error()),
			)
		} else {
			nextRunAt = &next
		}
	}

	if err := w.repo.UpdateAfterRun(ctx, s.ID, &now, nextRunAt); err != nil {
		slog.ErrorContext(ctx, "scheduler: update after run failed",
			slog.String("schedule_id", s.ID),
			slog.String("error", err.Error()),
		)
	}

	scheduledJobsExecuted.Inc()
	slog.InfoContext(ctx, "scheduler: fired",
		slog.String("schedule_id", s.ID),
		slog.String("notification_id", n.ID),
		slog.String("channel", s.Channel),
	)
}
