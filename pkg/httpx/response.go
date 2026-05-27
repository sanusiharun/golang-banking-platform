package httpx

import (
	"encoding/json"
	"net/http"
	"time"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
)

// Response is the universal API response envelope for all endpoints.
// T is the type of the data payload.
type Response[T any] struct {
	Success   bool       `json:"success"`
	Data      T          `json:"data,omitempty"`
	Error     *ErrorBody `json:"error,omitempty"`
	Meta      *Meta      `json:"meta,omitempty"`
	RequestID string     `json:"request_id"`
	Timestamp time.Time  `json:"timestamp"`
}

// ErrorBody carries structured error information.
type ErrorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// Meta carries pagination metadata for list responses.
type Meta struct {
	Page       int   `json:"page,omitempty"`
	PageSize   int   `json:"page_size,omitempty"`
	TotalCount int64 `json:"total_count,omitempty"`
	TotalPages int   `json:"total_pages,omitempty"`
}

// requestIDKey is the context key type for request IDs.
type requestIDKey struct{}

// RequestIDKey is the exported key for storing/retrieving request IDs from context.
var RequestIDKey = requestIDKey{}

// WriteSuccess writes a 200 OK JSON response with a data payload.
func WriteSuccess[T any](w http.ResponseWriter, r *http.Request, data T) {
	writeJSON(w, r, http.StatusOK, Response[T]{
		Success:   true,
		Data:      data,
		RequestID: requestIDFromRequest(r),
		Timestamp: time.Now().UTC(),
	})
}

// WriteCreated writes a 201 Created JSON response with a data payload.
func WriteCreated[T any](w http.ResponseWriter, r *http.Request, data T) {
	writeJSON(w, r, http.StatusCreated, Response[T]{
		Success:   true,
		Data:      data,
		RequestID: requestIDFromRequest(r),
		Timestamp: time.Now().UTC(),
	})
}

// WriteSuccessPaginated writes a 200 OK JSON response with a data payload and pagination meta.
func WriteSuccessPaginated[T any](w http.ResponseWriter, r *http.Request, data T, meta *Meta) {
	writeJSON(w, r, http.StatusOK, Response[T]{
		Success:   true,
		Data:      data,
		Meta:      meta,
		RequestID: requestIDFromRequest(r),
		Timestamp: time.Now().UTC(),
	})
}

// WriteNoContent writes a 204 No Content response.
func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// WriteError writes an error JSON response, deriving the status code from the
// domain error type when possible, falling back to 500.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	httpErr := mapDomainError(err)
	writeErrorResponse(w, r, httpErr)
}

// WriteHTTPError writes a known HTTPError response directly.
func WriteHTTPError(w http.ResponseWriter, r *http.Request, httpErr *HTTPError) {
	writeErrorResponse(w, r, httpErr)
}

// writeErrorResponse is the internal helper for writing error envelopes.
func writeErrorResponse(w http.ResponseWriter, r *http.Request, httpErr *HTTPError) {
	// Use a typed nil for the data field to avoid "data":null in output.
	type empty struct{}
	writeJSON(w, r, httpErr.StatusCode, Response[empty]{
		Success: false,
		Error: &ErrorBody{
			Code:    httpErr.Code,
			Message: httpErr.Message,
			Details: httpErr.Details,
		},
		RequestID: requestIDFromRequest(r),
		Timestamp: time.Now().UTC(),
	})
}

// writeJSON marshals v as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, _ *http.Request, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(statusCode)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// mapDomainError converts a domain/application error into an HTTPError.
func mapDomainError(err error) *HTTPError {
	if err == nil {
		return ErrInternal
	}

	switch {
	case pkgerrors.IsNotFound(err):
		return &HTTPError{
			StatusCode: http.StatusNotFound,
			Code:       "NOT_FOUND",
			Message:    err.Error(),
		}
	case pkgerrors.IsConflict(err):
		return &HTTPError{
			StatusCode: http.StatusConflict,
			Code:       "CONFLICT",
			Message:    err.Error(),
		}
	case pkgerrors.IsValidation(err):
		details := extractValidationDetails(err)
		return &HTTPError{
			StatusCode: http.StatusUnprocessableEntity,
			Code:       "VALIDATION_ERROR",
			Message:    err.Error(),
			Details:    details,
		}
	case pkgerrors.IsUnauthorized(err):
		return &HTTPError{
			StatusCode: http.StatusUnauthorized,
			Code:       "UNAUTHORIZED",
			Message:    err.Error(),
		}
	case pkgerrors.IsForbidden(err):
		return &HTTPError{
			StatusCode: http.StatusForbidden,
			Code:       "FORBIDDEN",
			Message:    err.Error(),
		}
	case pkgerrors.IsPreconditionFailed(err):
		return &HTTPError{
			StatusCode: http.StatusPreconditionFailed,
			Code:       "PRECONDITION_FAILED",
			Message:    err.Error(),
		}
	default:
		return &HTTPError{
			StatusCode: http.StatusInternalServerError,
			Code:       "INTERNAL_SERVER_ERROR",
			Message:    "an unexpected error occurred",
		}
	}
}

// extractValidationDetails pulls field-level details from validation errors.
func extractValidationDetails(err error) map[string]string {
	var multi *pkgerrors.ErrValidationMulti
	if pkgerrors.IsValidation(err) {
		// Try multi first
		if asMulti, ok := err.(*pkgerrors.ErrValidationMulti); ok {
			return asMulti.Fields
		}
		// Single field
		if single, ok := err.(*pkgerrors.ErrValidation); ok {
			return map[string]string{single.Field: single.Message}
		}
		_ = multi
	}
	return nil
}

// requestIDFromRequest extracts the request ID from the request context.
func requestIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if id, ok := r.Context().Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}
