// Package errors defines domain error types used across all services.
// These errors represent business/domain failures and are mapped to HTTP
// status codes at the transport layer.
package errors

import (
	"errors"
	"fmt"
)

// Sentinel error types for domain failures.

// ErrNotFound is returned when a requested resource does not exist.
type ErrNotFound struct {
	Resource string
	ID       string
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("%s with id %q not found", e.Resource, e.ID)
}

// ErrConflict is returned when an operation would violate a uniqueness constraint.
type ErrConflict struct {
	Resource string
	Field    string
	Value    string
}

func (e *ErrConflict) Error() string {
	return fmt.Sprintf("%s with %s=%q already exists", e.Resource, e.Field, e.Value)
}

// ErrValidation is returned when input fails business rule validation.
type ErrValidation struct {
	Field   string
	Message string
}

func (e *ErrValidation) Error() string {
	return fmt.Sprintf("validation error: field %q — %s", e.Field, e.Message)
}

// ErrValidationMulti carries multiple validation failures.
type ErrValidationMulti struct {
	Fields map[string]string // field → message
}

func (e *ErrValidationMulti) Error() string {
	return fmt.Sprintf("validation failed on %d field(s)", len(e.Fields))
}

// ErrUnauthorized is returned when the caller is not authenticated.
type ErrUnauthorized struct {
	Message string
}

func (e *ErrUnauthorized) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "unauthorized"
}

// ErrForbidden is returned when the caller lacks permission for an operation.
type ErrForbidden struct {
	Message string
}

func (e *ErrForbidden) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "forbidden"
}

// ErrInternal wraps unexpected infrastructure or system errors.
type ErrInternal struct {
	Cause error
}

func (e *ErrInternal) Error() string {
	return fmt.Sprintf("internal error: %v", e.Cause)
}

func (e *ErrInternal) Unwrap() error { return e.Cause }

// ErrPreconditionFailed is returned when optimistic locking detects a conflict.
type ErrPreconditionFailed struct {
	Resource string
	Message  string
}

func (e *ErrPreconditionFailed) Error() string {
	return fmt.Sprintf("precondition failed for %s: %s", e.Resource, e.Message)
}

// ErrRateLimited is returned when a caller exceeds their allowed rate.
type ErrRateLimited struct {
	RetryAfter int // seconds
}

func (e *ErrRateLimited) Error() string {
	return fmt.Sprintf("rate limit exceeded, retry after %d seconds", e.RetryAfter)
}

// Helper constructors

// NotFound creates an ErrNotFound error.
func NotFound(resource, id string) error {
	return &ErrNotFound{Resource: resource, ID: id}
}

// Conflict creates an ErrConflict error.
func Conflict(resource, field, value string) error {
	return &ErrConflict{Resource: resource, Field: field, Value: value}
}

// Validation creates a single-field ErrValidation error.
func Validation(field, message string) error {
	return &ErrValidation{Field: field, Message: message}
}

// ValidationMulti creates an ErrValidationMulti error.
func ValidationMulti(fields map[string]string) error {
	return &ErrValidationMulti{Fields: fields}
}

// Unauthorized creates an ErrUnauthorized error.
func Unauthorized(message string) error {
	return &ErrUnauthorized{Message: message}
}

// Forbidden creates an ErrForbidden error.
func Forbidden(message string) error {
	return &ErrForbidden{Message: message}
}

// Internal wraps an error as an ErrInternal.
func Internal(cause error) error {
	return &ErrInternal{Cause: cause}
}

// PreconditionFailed creates an ErrPreconditionFailed error.
func PreconditionFailed(resource, message string) error {
	return &ErrPreconditionFailed{Resource: resource, Message: message}
}

// Type check helpers

// IsNotFound reports whether err is an ErrNotFound.
func IsNotFound(err error) bool {
	var e *ErrNotFound
	return errors.As(err, &e)
}

// IsConflict reports whether err is an ErrConflict.
func IsConflict(err error) bool {
	var e *ErrConflict
	return errors.As(err, &e)
}

// IsValidation reports whether err is a validation error (single or multi).
func IsValidation(err error) bool {
	var e *ErrValidation
	var em *ErrValidationMulti
	return errors.As(err, &e) || errors.As(err, &em)
}

// IsUnauthorized reports whether err is an ErrUnauthorized.
func IsUnauthorized(err error) bool {
	var e *ErrUnauthorized
	return errors.As(err, &e)
}

// IsForbidden reports whether err is an ErrForbidden.
func IsForbidden(err error) bool {
	var e *ErrForbidden
	return errors.As(err, &e)
}

// IsInternal reports whether err is an ErrInternal.
func IsInternal(err error) bool {
	var e *ErrInternal
	return errors.As(err, &e)
}

// IsPreconditionFailed reports whether err is an ErrPreconditionFailed.
func IsPreconditionFailed(err error) bool {
	var e *ErrPreconditionFailed
	return errors.As(err, &e)
}
