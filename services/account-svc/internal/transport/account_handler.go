// Package transport contains HTTP handlers for account-svc.
// Handlers are thin adapters: decode request → call service → encode response.
package transport

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

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
func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "GetAccount")
	defer span.End()

	resp, err := h.svc.GetAccount(ctx, chi.URLParam(r, "id"))
	if err != nil {
		observability.RecordError(ctx, err)
		writeServiceError(w, r, err)
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
func (h *AccountHandler) Credit(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Credit")
	defer span.End()

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
func (h *AccountHandler) Debit(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Debit")
	defer span.End()

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
