package transport

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	pkgaudit "github.com/sanusi/banking/pkg/audit"
	pkgerrors "github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/pkg/httpx"
	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/audit-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/audit-svc/internal/repository/postgres"
	"github.com/sanusi/banking/services/audit-svc/internal/services"
)

// AuditHandler handles HTTP requests for audit-svc.
type AuditHandler struct {
	tr       *observability.ServiceTracer
	svc      services.AuditService
	validate *validator.Validate
}

// NewAuditHandler creates a new AuditHandler.
func NewAuditHandler(svc services.AuditService, validate *validator.Validate) *AuditHandler {
	return &AuditHandler{
		tr:       observability.NewServiceTracer("AuditHandler"),
		svc:      svc,
		validate: validate,
	}
}

// IngestEvent handles POST /v1/audit/events — sync HTTP ingest path.
// Use this only when the caller requires durability confirmation before proceeding.
// For fire-and-forget, use the NATS path instead.
func (h *AuditHandler) IngestEvent(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "IngestEvent")
	defer span.End()

	var req dto.IngestRequest
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

	event := pkgaudit.AuditEvent{
		ActorType:   req.ActorType,
		ActorID:     req.ActorID,
		ActorEmail:  req.ActorEmail,
		Action:      req.Action,
		Status:      req.Status,
		Resource:    req.Resource,
		ResourceID:  req.ResourceID,
		ServiceName: req.ServiceName,
		TraceID:     req.TraceID,
		IPAddress:   req.IPAddress,
		UserAgent:   req.UserAgent,
		Metadata:    req.Metadata,
	}

	if err := h.svc.Ingest(ctx, event); err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteHTTPError(w, r, httpx.ErrInternal)
		return
	}

	httpx.WriteCreated(w, r, map[string]string{"status": "ingested"})
}

// GetEvent handles GET /v1/audit/events/:id — single event lookup.
func (h *AuditHandler) GetEvent(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "GetEvent")
	defer span.End()

	id := chi.URLParam(r, "id")
	event, err := h.svc.GetByID(ctx, id)
	if err != nil {
		observability.RecordError(ctx, err)
		if errors.Is(err, postgres.ErrNotFound) || pkgerrors.IsNotFound(err) {
			httpx.WriteHTTPError(w, r, httpx.ErrNotFound)
			return
		}
		httpx.WriteHTTPError(w, r, httpx.ErrInternal)
		return
	}
	httpx.WriteSuccess(w, r, event)
}

// ListEvents handles GET /v1/audit/events — filtered, paginated list.
func (h *AuditHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "ListEvents")
	defer span.End()

	params := parseQueryParams(r)
	result, err := h.svc.List(ctx, params)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteHTTPError(w, r, httpx.ErrInternal)
		return
	}
	httpx.WriteSuccess(w, r, result)
}

// ListActorEvents handles GET /v1/audit/actors/:actor_id/events
func (h *AuditHandler) ListActorEvents(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "ListActorEvents")
	defer span.End()

	params := parseQueryParams(r)
	params.ActorID = chi.URLParam(r, "actor_id")

	result, err := h.svc.List(ctx, params)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteHTTPError(w, r, httpx.ErrInternal)
		return
	}
	httpx.WriteSuccess(w, r, result)
}

// ListResourceEvents handles GET /v1/audit/resources/:resource/:resource_id/events
func (h *AuditHandler) ListResourceEvents(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "ListResourceEvents")
	defer span.End()

	params := parseQueryParams(r)
	params.Resource = chi.URLParam(r, "resource")
	params.ResourceID = chi.URLParam(r, "resource_id")

	result, err := h.svc.List(ctx, params)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteHTTPError(w, r, httpx.ErrInternal)
		return
	}
	httpx.WriteSuccess(w, r, result)
}

// ── query param parsing ───────────────────────────────────────────────────────

func parseQueryParams(r *http.Request) dto.QueryParams {
	q := r.URL.Query()
	params := dto.QueryParams{
		ActorID:     q.Get("actor_id"),
		Action:      q.Get("action"),
		Status:      q.Get("status"),
		ServiceName: q.Get("service"),
		TraceID:     q.Get("trace_id"),
		Cursor:      q.Get("cursor"),
		Limit:       httpx.QueryInt(r, "limit", 50),
	}

	if from := q.Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			params.From = &t
		}
	}
	if to := q.Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			params.To = &t
		}
	}
	return params
}
