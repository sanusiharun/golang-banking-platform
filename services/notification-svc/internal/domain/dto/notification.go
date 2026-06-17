package dto

import "time"

// SendNotificationRequest is the body for POST /v1/notifications.
type SendNotificationRequest struct {
	Channel        string         `json:"channel" validate:"required,oneof=EMAIL SMS PUSH WHATSAPP WEBHOOK"`
	Recipient      string         `json:"recipient" validate:"required"`
	TemplateCode   string         `json:"template_code"`
	TemplateVars   map[string]any `json:"template_vars"`
	Payload        map[string]any `json:"payload"`
	Subject        string         `json:"subject"`
	Body           string         `json:"body"`
	MaxRetries     int            `json:"max_retries" validate:"min=0,max=10"`
	IdempotencyKey string         `json:"idempotency_key"`
	ScheduledAt    *time.Time     `json:"scheduled_at"`
	ScheduleID     string         `json:"schedule_id"`
}

// NotificationResponse is returned for all notification API endpoints.
type NotificationResponse struct {
	ID             string         `json:"id"`
	Channel        string         `json:"channel"`
	Recipient      string         `json:"recipient"`
	TemplateID     string         `json:"template_id,omitempty"`
	TemplateCode   string         `json:"template_code,omitempty"`
	TemplateVars   map[string]any `json:"template_vars,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
	Status         string         `json:"status"`
	ProviderRef    string         `json:"provider_ref,omitempty"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	RetryCount     int            `json:"retry_count"`
	MaxRetries     int            `json:"max_retries"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	ScheduleID     string         `json:"schedule_id,omitempty"`
	ScheduledAt    *time.Time     `json:"scheduled_at,omitempty"`
	SentAt         *time.Time     `json:"sent_at,omitempty"`
	DeliveredAt    *time.Time     `json:"delivered_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// ListNotificationsFilter holds query parameters for GET /v1/notifications.
type ListNotificationsFilter struct {
	Status       string
	Channel      string
	Recipient    string
	TemplateCode string
	ScheduleID   string
	From         *time.Time
	To           *time.Time
	Page         int
	PageSize     int
}

// PaginatedNotificationsResponse wraps a paginated list of notifications.
type PaginatedNotificationsResponse struct {
	Items      []*NotificationResponse `json:"items"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"page_size"`
	TotalCount int64                   `json:"total_count"`
	TotalPages int                     `json:"total_pages"`
}
