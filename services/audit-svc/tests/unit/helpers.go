package unit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// NewChiRequest builds an httptest request with chi URL params pre-populated,
// so handlers using chi.URLParam(r, ...) work without a full router.
func NewChiRequest(method, target, body string, params map[string]string) *http.Request {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}

	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// NewValidator returns a validator instance for handler tests.
func NewValidator() *validator.Validate {
	return validator.New()
}
