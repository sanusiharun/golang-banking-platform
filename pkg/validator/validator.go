// Package validator wraps go-playground/validator with platform conventions.
// It translates validation tag errors into the domain ErrValidationMulti type
// so they can be handled uniformly by the HTTP transport layer.
package validator

import (
	"errors"
	"fmt"
	"reflect"
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
		vld.v = validator.New()
		// Register custom tag name function to use JSON field names in errors.
		vld.v.RegisterTagNameFunc(func(fld reflect.StructField) string {
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

// fixedTagMessages holds validation messages that don't depend on fe.Param().
var fixedTagMessages = map[string]string{
	"required": "this field is required",
	"email":    "must be a valid email address",
	"url":      "must be a valid URL",
	"uuid":     "must be a valid UUID",
	"alpha":    "must contain only alphabetic characters",
	"alphanum": "must contain only alphanumeric characters",
	"numeric":  "must be a numeric value",
	"iso4217":  "must be a valid ISO 4217 currency code",
}

// paramTagMessages holds validation message templates for tags that embed
// fe.Param() (e.g. a length or comparison bound).
var paramTagMessages = map[string]string{
	"min":   "must be at least %s",
	"max":   "must be at most %s",
	"len":   "must be exactly %s characters",
	"oneof": "must be one of: %s",
	"gt":    "must be greater than %s",
	"gte":   "must be greater than or equal to %s",
	"lt":    "must be less than %s",
	"lte":   "must be less than or equal to %s",
}

// buildMessage produces a human-readable validation message for a field error.
func buildMessage(fe validator.FieldError) string {
	tag := fe.Tag()
	if msg, ok := fixedTagMessages[tag]; ok {
		return msg
	}
	if tmpl, ok := paramTagMessages[tag]; ok {
		return fmt.Sprintf(tmpl, fe.Param())
	}
	return fmt.Sprintf("failed validation: %s", tag)
}

// isValidationErrors is a helper to avoid direct type assertion repetition.
func isValidationErrors(err error, out *validator.ValidationErrors) bool {
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		*out = verrs
		return true
	}
	return false
}
