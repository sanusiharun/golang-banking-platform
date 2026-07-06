package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/sanusi/banking/pkg/httpx"
	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/payment-svc/internal/service"
)

const headerIdempotencyKey = "Idempotency-Key"

// PaymentHandler handles payment initiation and lifecycle endpoints.
type PaymentHandler struct {
	svc      service.PaymentService
	validate *validator.Validate
}

// NewPaymentHandler creates a PaymentHandler.
func NewPaymentHandler(svc service.PaymentService, validate *validator.Validate) *PaymentHandler {
	return &PaymentHandler{svc: svc, validate: validate}
}

// Transfer handles POST /v1/payments/transfer
func (h *PaymentHandler) Transfer(w http.ResponseWriter, r *http.Request) {
	idempKey := r.Header.Get(headerIdempotencyKey)
	if idempKey == "" {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "MISSING_IDEMPOTENCY_KEY", "Idempotency-Key header is required"))
		return
	}

	var req dto.TransferRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		httpx.WriteValidationError(w, r, err)
		return
	}

	initiatedBy := pkgmiddleware.UserIDFromContext(r.Context())
	resp, err := h.svc.InitiateTransfer(r.Context(), idempKey, &req, initiatedBy)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteCreated(w, r, resp)
}

// MerchantPayment handles POST /v1/payments/merchant
func (h *PaymentHandler) MerchantPayment(w http.ResponseWriter, r *http.Request) {
	idempKey := r.Header.Get(headerIdempotencyKey)
	if idempKey == "" {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "MISSING_IDEMPOTENCY_KEY", "Idempotency-Key header is required"))
		return
	}

	var req dto.MerchantPaymentRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		httpx.WriteValidationError(w, r, err)
		return
	}

	initiatedBy := pkgmiddleware.UserIDFromContext(r.Context())
	resp, err := h.svc.InitiateMerchantPayment(r.Context(), idempKey, &req, initiatedBy)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteCreated(w, r, resp)
}

// Fee handles POST /v1/payments/fee
func (h *PaymentHandler) Fee(w http.ResponseWriter, r *http.Request) {
	idempKey := r.Header.Get(headerIdempotencyKey)
	if idempKey == "" {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "MISSING_IDEMPOTENCY_KEY", "Idempotency-Key header is required"))
		return
	}

	var req dto.FeeRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		httpx.WriteValidationError(w, r, err)
		return
	}

	initiatedBy := pkgmiddleware.UserIDFromContext(r.Context())
	resp, err := h.svc.InitiateFee(r.Context(), idempKey, &req, initiatedBy)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteCreated(w, r, resp)
}

// Refund handles POST /v1/payments/refund
func (h *PaymentHandler) Refund(w http.ResponseWriter, r *http.Request) {
	idempKey := r.Header.Get(headerIdempotencyKey)
	if idempKey == "" {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "MISSING_IDEMPOTENCY_KEY", "Idempotency-Key header is required"))
		return
	}

	var req dto.RefundRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		httpx.WriteValidationError(w, r, err)
		return
	}

	initiatedBy := pkgmiddleware.UserIDFromContext(r.Context())
	resp, err := h.svc.InitiateRefund(r.Context(), idempKey, &req, initiatedBy)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteCreated(w, r, resp)
}

// Reverse handles POST /v1/payments/{id}/reverse
func (h *PaymentHandler) Reverse(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	initiatedBy := pkgmiddleware.UserIDFromContext(r.Context())
	resp, err := h.svc.Reverse(r.Context(), id, initiatedBy)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// Cancel handles POST /v1/payments/{id}/cancel
func (h *PaymentHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	initiatedBy := pkgmiddleware.UserIDFromContext(r.Context())
	resp, err := h.svc.Cancel(r.Context(), id, initiatedBy)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// Retry handles POST /v1/payments/{id}/retry
func (h *PaymentHandler) Retry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	initiatedBy := pkgmiddleware.UserIDFromContext(r.Context())
	resp, err := h.svc.Retry(r.Context(), id, initiatedBy)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}
