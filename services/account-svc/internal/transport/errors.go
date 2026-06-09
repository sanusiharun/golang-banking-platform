// errors.go — account-svc–specific error mapping for the transport layer.
// Maps domain/repository sentinel errors to pkg/httpx HTTP errors.
// All other HTTP writing (success, validation, generic errors) goes through pkg/httpx directly.
package transport

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/sanusi/banking/pkg/httpx"
	"github.com/sanusi/banking/services/account-svc/internal/repository"
)

// writeAccountError maps well-known account-svc sentinel errors to HTTP responses.
// Unknown errors produce 500 and are logged.
func writeAccountError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusNotFound, "ACCOUNT_NOT_FOUND", "account not found"))
	case errors.Is(err, repository.ErrInsufficientFunds):
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusUnprocessableEntity, "INSUFFICIENT_FUNDS", err.Error()))
	case errors.Is(err, repository.ErrAccountNotActive):
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusUnprocessableEntity, "ACCOUNT_NOT_ACTIVE", err.Error()))
	case errors.Is(err, repository.ErrConflict):
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusConflict, "CONFLICT", err.Error()))
	default:
		slog.ErrorContext(r.Context(), "unhandled service error", slog.String("error", err.Error()))
		httpx.WriteHTTPError(w, r, httpx.ErrInternal)
	}
}
