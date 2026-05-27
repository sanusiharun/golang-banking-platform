package middleware

import "net/http"

// Chain builds a middleware pipeline that applies each middleware in order
// (left-to-right). The first middleware is the outermost wrapper.
//
// Usage:
//
//	chain := middleware.Chain(RequestID, Logger(log), Recovery(log), CORS(cfg))
//	http.Handle("/", chain(myRouter))
func Chain(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(final http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			final = middlewares[i](final)
		}
		return final
	}
}
