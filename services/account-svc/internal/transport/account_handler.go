// Package transport contains HTTP handlers for account-svc.
// Handlers are thin adapters: decode request → call service → encode response.
// Uses global slog — no logger constructor needed.
// Response helpers (writeJSON, writeError, etc.) live in response.go.
package transport

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/sanusi/banking/services/account-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/account-svc/internal/services"
)

// AccountHandler holds dependencies for account HTTP handlers.
type AccountHandler struct {
	svc      services.AccountService
	validate *validator.Validate
}

// NewAccountHandler creates the handler. No logger param — uses global slog.
func NewAccountHandler(svc services.AccountService, validate *validator.Validate) *AccountHandler {
	return &AccountHandler{svc: svc, validate: validate}
}

// CreateAccount handles POST /v1/accounts
func (h *AccountHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateAccountRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		writeValidationError(w, err)
		return
	}
	resp, err := h.svc.CreateAccount(r.Context(), &req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

// GetAccount handles GET /v1/accounts/{id}
func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.GetAccount(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetBalance handles GET /v1/accounts/{id}/balance
func (h *AccountHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.GetBalance(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// Credit handles POST /v1/accounts/{id}/credit
func (h *AccountHandler) Credit(w http.ResponseWriter, r *http.Request) {
	var req dto.CreditRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		writeValidationError(w, err)
		return
	}
	resp, err := h.svc.Credit(r.Context(), chi.URLParam(r, "id"), &req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// Debit handles POST /v1/accounts/{id}/debit
func (h *AccountHandler) Debit(w http.ResponseWriter, r *http.Request) {
	var req dto.DebitRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		writeValidationError(w, err)
		return
	}
	resp, err := h.svc.Debit(r.Context(), chi.URLParam(r, "id"), &req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// ListAccounts handles GET /v1/accounts
func (h *AccountHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	customerID := r.URL.Query().Get("customer_id")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	resp, err := h.svc.ListAccounts(r.Context(), customerID, page, pageSize)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
