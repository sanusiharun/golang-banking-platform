package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/sanusi/banking/pkg/httpx"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/payment-svc/internal/service"
)

// InquiryHandler handles transaction lookup and listing endpoints.
type InquiryHandler struct {
	svc service.PaymentService
}

// NewInquiryHandler creates an InquiryHandler.
func NewInquiryHandler(svc service.PaymentService) *InquiryHandler {
	return &InquiryHandler{svc: svc}
}

// GetByID handles GET /v1/payments/{id}
func (h *InquiryHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	resp, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writePaymentError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

// List handles GET /v1/payments?account_id=&status=&limit=&offset=
func (h *InquiryHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := dto.ListFilter{
		AccountID: q.Get("account_id"),
		Status:    q.Get("status"),
		Limit:     parseIntQuery(q.Get("limit"), 20),
		Offset:    parseIntQuery(q.Get("offset"), 0),
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	resp, err := h.svc.List(r.Context(), filter)
	if err != nil {
		writePaymentError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, resp)
}

func parseIntQuery(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}
