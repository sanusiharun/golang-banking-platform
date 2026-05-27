package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// DecodeJSON decodes the request body into dst. Returns an HTTPError on failure
// so it can be returned directly to the caller via WriteHTTPError.
func DecodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return fmt.Errorf("request body is empty")
	}
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var syntaxErr *json.SyntaxError
		var unmarshalErr *json.UnmarshalTypeError

		switch {
		case errors.As(err, &syntaxErr):
			return fmt.Errorf("request body contains badly-formed JSON (at position %d)", syntaxErr.Offset)
		case errors.As(err, &unmarshalErr):
			return fmt.Errorf("request body contains an invalid value for the %q field (at position %d)", unmarshalErr.Field, unmarshalErr.Offset)
		default:
			return fmt.Errorf("failed to decode request body: %w", err)
		}
	}
	return nil
}

// QueryParamString returns a query parameter value, or the defaultValue if absent.
func QueryParamString(r *http.Request, key, defaultValue string) string {
	val := r.URL.Query().Get(key)
	if val == "" {
		return defaultValue
	}
	return val
}

// QueryParamInt returns an integer query parameter, or the defaultValue if absent or invalid.
func QueryParamInt(r *http.Request, key string, defaultValue int) int {
	val := r.URL.Query().Get(key)
	if val == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultValue
	}
	return n
}

// QueryParamInt64 returns an int64 query parameter, or the defaultValue if absent or invalid.
func QueryParamInt64(r *http.Request, key string, defaultValue int64) int64 {
	val := r.URL.Query().Get(key)
	if val == "" {
		return defaultValue
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return defaultValue
	}
	return n
}

// PaginationParams extracts and validates pagination query params.
// Returns page (1-based) and pageSize, clamping pageSize to [1, maxPageSize].
func PaginationParams(r *http.Request, maxPageSize int) (page int, pageSize int) {
	page = QueryParamInt(r, "page", 1)
	pageSize = QueryParamInt(r, "page_size", 20)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 1
	}
	if maxPageSize > 0 && pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}
