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

// TemplateHandler handles HTTP requests for /v1/templates.
type TemplateHandler struct {
	tr       *observability.ServiceTracer
	svc      services.TemplateService
	validate *validator.Validate
}

// NewTemplateHandler creates a TemplateHandler.
func NewTemplateHandler(svc services.TemplateService, validate *validator.Validate) *TemplateHandler {
	return &TemplateHandler{
		tr:       observability.NewServiceTracer("TemplateHandler"),
		svc:      svc,
		validate: validate,
	}
}

// Create handles POST /v1/templates
func (h *TemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Create")
	defer span.End()

	var req dto.CreateTemplateRequest
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
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteCreated(w, r, resp)
}

// GetByID handles GET /v1/templates/{id}
func (h *TemplateHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "GetByID")
	defer span.End()

	resp, err := h.svc.GetByID(ctx, chi.URLParam(r, "id"))
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// List handles GET /v1/templates
func (h *TemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "List")
	defer span.End()

	page, pageSize := httpx.PaginationParams(r, 50)

	var active *bool
	if v := r.URL.Query().Get("active"); v == "true" {
		t := true
		active = &t
	} else if v == "false" {
		f := false
		active = &f
	}

	filter := dto.ListTemplatesFilter{
		Channel:  r.URL.Query().Get("channel"),
		Code:     r.URL.Query().Get("code"),
		Active:   active,
		Page:     page,
		PageSize: pageSize,
	}

	resp, err := h.svc.List(ctx, filter)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// Update handles PUT /v1/templates/{id}
func (h *TemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Update")
	defer span.End()

	var req dto.UpdateTemplateRequest
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

	resp, err := h.svc.Update(ctx, chi.URLParam(r, "id"), &req)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// Delete handles DELETE /v1/templates/{id}
func (h *TemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Delete")
	defer span.End()

	if err := h.svc.Delete(ctx, chi.URLParam(r, "id")); err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteNoContent(w)
}

// Preview handles POST /v1/templates/{id}/preview
func (h *TemplateHandler) Preview(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Preview")
	defer span.End()

	var req dto.PreviewTemplateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}

	resp, err := h.svc.Preview(ctx, chi.URLParam(r, "id"), &req)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}
