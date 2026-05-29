// Package transport contains HTTP handlers for auth-svc.
package transport

import (
	"errors"
	"net/http"

	"github.com/go-playground/validator/v10"

	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/auth-svc/internal/services"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	tr       *observability.ServiceTracer
	svc      services.AuthService
	validate *validator.Validate
}

func NewAuthHandler(svc services.AuthService, validate *validator.Validate) *AuthHandler {
	return &AuthHandler{
		tr:       observability.NewServiceTracer("AuthHandler"),
		svc:      svc,
		validate: validate,
	}
}

// Login handles POST /auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Login")
	defer span.End()

	var req dto.LoginRequest
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

	resp, err := h.svc.Login(ctx, &req)
	if err != nil {
		observability.RecordError(ctx, err)
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
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Refresh")
	defer span.End()

	var req dto.RefreshRequest
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

	resp, err := h.svc.Refresh(ctx, &req)
	if err != nil {
		observability.RecordError(ctx, err)
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
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Logout")
	defer span.End()

	var req dto.LogoutRequest
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

	if err := h.svc.Logout(ctx, &req); err != nil {
		observability.RecordError(ctx, err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}
