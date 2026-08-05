// Package transport contains HTTP handlers for auth-svc.
package transport

import (
	"errors"
	"net/http"

	"github.com/go-playground/validator/v10"

	pkgaudit "github.com/sanusi/banking/pkg/audit"
	"github.com/sanusi/banking/pkg/featureflag"
	"github.com/sanusi/banking/pkg/httpx"
	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/auth-svc/internal/services"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	tr       *observability.ServiceTracer
	svc      services.AuthService
	validate *validator.Validate
	audit    pkgaudit.Publisher
}

func NewAuthHandler(svc services.AuthService, validate *validator.Validate, audit pkgaudit.Publisher) *AuthHandler {
	return &AuthHandler{
		tr:       observability.NewServiceTracer("AuthHandler"),
		svc:      svc,
		validate: validate,
		audit:    audit,
	}
}

// Login handles POST /auth/login.
//
// Feature flag: "maintenance_mode"
//
//	enabled  → returns 503 Service Unavailable, login is blocked
//	disabled → normal login flow (default)
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Login")
	defer span.End()

	// Feature flag: block all logins during maintenance window.
	// Toggle in Flipt UI → http://localhost:8085 → flag key: maintenance_mode
	if featureflag.IsEnabled(ctx, "maintenance_mode", false) {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusServiceUnavailable, "MAINTENANCE_MODE", "service is temporarily unavailable for maintenance"))
		return
	}

	var req dto.LoginRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteValidationError(w, r, err)
		return
	}

	resp, err := h.svc.Login(ctx, &req)
	if err != nil {
		observability.RecordError(ctx, err)
		if errors.Is(err, services.ErrInvalidCredentials) {
			_ = h.audit.Publish(r.Context(), pkgaudit.AuditEvent{ //nolint:errcheck // audit publish failure must not block the response to the caller
				ActorType:   pkgaudit.ActorTypeUser,
				ActorID:     req.Username,
				ActorEmail:  req.Username,
				Action:      pkgaudit.ActionAuthLoginFailed,
				Status:      pkgaudit.StatusFailure,
				ServiceName: "auth-svc",
				IPAddress:   r.RemoteAddr,
				UserAgent:   r.UserAgent(),
				TraceID:     span.SpanContext().TraceID().String(),
			})
			httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusUnauthorized, "INVALID_CREDENTIALS", err.Error()))
			return
		}
		httpx.WriteHTTPError(w, r, httpx.ErrInternal)
		return
	}

	_ = h.audit.Publish(r.Context(), pkgaudit.AuditEvent{ //nolint:errcheck // audit publish failure must not block the response to the caller
		ActorType:   pkgaudit.ActorTypeUser,
		ActorID:     resp.UserID,
		ActorEmail:  req.Username,
		Action:      pkgaudit.ActionAuthLogin,
		Status:      pkgaudit.StatusSuccess,
		ServiceName: "auth-svc",
		IPAddress:   r.RemoteAddr,
		UserAgent:   r.UserAgent(),
		TraceID:     span.SpanContext().TraceID().String(),
	})

	httpx.WriteSuccess(w, r, resp)
}

// Refresh handles POST /auth/refresh.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Refresh")
	defer span.End()

	var req dto.RefreshRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteValidationError(w, r, err)
		return
	}

	resp, err := h.svc.Refresh(ctx, &req)
	if err != nil {
		observability.RecordError(ctx, err)
		if errors.Is(err, services.ErrInvalidToken) {
			httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", err.Error()))
			return
		}
		httpx.WriteHTTPError(w, r, httpx.ErrInternal)
		return
	}

	_ = h.audit.Publish(r.Context(), pkgaudit.AuditEvent{ //nolint:errcheck // audit publish failure must not block the response to the caller
		ActorType:   pkgaudit.ActorTypeUser,
		ActorID:     resp.UserID,
		Action:      pkgaudit.ActionAuthTokenRefresh,
		Status:      pkgaudit.StatusSuccess,
		ServiceName: "auth-svc",
		IPAddress:   r.RemoteAddr,
		UserAgent:   r.UserAgent(),
		TraceID:     span.SpanContext().TraceID().String(),
	})

	httpx.WriteSuccess(w, r, resp)
}

// Logout handles POST /auth/logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "Logout")
	defer span.End()

	var req dto.LogoutRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusBadRequest, "INVALID_JSON", err.Error()))
		return
	}
	if err := h.validate.Struct(&req); err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteValidationError(w, r, err)
		return
	}

	if err := h.svc.Logout(ctx, &req); err != nil {
		observability.RecordError(ctx, err)
		httpx.WriteHTTPError(w, r, httpx.ErrInternal)
		return
	}

	_ = h.audit.Publish(r.Context(), pkgaudit.AuditEvent{ //nolint:errcheck // audit publish failure must not block the response to the caller
		ActorType:   pkgaudit.ActorTypeUser,
		ActorID:     pkgmiddleware.UserIDFromContext(ctx),
		Action:      pkgaudit.ActionAuthLogout,
		Status:      pkgaudit.StatusSuccess,
		ServiceName: "auth-svc",
		IPAddress:   r.RemoteAddr,
		UserAgent:   r.UserAgent(),
		TraceID:     span.SpanContext().TraceID().String(),
	})

	httpx.WriteSuccess(w, r, map[string]string{"message": "logged out"})
}
