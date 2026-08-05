package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sanusi/banking/pkg/middleware"
)

func TestRequireServiceSecret(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	t.Run("empty secret is a no-op", func(t *testing.T) {
		h := middleware.RequireServiceSecret("")(ok)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("missing header rejected", func(t *testing.T) {
		h := middleware.RequireServiceSecret("s3cret")(ok)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("wrong secret rejected", func(t *testing.T) {
		h := middleware.RequireServiceSecret("s3cret")(ok)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set(middleware.HeaderServiceSecret, "wrong")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("matching secret allowed", func(t *testing.T) {
		h := middleware.RequireServiceSecret("s3cret")(ok)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set(middleware.HeaderServiceSecret, "s3cret")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
