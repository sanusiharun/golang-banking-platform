// Package transport contains HTTP handlers for auth-svc.
package transport

import (
	"errors"
	"net/http"

	"github.com/go-playground/validator/v10"

	"github.com/sanusi/banking/services/auth-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/auth-svc/internal/services"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	svc      services.AuthService
	validate *validator.Validate
}

func NewAuthHandler(svc services.AuthService, validate *validator.Validate) *AuthHandler {
	return &AuthHandler{svc: svc, validate: validate}
}

// Login handles POST /auth/login.
//
// Request:  {"username":"admin","password":"Admin@12345"}
// Response: {"success":true,"data":{"access_token":"...","refresh_token":"...","access_token_expires_at":"...","refresh_token_expires_at":"..."}}
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		writeValidationError(w, err)
		return
	}

	resp, err := h.svc.Login(r.Context(), &req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// Refresh handles POST /auth/refresh.
// Validates the refresh token, revokes it, and issues a new token pair (rotation).
//
// Request:  {"refresh_token":"<uuid>"}
// Response: same shape as Login
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req dto.RefreshRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		writeValidationError(w, err)
		return
	}

	resp, err := h.svc.Refresh(r.Context(), &req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidToken) {
			writeError(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// Logout handles POST /auth/logout.
// Revokes the refresh token. The access token expires on its own (short TTL).
//
// Request:  {"refresh_token":"<uuid>"}
// Response: {"success":true,"data":{"message":"logged out"}}
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req dto.LogoutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		writeValidationError(w, err)
		return
	}

	if err := h.svc.Logout(r.Context(), &req); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}
