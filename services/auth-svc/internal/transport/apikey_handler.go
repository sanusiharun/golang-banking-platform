package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/sanusi/banking/pkg/httpx"
	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/auth-svc/internal/services"
)

// APIKeyHandler handles service account and API key management endpoints.
// All routes require admin-level JWT auth (not API key auth — bootstrapping concern).
type APIKeyHandler struct {
	svc      services.APIKeyService
	validate *validator.Validate
}

func NewAPIKeyHandler(svc services.APIKeyService, validate *validator.Validate) *APIKeyHandler {
	return &APIKeyHandler{svc: svc, validate: validate}
}

// ── Service Account endpoints ─────────────────────────────────────────────────

// CreateServiceAccount POST /internal/service-accounts
func (h *APIKeyHandler) CreateServiceAccount(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateServiceAccountRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	callerID := pkgmiddleware.UserIDFromContext(r.Context())
	sa, err := h.svc.CreateServiceAccount(r.Context(), &req, callerID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteCreated(w, r, sa)
}

// GetServiceAccount GET /internal/service-accounts/{id}
func (h *APIKeyHandler) GetServiceAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sa, err := h.svc.GetServiceAccount(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, sa)
}

// UpdateServiceAccount PATCH /internal/service-accounts/{id}
func (h *APIKeyHandler) UpdateServiceAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req dto.UpdateServiceAccountRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	sa, err := h.svc.UpdateServiceAccount(r.Context(), id, &req)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, sa)
}

// ListServiceAccounts GET /internal/service-accounts
func (h *APIKeyHandler) ListServiceAccounts(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	accounts, err := h.svc.ListServiceAccounts(r.Context(), tenantID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, accounts)
}

// ── API Key endpoints ─────────────────────────────────────────────────────────

// CreateAPIKey POST /internal/service-accounts/{id}/api-keys
func (h *APIKeyHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	serviceAccountID := chi.URLParam(r, "id")

	var req dto.CreateAPIKeyRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	callerID := pkgmiddleware.UserIDFromContext(r.Context())
	resp, err := h.svc.CreateAPIKey(r.Context(), serviceAccountID, &req, callerID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// 201 Created — raw key is in the body, visible only this once.
	httpx.WriteCreated(w, r, resp)
}

// ListAPIKeys GET /internal/service-accounts/{id}/api-keys
func (h *APIKeyHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	serviceAccountID := chi.URLParam(r, "id")
	keys, err := h.svc.ListAPIKeys(r.Context(), serviceAccountID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteSuccess(w, r, keys)
}

// RevokeAPIKey DELETE /internal/service-accounts/{id}/api-keys/{keyID}
func (h *APIKeyHandler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	serviceAccountID := chi.URLParam(r, "id")
	keyID := chi.URLParam(r, "keyID")

	if err := h.svc.RevokeAPIKey(r.Context(), serviceAccountID, keyID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteNoContent(w)
}

// IntrospectAPIKey POST /auth/apikey/introspect
// Called by downstream services to validate an API key hash and get the caller identity.
// Accepts a SHA-256 hex hash (never the raw key). No JWT required — hash is the credential.
func (h *APIKeyHandler) IntrospectAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Hash string `json:"hash" validate:"required,len=64"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		httpx.WriteValidationError(w, r, err)
		return
	}

	identity, err := h.svc.IntrospectAPIKey(r.Context(), req.Hash)
	if err != nil {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusUnauthorized, "INVALID_API_KEY", "invalid or expired api key"))
		return
	}
	httpx.WriteSuccess(w, r, identity)
}
