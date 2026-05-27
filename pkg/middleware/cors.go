package middleware

import (
	"net/http"
	"strconv"
	"strings"
)

// CORSConfig holds CORS policy settings.
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int // seconds
}

// DefaultCORSConfig returns a permissive development CORS configuration.
// Override AllowedOrigins in production with explicit domain allowlists.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowedHeaders: []string{
			"Accept", "Authorization", "Content-Type",
			"X-Request-ID", "X-Correlation-ID",
		},
		ExposedHeaders:   []string{"X-Request-ID", "X-Correlation-ID"},
		AllowCredentials: false,
		MaxAge:           86400,
	}
}

// CORS returns a middleware that applies the given CORSConfig to every response.
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	wildcardOrigin := false
	allowedOrigins := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			wildcardOrigin = true
		}
		allowedOrigins[strings.ToLower(o)] = struct{}{}
	}

	allowedMethods := strings.Join(cfg.AllowedMethods, ", ")
	allowedHeaders := strings.Join(cfg.AllowedHeaders, ", ")
	exposedHeaders := strings.Join(cfg.ExposedHeaders, ", ")
	maxAgeStr := strconv.Itoa(cfg.MaxAge)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" {
				originAllowed := wildcardOrigin
				if !originAllowed {
					_, originAllowed = allowedOrigins[strings.ToLower(origin)]
				}

				if originAllowed {
					if wildcardOrigin && !cfg.AllowCredentials {
						w.Header().Set("Access-Control-Allow-Origin", "*")
					} else {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Add("Vary", "Origin")
					}
					if cfg.AllowCredentials {
						w.Header().Set("Access-Control-Allow-Credentials", "true")
					}
					if exposedHeaders != "" {
						w.Header().Set("Access-Control-Expose-Headers", exposedHeaders)
					}
				}
			}

			// Handle CORS preflight.
			if r.Method == http.MethodOptions && origin != "" {
				w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
				if cfg.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", maxAgeStr)
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
