package http

import (
	"net/http"
	"strings"

	"gostock/internal/handle/http/response"
)

// addVaryOrigin ensures the response varies on Origin, so shared caches never
// serve one origin's CORS headers to another. Existing Vary values are kept,
// and "Origin" is added only if no existing value already lists it (compared
// case-insensitively across all Vary lines and comma-separated tokens).
func addVaryOrigin(h http.Header) {
	for _, v := range h.Values("Vary") {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), "Origin") {
				return
			}
		}
	}
	h.Add("Vary", "Origin")
}

func isMutationMethod(method string) bool {
	return method == http.MethodPost ||
		method == http.MethodPut ||
		method == http.MethodPatch ||
		method == http.MethodDelete
}

// CORSMiddleware enforces the CORS policy for credentialed browser requests.
//
// allowedOrigins must already be canonical, as produced by config.Load
// (lowercase scheme and host, no default port, no trailing slash). The
// request's Origin header is compared against them exactly and is never
// normalized, so a malformed or non-canonical Origin never matches; browsers
// always send the canonical form.
//
// Requests from an allowed origin get CORS headers. Write requests (POST, PUT,
// PATCH, DELETE) from any other origin are rejected with 403 before reaching
// the handler. Requests without an Origin header (non-browser clients) pass
// through unchanged. OPTIONS preflight requests are answered with 204 here.
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			addVaryOrigin(w.Header())

			if origin := r.Header.Get("Origin"); origin != "" {
				if _, ok := allowed[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
				} else if isMutationMethod(r.Method) {
					// Reject cross-site writes from unrecognized or malformed origins.
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
