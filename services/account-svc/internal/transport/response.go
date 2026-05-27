// response.go — shared HTTP response helpers for account-svc transport layer.
// All handlers in this package use these helpers; no handler defines its own.
package transport

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-playground/validator/v10"

	"github.com/sanusi/banking/services/account-svc/internal/repository"
)

// ── Envelope ──────────────────────────────────────────────────────────────────

// envelope is the universal response wrapper.
// Success responses:  {"success":true,  "data":{...}}
// Error responses:    {"success":false, "error":{"code":"...","message":"..."}}
type envelope struct {
	Success bool      `json:"success"`
	Data    any       `json:"data,omitempty"`
	Error   *apiError `json:"error,omitempty"`
}

type apiError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// ── Write helpers ─────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Success: true, Data: data})
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{
		Success: false,
		Error:   &apiError{Code: code, Message: msg},
	})
}

func writeValidationError(w http.ResponseWriter, err error) {
	details := make(map[string]string)
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		for _, fe := range ve {
			details[fe.Field()] = fe.Tag()
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(envelope{
		Success: false,
		Error: &apiError{
			Code:    "VALIDATION_ERROR",
			Message: "request validation failed",
			Details: details,
		},
	})
}

// writeServiceError maps well-known sentinel errors to HTTP status codes.
// Unknown errors produce 500 and are logged.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "account not found")
	case errors.Is(err, repository.ErrInsufficientFunds):
		writeError(w, http.StatusUnprocessableEntity, "INSUFFICIENT_FUNDS", err.Error())
	case errors.Is(err, repository.ErrAccountNotActive):
		writeError(w, http.StatusUnprocessableEntity, "ACCOUNT_NOT_ACTIVE", err.Error())
	case errors.Is(err, repository.ErrConflict):
		writeError(w, http.StatusConflict, "CONFLICT", err.Error())
	default:
		slog.ErrorContext(r.Context(), "unhandled service error", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
	}
}

// ── Request helpers ───────────────────────────────────────────────────────────

// decodeJSON decodes the request body into dst.
// Limits body to 1 MiB and disallows unknown fields.
func decodeJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
