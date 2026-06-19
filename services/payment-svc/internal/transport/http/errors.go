package http

import (
	"errors"
	"net/http"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/pkg/httpx"
)

// writePaymentError maps domain errors to HTTP responses.
func writePaymentError(w http.ResponseWriter, r *http.Request, err error) {
	var notFound *pkgerrors.ErrNotFound
	if errors.As(err, &notFound) {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusNotFound, "NOT_FOUND", err.Error()))
		return
	}

	var conflict *pkgerrors.ErrConflict
	if errors.As(err, &conflict) {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusConflict, "CONFLICT", err.Error()))
		return
	}

	var validation *pkgerrors.ErrValidation
	if errors.As(err, &validation) {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error()))
		return
	}

	var forbidden *pkgerrors.ErrForbidden
	if errors.As(err, &forbidden) {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusForbidden, "FORBIDDEN", err.Error()))
		return
	}

	var precondition *pkgerrors.ErrPreconditionFailed
	if errors.As(err, &precondition) {
		httpx.WriteHTTPError(w, r, httpx.NewHTTPError(http.StatusUnprocessableEntity, "PRECONDITION_FAILED", err.Error()))
		return
	}

	httpx.WriteError(w, r, err)
}
