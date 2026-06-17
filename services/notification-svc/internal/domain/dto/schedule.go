package dto

import "time"

// CreateScheduleRequest is the body for POST /v1/schedules.
type CreateScheduleRequest struct {
	Name         string         `json:"name" validate:"required"`
	Description  string         `json:"description"`
	Channel      string         `json:"channel" validate:"required,oneof=EMAIL SMS PUSH WHATSAPP WEBHOOK"`
	TemplateCode string         `json:"template_code" validate:"required"`
	Recipient    string         `json:"recipient" validate:"required"`
	TemplateVars map[string]any `json:"template_vars"`
	CronExpr     string         `json:"cron_expr"`
	ScheduledAt  *time.Time     `json:"scheduled_at"`
	Recurring    bool           `json:"recurring"`
}

// UpdateScheduleRequest is the body for PUT /v1/schedules/{id}.
type UpdateScheduleRequest struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	TemplateCode string         `json:"template_code"`
	Recipient    string         `json:"recipient"`
	TemplateVars map[string]any `json:"template_vars"`
	CronExpr     string         `json:"cron_expr"`
	ScheduledAt  *time.Time     `json:"scheduled_at"`
}

// ScheduleResponse is returned for all schedule API endpoints.
type ScheduleResponse struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	Channel      string         `json:"channel"`
	TemplateCode string         `json:"template_code"`
	Recipient    string         `json:"recipient"`
	TemplateVars map[string]any `json:"template_vars,omitempty"`
	CronExpr     string         `json:"cron_expr,omitempty"`
	ScheduledAt  *time.Time     `json:"scheduled_at,omitempty"`
	Recurring    bool           `json:"recurring"`
	Enabled      bool           `json:"enabled"`
	LastRunAt    *time.Time     `json:"last_run_at,omitempty"`
	NextRunAt    *time.Time     `json:"next_run_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// ListSchedulesFilter holds query parameters for GET /v1/schedules.
type ListSchedulesFilter struct {
	Channel   string
	Enabled   *bool
	Recurring *bool
	Page      int
	PageSize  int
}

// PaginatedSchedulesResponse wraps a paginated list of schedules.
type PaginatedSchedulesResponse struct {
	Items      []*ScheduleResponse `json:"items"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"page_size"`
	TotalCount int64               `json:"total_count"`
	TotalPages int                 `json:"total_pages"`
}
