package transport

import (
	"net/http"

	"github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/pkg/httpx"
)

// writeError maps domain errors to HTTP responses.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.IsNotFound(err):
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusNotFound, "NOT_FOUND", err.Error()))
	case errors.IsConflict(err):
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusConflict, "CONFLICT", err.Error()))
	case errors.IsValidation(err):
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error()))
	case errors.IsUnauthorized(err):
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", err.Error()))
	case errors.IsForbidden(err):
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusForbidden, "FORBIDDEN", err.Error()))
	default:
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred"))
	}
}
