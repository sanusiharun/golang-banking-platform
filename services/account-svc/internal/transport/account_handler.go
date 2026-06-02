// Package transport contains HTTP handlers for account-svc.
// Handlers are thin adapters: decode request → call service → encode response.
package transport

import (
	"context"
	"fmt"
	"time"

	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/sanusi/banking/pkg/featureflag"
	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/account-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/account-svc/internal/services"
)

// AccountHandler holds dependencies for account HTTP handlers.
type AccountHandler struct {

	tr       *observability.ServiceTracer
	svc      services.AccountService
	validate *validator.Validate
}

func NewAccountHandler(svc services.AccountService, validate *validator.Validate) *AccountHandler {
	return &AccountHandler{
		tr:       observability.NewServiceTracer("AccountHandler"),
		svc:      svc,
		validate: validate,
	}
}

// CreateAccount handles POST /v1/accounts
func (h *AccountHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "CreateAccount")
	defer span.End()

	var req dto.CreateAccountRequest
	if err := decodeJSON(r, &req); err != nil {
		observability.RecordError(ctx, err)
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		observability.RecordError(ctx, err)
		writeValidationError(w, err)
		return
	}
	resp, err := h.svc.CreateAccount(ctx, &req)
	if err != nil {
		observability.RecordError(ctx, err)
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

// GetAccount handles GET /v1/accounts/{id}
//
// Feature flag: "show_account_metadata"
//   enabled  → response includes metadata field with extra info
//   disabled → standard response (default)
func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "GetAccount")
	defer span.End()

	resp, err := h.svc.GetAccount(ctx, chi.URLParam(r, "id"))
	if err != nil {
		observability.RecordError(ctx, err)
		writeServiceError(w, r, err)
		return
	}

	// Feature flag: enrich response with metadata when flag is enabled.
	// Toggle in Flipt UI → http://localhost:8082 → flag key: show_account_metadata
	if featureflag.IsEnabled(ctx, "show_account_metadata", false) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": resp,
			"metadata": map[string]any{
				"feature_flags": map[string]bool{
					"show_account_metadata": true,
				},
				"api_version": "v2",
			},
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetBalance handles GET /v1/accounts/{id}/balance
func (h *AccountHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "GetBalance")
	defer span.End()

	resp, err := h.svc.GetBalance(ctx, chi.URLParam(r, "id"))
	if err != nil {
		observability.RecordError(ctx, err)
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// Credit handles POST /v1/accounts/{id}/credit
//
// Controlled by Flipt flag: "banking_operation_hours" (string variant, e.g. "07:00-15:00")
// Requests outside the configured window are rejected with 403.
func (h *AccountHandler) Credit(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Credit")
	defer span.End()

	if err := h.checkOperationHours(ctx, w); err != nil {
		return
	}

	var req dto.CreditRequest
	if err := decodeJSON(r, &req); err != nil {
		observability.RecordError(ctx, err)
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		observability.RecordError(ctx, err)
		writeValidationError(w, err)
		return
	}
	resp, err := h.svc.Credit(ctx, chi.URLParam(r, "id"), &req)
	if err != nil {
		observability.RecordError(ctx, err)
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// Debit handles POST /v1/accounts/{id}/debit
//
// Controlled by Flipt flag: "banking_operation_hours" (string variant, e.g. "07:00-15:00")
// Requests outside the configured window are rejected with 403.
func (h *AccountHandler) Debit(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Debit")
	defer span.End()

	if err := h.checkOperationHours(ctx, w); err != nil {
		return
	}

	var req dto.DebitRequest
	if err := decodeJSON(r, &req); err != nil {
		observability.RecordError(ctx, err)
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		observability.RecordError(ctx, err)
		writeValidationError(w, err)
		return
	}
	resp, err := h.svc.Debit(ctx, chi.URLParam(r, "id"), &req)
	if err != nil {
		observability.RecordError(ctx, err)
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// ListAccounts handles GET /v1/accounts
func (h *AccountHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "ListAccounts")
	defer span.End()

	customerID := r.URL.Query().Get("customer_id")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	resp, err := h.svc.ListAccounts(ctx, customerID, page, pageSize)
	if err != nil {
		observability.RecordError(ctx, err)
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// checkOperationHours reads the banking_operation_hours flag from Flipt and
// rejects the request with 403 if the current time is outside the window.
// Returns nil if the operation is allowed, non-nil if the response was already written.
func (h *AccountHandler) checkOperationHours(ctx context.Context, w http.ResponseWriter) error {
	window := featureflag.GetString(ctx, "banking_operation_hours", "")
	ok, err := featureflag.IsWithinOperationHours(window, time.UTC)
	if err != nil {
		// Bad config — log and allow through (don't block on misconfiguration)
		return nil
	}
	if !ok {
		writeError(w, http.StatusForbidden, "OUTSIDE_OPERATION_HOURS",
			"debit/credit operations are only allowed during banking hours: "+window)
		return fmt.Errorf("outside operation hours")
	}
	return nil
}
