// Package transport contains HTTP handlers and NATS consumer for notification-svc.
// Handlers are thin adapters: decode request → call service → encode response.
package transport

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/sanusi/banking/pkg/httpx"
	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/notification-svc/internal/services"
)

// NotificationHandler handles HTTP requests for /v1/notifications.
type NotificationHandler struct {
	tr       *observability.ServiceTracer
	svc      services.NotificationService
	validate *validator.Validate
}

// NewNotificationHandler creates a NotificationHandler.
func NewNotificationHandler(svc services.NotificationService, validate *validator.Validate) *NotificationHandler {
	return &NotificationHandler{
		tr:       observability.NewServiceTracer("NotificationHandler"),
		svc:      svc,
		validate: validate,
	}
}

// Send handles POST /v1/notifications
func (h *NotificationHandler) Send(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Send")
	defer span.End()

	var req dto.SendNotificationRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteValidationError(w, r, err)
		return
	}

	resp, err := h.svc.Send(ctx, &req)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteCreated(w, r, resp)
}

// GetByID handles GET /v1/notifications/{id}
func (h *NotificationHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "GetByID")
	defer span.End()

	id := chi.URLParam(r, "id")
	resp, err := h.svc.GetByID(ctx, id)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// List handles GET /v1/notifications
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "List")
	defer span.End()

	page, pageSize := httpx.PaginationParams(r, 50)
	filter := dto.ListNotificationsFilter{
		Status:       r.URL.Query().Get("status"),
		Channel:      r.URL.Query().Get("channel"),
		Recipient:    r.URL.Query().Get("recipient"),
		TemplateCode: r.URL.Query().Get("template_code"),
		ScheduleID:   r.URL.Query().Get("schedule_id"),
		Page:         page,
		PageSize:     pageSize,
	}
	if from := r.URL.Query().Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			filter.From = &t
		}
	}
	if to := r.URL.Query().Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			filter.To = &t
		}
	}

	resp, err := h.svc.List(ctx, filter)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// Retry handles POST /v1/notifications/{id}/retry
func (h *NotificationHandler) Retry(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Retry")
	defer span.End()

	id := chi.URLParam(r, "id")
	resp, err := h.svc.Retry(ctx, id)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// Cancel handles POST /v1/notifications/{id}/cancel
func (h *NotificationHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Cancel")
	defer span.End()

	id := chi.URLParam(r, "id")
	resp, err := h.svc.Cancel(ctx, id)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}
