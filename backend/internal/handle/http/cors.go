package http

import (
	"net/http"
	"strings"

	"gostock/internal/handle/http/response"
)

// CORSMiddleware enforces CORS policies and origin checks without wildcards for credentials.
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowedMap := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowedMap[strings.ToLower(strings.TrimSpace(origin))] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				cleanOrigin := strings.ToLower(strings.TrimSpace(origin))
				if allowedMap[cleanOrigin] {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
				} else if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
					// Reject cross-site mutating requests with unrecognized origin
					response.Error(w, http.StatusForbidden, "forbidden: cross-origin request not allowed")
					return
				}
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
