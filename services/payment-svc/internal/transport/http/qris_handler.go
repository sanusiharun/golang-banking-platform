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

// QRISHandler handles QRIS merchant, QR generation/decoding, and payment endpoints.
type QRISHandler struct {
	svc      service.QRISService
	validate *validator.Validate
}

// NewQRISHandler creates a QRISHandler.
func NewQRISHandler(svc service.QRISService, validate *validator.Validate) *QRISHandler {
	return &QRISHandler{svc: svc, validate: validate}
}

// RegisterMerchant handles POST /v1/merchants
func (h *QRISHandler) RegisterMerchant(w http.ResponseWriter, r *http.Request) {
	var req dto.MerchantRegisterRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		httpx.WriteValidationError(w, r, err)
		return
	}

	resp, err := h.svc.RegisterMerchant(r.Context(), &req)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteCreated(w, r, resp)
}

// GetMerchant handles GET /v1/merchants/{id}
func (h *QRISHandler) GetMerchant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	resp, err := h.svc.GetMerchant(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// Generate handles POST /v1/payments/qris/generate
func (h *QRISHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req dto.QRISGenerateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		httpx.WriteValidationError(w, r, err)
		return
	}

	resp, err := h.svc.GenerateCharge(r.Context(), &req)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteCreated(w, r, resp)
}

// Decode handles POST /v1/payments/qris/decode
func (h *QRISHandler) Decode(w http.ResponseWriter, r *http.Request) {
	var req dto.QRISDecodeRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		httpx.WriteValidationError(w, r, err)
		return
	}

	resp, err := h.svc.Decode(r.Context(), req.QRString)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// Pay handles POST /v1/payments/qris/pay
func (h *QRISHandler) Pay(w http.ResponseWriter, r *http.Request) {
	idempKey := r.Header.Get(headerIdempotencyKey)
	if idempKey == "" {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "MISSING_IDEMPOTENCY_KEY", "Idempotency-Key header is required"))
		return
	}

	var req dto.QRISPayRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		httpx.WriteValidationError(w, r, err)
		return
	}

	initiatedBy := pkgmiddleware.UserIDFromContext(r.Context())
	resp, err := h.svc.Pay(r.Context(), idempKey, &req, initiatedBy)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteCreated(w, r, resp)
}
