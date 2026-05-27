// Package validator wraps go-playground/validator with platform conventions.
// It translates validation tag errors into the domain ErrValidationMulti type
// so they can be handled uniformly by the HTTP transport layer.
package validator

import (
	"fmt"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
	pkgerrors "github.com/sanusi/banking/pkg/errors"
)

// Validator is a thread-safe wrapper around go-playground/validator.
type Validator struct {
	v    *validator.Validate
	once sync.Once
}

// New creates a new Validator instance.
func New() *Validator {
	vld := &Validator{}
	vld.once.Do(func() {
		vld.v = validator.New(validator.WithRequiredStructFields())
		// Register custom tag name function to use JSON field names in errors.
		vld.v.RegisterTagNameFunc(func(fld validator.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			return name
		})
	})
	return vld
}

// Validate validates the struct s using struct tags.
// Returns nil if validation passes, or an ErrValidationMulti/ErrValidation on failure.
func (vld *Validator) Validate(s any) error {
	if err := vld.v.Struct(s); err != nil {
		var validationErrors validator.ValidationErrors
		if ok := isValidationErrors(err, &validationErrors); ok {
			fields := make(map[string]string, len(validationErrors))
			for _, fe := range validationErrors {
				fields[fe.Field()] = buildMessage(fe)
			}
			return pkgerrors.ValidationMulti(fields)
		}
		// Fallback for non-field errors.
		return pkgerrors.Validation("_", err.Error())
	}
	return nil
}

// ValidateVar validates a single variable against the provided tag string.
func (vld *Validator) ValidateVar(field string, value any, tag string) error {
	if err := vld.v.Var(value, tag); err != nil {
		return pkgerrors.Validation(field, fmt.Sprintf("failed validation for tag %q", tag))
	}
	return nil
}

// buildMessage produces a human-readable validation message for a field error.
func buildMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "this field is required"
	case "min":
		return fmt.Sprintf("must be at least %s", fe.Param())
	case "max":
		return fmt.Sprintf("must be at most %s", fe.Param())
	case "len":
		return fmt.Sprintf("must be exactly %s characters", fe.Param())
	case "email":
		return "must be a valid email address"
	case "url":
		return "must be a valid URL"
	case "uuid":
		return "must be a valid UUID"
	case "oneof":
		return fmt.Sprintf("must be one of: %s", fe.Param())
	case "gt":
		return fmt.Sprintf("must be greater than %s", fe.Param())
	case "gte":
		return fmt.Sprintf("must be greater than or equal to %s", fe.Param())
	case "lt":
		return fmt.Sprintf("must be less than %s", fe.Param())
	case "lte":
		return fmt.Sprintf("must be less than or equal to %s", fe.Param())
	case "alpha":
		return "must contain only alphabetic characters"
	case "alphanum":
		return "must contain only alphanumeric characters"
	case "numeric":
		return "must be a numeric value"
	case "iso4217":
		return "must be a valid ISO 4217 currency code"
	default:
		return fmt.Sprintf("failed validation: %s", fe.Tag())
	}
}

// isValidationErrors is a helper to avoid direct type assertion repetition.
func isValidationErrors(err error, out *validator.ValidationErrors) bool {
	if verrs, ok := err.(validator.ValidationErrors); ok {
		*out = verrs
		return true
	}
	return false
}
