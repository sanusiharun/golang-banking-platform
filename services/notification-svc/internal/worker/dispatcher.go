// Package worker contains background processing goroutines for notification-svc.
package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/sanusi/banking/services/notification-svc/internal/channel"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/notification-svc/internal/repository"
	"github.com/sanusi/banking/services/notification-svc/internal/template"
)

// ── Prometheus metrics ────────────────────────────────────────────────────────

var (
	notificationsSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "notification_sent_total",
		Help: "Total number of notifications successfully sent.",
	}, []string{"channel"})

	notificationsFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "notification_failed_total",
		Help: "Total number of notifications that permanently failed.",
	}, []string{"channel"})

	notificationsRetried = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "notification_retried_total",
		Help: "Total number of notification retry attempts.",
	}, []string{"channel"})

	processingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "notification_processing_duration_seconds",
		Help:    "Duration of notification processing.",
		Buckets: prometheus.DefBuckets,
	}, []string{"channel"})

	queueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "notification_queue_depth",
		Help: "Approximate number of PENDING notifications in the queue.",
	})
)

// DispatcherConfig controls the dispatcher worker pool.
type DispatcherConfig struct {
	Workers   int           // number of concurrent workers (default 5)
	BatchSize int           // notifications claimed per poll cycle (default 10)
	PollEvery time.Duration // poll interval (default 5s)
}

// Dispatcher is a pool of goroutines that claim and deliver PENDING notifications.
type Dispatcher struct {
	cfg      DispatcherConfig
	repo     repository.NotificationRepository
	tmplRepo repository.TemplateRepository
	registry *channel.Registry
	engine   *template.Engine
}

// NewDispatcher creates a Dispatcher.
func NewDispatcher(
	cfg DispatcherConfig,
	repo repository.NotificationRepository,
	tmplRepo repository.TemplateRepository,
	registry *channel.Registry,
	engine *template.Engine,
) *Dispatcher {
	if cfg.Workers <= 0 {
		cfg.Workers = 5
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.PollEvery <= 0 {
		cfg.PollEvery = 5 * time.Second
	}
	return &Dispatcher{
		cfg:      cfg,
		repo:     repo,
		tmplRepo: tmplRepo,
		registry: registry,
		engine:   engine,
	}
}

// Start launches the worker pool and blocks until ctx is done.
func (d *Dispatcher) Start(ctx context.Context) {
	slog.Info("dispatcher: starting", slog.Int("workers", d.cfg.Workers))

	work := make(chan *dao.Notification, d.cfg.Workers*2)

	var wg sync.WaitGroup
	for i := range d.cfg.Workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for n := range work {
				d.process(ctx, n)
			}
		}(i)
	}

	ticker := time.NewTicker(d.cfg.PollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(work)
			wg.Wait()
			slog.Info("dispatcher: stopped cleanly")
			return
		case <-ticker.C:
			d.poll(ctx, work)
		}
	}
}

func (d *Dispatcher) poll(ctx context.Context, work chan<- *dao.Notification) {
	notifications, err := d.repo.ClaimPending(ctx, d.cfg.BatchSize, time.Now().UTC())
	if err != nil {
		slog.ErrorContext(ctx, "dispatcher: claim pending failed", slog.String("error", err.Error()))
		return
	}
	queueDepth.Set(float64(len(notifications)))
	for _, n := range notifications {
		select {
		case work <- n:
		case <-ctx.Done():
			return
		}
	}
}

