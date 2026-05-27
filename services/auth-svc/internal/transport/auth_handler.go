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
// Request:
//
//	{"username":"admin","password":"Admin@12345"}
//
// Response 200:
//
//	{"success":true,"data":{"token":"<rs256-jwt>","expires_at":"...","user_id":"...","roles":[...]}}
//
// Response 401:
//
//	{"success":false,"error":{"code":"INVALID_CREDENTIALS","message":"invalid username or password"}}
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
