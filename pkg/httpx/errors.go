// Package httpx provides HTTP transport utilities: response writers, request
// decoders, and HTTP-layer error types.
package httpx

import (
	"net/http"
)

// HTTPError maps a domain error to an HTTP status code and standardised error code.
type HTTPError struct {
	StatusCode int
	Code       string
	Message    string
	Details    map[string]string
}

func (e *HTTPError) Error() string { return e.Message }

// Common pre-built HTTP errors.
var (
	ErrBadRequest = &HTTPError{
		StatusCode: http.StatusBadRequest,
		Code:       "BAD_REQUEST",
		Message:    "the request was malformed or contained invalid parameters",
	}
	ErrUnauthorized = &HTTPError{
		StatusCode: http.StatusUnauthorized,
		Code:       "UNAUTHORIZED",
		Message:    "authentication is required",
	}
	ErrForbidden = &HTTPError{
		StatusCode: http.StatusForbidden,
		Code:       "FORBIDDEN",
		Message:    "you do not have permission to perform this action",
	}
	ErrNotFound = &HTTPError{
		StatusCode: http.StatusNotFound,
		Code:       "NOT_FOUND",
		Message:    "the requested resource was not found",
	}
	ErrConflict = &HTTPError{
		StatusCode: http.StatusConflict,
		Code:       "CONFLICT",
		Message:    "the resource already exists or conflicts with another",
	}
	ErrPreconditionFailed = &HTTPError{
		StatusCode: http.StatusPreconditionFailed,
		Code:       "PRECONDITION_FAILED",
		Message:    "the precondition for this operation was not met",
	}
	ErrUnprocessable = &HTTPError{
		StatusCode: http.StatusUnprocessableEntity,
		Code:       "UNPROCESSABLE_ENTITY",
		Message:    "the request was well-formed but contained semantic errors",
	}
	ErrTooManyRequests = &HTTPError{
		StatusCode: http.StatusTooManyRequests,
		Code:       "RATE_LIMITED",
		Message:    "too many requests, please slow down",
	}
	ErrInternal = &HTTPError{
		StatusCode: http.StatusInternalServerError,
		Code:       "INTERNAL_SERVER_ERROR",
		Message:    "an unexpected error occurred",
	}
)

// NewHTTPError constructs a custom HTTPError.
func NewHTTPError(statusCode int, code, message string) *HTTPError {
	return &HTTPError{StatusCode: statusCode, Code: code, Message: message}
}

// WithDetails returns a copy of the HTTPError with additional field-level details.
func (e *HTTPError) WithDetails(details map[string]string) *HTTPError {
	return &HTTPError{
		StatusCode: e.StatusCode,
		Code:       e.Code,
		Message:    e.Message,
		Details:    details,
	}
}