func (d *Dispatcher) process(ctx context.Context, n *dao.Notification) {
	start := time.Now()
	defer func() {
		processingDuration.WithLabelValues(n.Channel).Observe(time.Since(start).Seconds())
	}()

	slog.InfoContext(ctx, "dispatcher: processing",
		slog.String("notification_id", n.ID),
		slog.String("channel", n.Channel),
		slog.Int("retry_count", n.RetryCount),
	)

	// Resolve channel provider.
	provider, err := d.registry.Get(n.Channel)
	if err != nil {
		d.markFailed(ctx, n, fmt.Sprintf("no provider for channel %q", n.Channel))
		return
	}

	// Render body.
	body, subject, renderErr := d.render(ctx, n)
	if renderErr != nil {
		d.markFailed(ctx, n, fmt.Sprintf("render error: %s", renderErr))
		return
	}

	// Build metadata from payload.
	var metadata map[string]any
	if len(n.Payload) > 0 {
		_ = json.Unmarshal(n.Payload, &metadata)
	}

	// Send via provider.
	result, sendErr := provider.Send(ctx, &channel.SendRequest{
		Recipient: n.Recipient,
		Subject:   subject,
		Body:      body,
		Metadata:  metadata,
	})

	if sendErr != nil {
		slog.WarnContext(ctx, "dispatcher: send failed",
			slog.String("notification_id", n.ID),
			slog.String("channel", n.Channel),
			slog.Int("retry_count", n.RetryCount),
			slog.String("error", sendErr.Error()),
		)
		d.markRetryOrFailed(ctx, n, sendErr.Error())
		return
	}

	// Mark SENT.
	now := time.Now().UTC()
	providerRespJSON, _ := json.Marshal(result.ProviderResp)
	extras := map[string]any{
		"provider_ref":  result.ProviderRef,
		"provider_resp": providerRespJSON,
		"sent_at":       now,
	}
	if err := d.repo.UpdateStatus(ctx, n.ID, dao.StatusSent, extras); err != nil {
		slog.ErrorContext(ctx, "dispatcher: mark sent failed",
			slog.String("notification_id", n.ID),
			slog.String("error", err.Error()),
		)
	}

	notificationsSent.WithLabelValues(n.Channel).Inc()
	slog.InfoContext(ctx, "dispatcher: sent",
		slog.String("notification_id", n.ID),
		slog.String("channel", n.Channel),
		slog.String("provider_ref", result.ProviderRef),
	)
}

func (d *Dispatcher) render(ctx context.Context, n *dao.Notification) (body, subject string, err error) {
	// If no template_code, treat payload as pre-rendered body.
	if !n.TemplateCode.Valid || n.TemplateCode.String == "" {
		var p map[string]any
		if len(n.Payload) > 0 {
			if err = json.Unmarshal(n.Payload, &p); err != nil {
				return "", "", fmt.Errorf("unmarshal payload: %w", err)
			}
		}
		if b, ok := p["body"].(string); ok {
			body = b
		}
		if s, ok := p["subject"].(string); ok {
			subject = s
		}
		return body, subject, nil
	}

	tmpl, err := d.tmplRepo.GetByCode(ctx, n.TemplateCode.String)
	if err != nil {
		return "", "", fmt.Errorf("load template %q: %w", n.TemplateCode.String, err)
	}

	var vars map[string]any
	if len(n.TemplateVars) > 0 {
		_ = json.Unmarshal(n.TemplateVars, &vars)
	}

	body, err = d.engine.Render(tmpl.Format, tmpl.Body, vars)
	if err != nil {
		return "", "", err
	}
	subject, err = d.engine.RenderSubject(tmpl.Subject, vars)
	if err != nil {
		return "", "", err
	}
	return body, subject, nil
}

func (d *Dispatcher) markRetryOrFailed(ctx context.Context, n *dao.Notification, errMsg string) {
	nextCount := n.RetryCount + 1
	var nextStatus string
	if nextCount >= n.MaxRetries {
		nextStatus = dao.StatusFailed
		notificationsFailed.WithLabelValues(n.Channel).Inc()
	} else {
		nextStatus = dao.StatusRetrying
		notificationsRetried.WithLabelValues(n.Channel).Inc()
	}

	extras := map[string]any{
		"retry_count":   nextCount,
		"error_message": sql.NullString{String: errMsg, Valid: true},
	}
	if err := d.repo.UpdateStatus(ctx, n.ID, nextStatus, extras); err != nil {
		slog.ErrorContext(ctx, "dispatcher: mark retry/failed",
			slog.String("notification_id", n.ID),
			slog.String("error", err.Error()),
		)
	}
}

func (d *Dispatcher) markFailed(ctx context.Context, n *dao.Notification, errMsg string) {
	extras := map[string]any{
		"error_message": sql.NullString{String: errMsg, Valid: true},
	}
	if err := d.repo.UpdateStatus(ctx, n.ID, dao.StatusFailed, extras); err != nil {
		slog.ErrorContext(ctx, "dispatcher: mark failed",
			slog.String("notification_id", n.ID),
			slog.String("error", err.Error()),
		)
	}
	notificationsFailed.WithLabelValues(n.Channel).Inc()
}
