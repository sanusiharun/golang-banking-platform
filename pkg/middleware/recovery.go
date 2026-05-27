package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/sanusi/banking/pkg/httpx"
)

// Recovery is a panic recovery middleware that logs the panic and stack trace
// via the global slog logger and returns HTTP 500 to the client.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := debug.Stack()
				requestID, _ := r.Context().Value(httpx.RequestIDKey).(string)

				slog.ErrorContext(r.Context(), "recovered from panic",
					slog.String("request_id", requestID),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("panic", fmt.Sprintf("%v", rec)),
					slog.String("stack", string(stack)),
				)

				httpx.WriteHTTPError(w, r, httpx.ErrInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
