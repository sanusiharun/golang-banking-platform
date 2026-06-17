package dto

import "time"

// CreateTemplateRequest is the body for POST /v1/templates.
type CreateTemplateRequest struct {
	Code      string         `json:"code" validate:"required,alphanum"`
	Name      string         `json:"name" validate:"required"`
	Channel   string         `json:"channel" validate:"required,oneof=EMAIL SMS PUSH WHATSAPP WEBHOOK"`
	Format    string         `json:"format" validate:"required,oneof=TEXT HTML"`
	Subject   string         `json:"subject"`
	Body      string         `json:"body" validate:"required"`
	Variables map[string]any `json:"variables"`
}

// UpdateTemplateRequest is the body for PUT /v1/templates/{id}.
type UpdateTemplateRequest struct {
	Name      string         `json:"name"`
	Subject   string         `json:"subject"`
	Body      string         `json:"body" validate:"required"`
	Variables map[string]any `json:"variables"`
}

// PreviewTemplateRequest is the body for POST /v1/templates/{id}/preview.
type PreviewTemplateRequest struct {
	Variables map[string]any `json:"variables"`
}

// TemplateResponse is returned for all template API endpoints.
type TemplateResponse struct {
	ID        string         `json:"id"`
	Code      string         `json:"code"`
	Name      string         `json:"name"`
	Channel   string         `json:"channel"`
	Format    string         `json:"format"`
	Subject   string         `json:"subject,omitempty"`
	Body      string         `json:"body"`
	Variables map[string]any `json:"variables,omitempty"`
	Version   int            `json:"version"`
	Active    bool           `json:"active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// PreviewTemplateResponse is returned by the preview endpoint.
type PreviewTemplateResponse struct {
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body"`
	Format  string `json:"format"`
}

// ListTemplatesFilter holds query parameters for GET /v1/templates.
type ListTemplatesFilter struct {
	Channel  string
	Code     string
	Active   *bool
	Page     int
	PageSize int
}

// PaginatedTemplatesResponse wraps a paginated list of templates.
type PaginatedTemplatesResponse struct {
	Items      []*TemplateResponse `json:"items"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"page_size"`
	TotalCount int64               `json:"total_count"`
	TotalPages int                 `json:"total_pages"`
}
