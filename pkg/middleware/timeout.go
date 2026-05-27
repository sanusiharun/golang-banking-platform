package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/sanusi/banking/pkg/httpx"
)

// Timeout wraps each request in a context with the specified deadline.
// If the handler does not complete within the timeout, the middleware
// cancels the context and returns HTTP 503 Service Unavailable.
func Timeout(duration time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), duration)
			defer cancel()

			done := make(chan struct{})
			tw := &timeoutResponseWriter{w: w}

			go func() {
				next.ServeHTTP(tw, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				tw.flush(w)
			case <-ctx.Done():
				httpx.WriteHTTPError(w, r, &httpx.HTTPError{
					StatusCode: http.StatusServiceUnavailable,
					Code:       "REQUEST_TIMEOUT",
					Message:    "request processing exceeded the allowed time limit",
				})
			}
		})
	}
}

// timeoutResponseWriter buffers the response so we can discard it if the
// context times out before the handler finishes.
type timeoutResponseWriter struct {
	w       http.ResponseWriter
	headers http.Header
	body    []byte
	status  int
}

func (tw *timeoutResponseWriter) Header() http.Header {
	if tw.headers == nil {
		tw.headers = make(http.Header)
	}
	return tw.headers
}

func (tw *timeoutResponseWriter) WriteHeader(code int) {
	tw.status = code
}

func (tw *timeoutResponseWriter) Write(b []byte) (int, error) {
	tw.body = append(tw.body, b...)
	return len(b), nil
}

func (tw *timeoutResponseWriter) flush(w http.ResponseWriter) {
	for k, v := range tw.headers {
		w.Header()[k] = v
	}
	if tw.status != 0 {
		w.WriteHeader(tw.status)
	}
	_, _ = w.Write(tw.body)
}
