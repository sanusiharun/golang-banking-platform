package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/sanusi/banking/pkg/httpx"
	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/notification-svc/internal/services"
)

// ScheduleHandler handles HTTP requests for /v1/schedules.
type ScheduleHandler struct {
	tr       *observability.ServiceTracer
	svc      services.SchedulerService
	validate *validator.Validate
}

// NewScheduleHandler creates a ScheduleHandler.
func NewScheduleHandler(svc services.SchedulerService, validate *validator.Validate) *ScheduleHandler {
	return &ScheduleHandler{
		tr:       observability.NewServiceTracer("ScheduleHandler"),
		svc:      svc,
		validate: validate,
	}
}

// Create handles POST /v1/schedules
func (h *ScheduleHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Create")
	defer span.End()

	var req dto.CreateScheduleRequest
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

	resp, err := h.svc.Create(ctx, &req)
	if err != nil {
		observability.RecordError(ctx, err)
		writeError(w, r, err)
		return
	}
	httpx.WriteCreated(w, r, resp)
}

// GetByID handles GET /v1/schedules/{id}
func (h *ScheduleHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "GetByID")
	defer span.End()

	resp, err := h.svc.GetByID(ctx, chi.URLParam(r, "id"))
	if err != nil {
		observability.RecordError(ctx, err)
		writeError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// List handles GET /v1/schedules
func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "List")
	defer span.End()

	page, pageSize := httpx.PaginationParams(r, 50)

	var enabled *bool
	if v := r.URL.Query().Get("enabled"); v == "true" {
		t := true
		enabled = &t
	} else if v == "false" {
		f := false
		enabled = &f
	}

	var recurring *bool
	if v := r.URL.Query().Get("recurring"); v == "true" {
		t := true
		recurring = &t
	} else if v == "false" {
		f := false
		recurring = &f
	}

	filter := dto.ListSchedulesFilter{
		Channel:   r.URL.Query().Get("channel"),
		Enabled:   enabled,
		Recurring: recurring,
		Page:      page,
		PageSize:  pageSize,
	}

	resp, err := h.svc.List(ctx, filter)
	if err != nil {
		observability.RecordError(ctx, err)
		writeError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// Update handles PUT /v1/schedules/{id}
func (h *ScheduleHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Update")
	defer span.End()

	var req dto.UpdateScheduleRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}

	resp, err := h.svc.Update(ctx, chi.URLParam(r, "id"), &req)
	if err != nil {
		observability.RecordError(ctx, err)
		writeError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// Delete handles DELETE /v1/schedules/{id}
func (h *ScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Delete")
	defer span.End()

	if err := h.svc.Delete(ctx, chi.URLParam(r, "id")); err != nil {
		observability.RecordError(ctx, err)
		writeError(w, r, err)
		return
	}
	httpx.WriteNoContent(w)
}

// Enable handles POST /v1/schedules/{id}/enable
func (h *ScheduleHandler) Enable(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Enable")
	defer span.End()

	resp, err := h.svc.Enable(ctx, chi.URLParam(r, "id"))
	if err != nil {
		observability.RecordError(ctx, err)
		writeError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// Disable handles POST /v1/schedules/{id}/disable
func (h *ScheduleHandler) Disable(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Disable")
	defer span.End()

	resp, err := h.svc.Disable(ctx, chi.URLParam(r, "id"))
	if err != nil {
		observability.RecordError(ctx, err)
		writeError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}
