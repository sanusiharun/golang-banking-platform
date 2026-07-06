// Package transport contains HTTP handlers for account-svc.
// Handlers are thin adapters: decode request → call service → encode response.
package transport

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	pkgaudit "github.com/sanusi/banking/pkg/audit"
	"github.com/sanusi/banking/pkg/featureflag"
	"github.com/sanusi/banking/pkg/httpx"
	"github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/account-svc/internal/client/authclient"
	"github.com/sanusi/banking/services/account-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/account-svc/internal/services"
)

// AccountHandler holds dependencies for account HTTP handlers.
type AccountHandler struct {
	authClient *authclient.Client

	tr       *observability.ServiceTracer
	svc      services.AccountService
	validate *validator.Validate
	audit    pkgaudit.Publisher
}

func NewAccountHandler(svc services.AccountService, validate *validator.Validate, authClient *authclient.Client, audit pkgaudit.Publisher) *AccountHandler {
	return &AccountHandler{
		tr:         observability.NewServiceTracer("AccountHandler"),
		svc:        svc,
		validate:   validate,
		authClient: authClient,
		audit:      audit,
	}
}

// CreateAccount handles POST /v1/accounts
func (h *AccountHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "CreateAccount")
	defer span.End()

	// RequireRole on this route enforces ADMIN|TELLER before we get here.
	if _, ok := middleware.ClaimsFromContext(ctx); !ok {
		httpx.WriteHTTPError(w, r, httpx.ErrUnauthorized)
		return
	}

	var req dto.CreateAccountRequest
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
	resp, err := h.svc.CreateAccount(ctx, &req)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}

	callerID := middleware.UserIDFromContext(ctx)
	_ = h.audit.Publish(r.Context(), pkgaudit.AuditEvent{
		ActorType:   pkgaudit.ActorTypeUser,
		ActorID:     callerID,
		Action:      pkgaudit.ActionAccountCreated,
		Status:      pkgaudit.StatusSuccess,
		Resource:    "account",
		ResourceID:  resp.ID,
		ServiceName: "account-svc",
		IPAddress:   r.RemoteAddr,
		UserAgent:   r.UserAgent(),
		TraceID:     span.SpanContext().TraceID().String(),
	})

	httpx.WriteCreated(w, r, resp)
}

// GetAccount handles GET /v1/accounts/{id}
//
// Feature flag: "show_account_metadata"
//
//	enabled  → response includes metadata field with extra info
//	disabled → standard response (default)
func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "GetAccount")
	defer span.End()

	resp, err := h.svc.GetAccount(ctx, chi.URLParam(r, "id"))
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}

	// Feature flag: enrich response with metadata when flag is enabled.
	// Toggle in Flipt UI → http://localhost:8085 → flag key: show_account_metadata
	if featureflag.IsEnabled(ctx, "show_account_metadata", false) {
		httpx.WriteSuccess(w, r, map[string]any{
			"account": resp,
			"metadata": map[string]any{
				"feature_flags": map[string]bool{
					"show_account_metadata": true,
				},
				"api_version": "v2",
			},
		})
		return
	}

	httpx.WriteSuccess(w, r, resp)
}

// GetBalance handles GET /v1/accounts/{id}/balance
func (h *AccountHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "GetBalance")
	defer span.End()

	accountID := chi.URLParam(r, "id")
	resp, err := h.svc.GetBalance(ctx, accountID)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}

	callerID := middleware.UserIDFromContext(ctx)
	_ = h.audit.Publish(r.Context(), pkgaudit.AuditEvent{
		ActorType:   pkgaudit.ActorTypeUser,
		ActorID:     callerID,
		Action:      pkgaudit.ActionAccountBalanceRead,
		Status:      pkgaudit.StatusSuccess,
		Resource:    "account",
		ResourceID:  accountID,
		ServiceName: "account-svc",
		IPAddress:   r.RemoteAddr,
		UserAgent:   r.UserAgent(),
		TraceID:     span.SpanContext().TraceID().String(),
	})

	httpx.WriteSuccess(w, r, resp)
}

// Credit handles POST /v1/accounts/{id}/credit
//
// Controlled by Flipt flag: "banking_operation_hours" (string variant, e.g. "07:00-15:00")
// Requests outside the configured window are rejected with 403.
func (h *AccountHandler) Credit(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Credit")
	defer span.End()

	if err := h.checkOperationHours(ctx, w, r); err != nil {
		return
	}

	var req dto.CreditRequest
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
	accountID := chi.URLParam(r, "id")
	resp, err := h.svc.Credit(ctx, accountID, &req)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}

	callerID := middleware.UserIDFromContext(ctx)
	_ = h.audit.Publish(r.Context(), pkgaudit.AuditEvent{
		ActorType:   pkgaudit.ActorTypeUser,
		ActorID:     callerID,
		Action:      pkgaudit.ActionAccountCredit,
		Status:      pkgaudit.StatusSuccess,
		Resource:    "account",
		ResourceID:  accountID,
		ServiceName: "account-svc",
		IPAddress:   r.RemoteAddr,
		UserAgent:   r.UserAgent(),
		TraceID:     span.SpanContext().TraceID().String(),
		Metadata:    map[string]any{"amount": req.Amount},
	})

	httpx.WriteSuccess(w, r, resp)
}

// Debit handles POST /v1/accounts/{id}/debit
//
// Controlled by Flipt flag: "banking_operation_hours" (string variant, e.g. "07:00-15:00")
// Requests outside the configured window are rejected with 403.
func (h *AccountHandler) Debit(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Debit")
	defer span.End()

	if err := h.checkOperationHours(ctx, w, r); err != nil {
		return
	}

	var req dto.DebitRequest
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
	accountID := chi.URLParam(r, "id")
	resp, err := h.svc.Debit(ctx, accountID, &req)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}

	callerID := middleware.UserIDFromContext(ctx)
	_ = h.audit.Publish(r.Context(), pkgaudit.AuditEvent{
		ActorType:   pkgaudit.ActorTypeUser,
		ActorID:     callerID,
		Action:      pkgaudit.ActionAccountDebit,
		Status:      pkgaudit.StatusSuccess,
		Resource:    "account",
		ResourceID:  accountID,
		ServiceName: "account-svc",
		IPAddress:   r.RemoteAddr,
		UserAgent:   r.UserAgent(),
		TraceID:     span.SpanContext().TraceID().String(),
		Metadata:    map[string]any{"amount": req.Amount},
	})

	httpx.WriteSuccess(w, r, resp)
}

// ListAccounts handles GET /v1/accounts
func (h *AccountHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "ListAccounts")
	defer span.End()

	page, pageSize := httpx.PaginationParams(r, 100)
	customerID := r.URL.Query().Get("customer_id")

	resp, err := h.svc.ListAccounts(ctx, customerID, page, pageSize)
	if err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// checkOperationHours reads the banking_operation_hours flag from Flipt and
// rejects the request with 403 if the current time is outside the window.
// Returns nil if the operation is allowed, non-nil if the response was already written.
func (h *AccountHandler) checkOperationHours(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	window := featureflag.GetString(ctx, "banking_operation_hours", "")
	ok, err := featureflag.IsWithinOperationHours(window, time.UTC)
	if err != nil {
		// Bad config — log and allow through (don't block on misconfiguration)
		return nil
	}
	if !ok {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(
			http.StatusForbidden,
			"OUTSIDE_OPERATION_HOURS",
			"debit/credit operations are only allowed during banking hours: "+window,
		))
		return fmt.Errorf("outside operation hours")
	}
	return nil
}
