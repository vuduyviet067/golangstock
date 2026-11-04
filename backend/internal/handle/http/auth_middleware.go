package http

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"gostock/internal/domain"
	"gostock/internal/handle/http/response"
	"gostock/internal/usecase"
)

type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*domain.User, error)
}

type contextKey string

const authenticatedUserKey contextKey = "authenticatedUser"

func AuthMiddleware(auth Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				response.Error(w, http.StatusUnauthorized, "authentication required")
				return
			}

			user, err := auth.Authenticate(r.Context(), token)
			if err != nil || user == nil {
				response.Error(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), authenticatedUserKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token := strings.TrimSpace(parts[1])
			if token != "" {
				return token
			}
		}
	}

	cookie, err := r.Cookie(AccessTokenCookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	// Legacy cookie fallback
	legacyCookie, err := r.Cookie(LegacySessionCookie)
	if err == nil && legacyCookie.Value != "" {
		return legacyCookie.Value
	}

	return ""
}

func CurrentUser(ctx context.Context) (*domain.User, bool) {
	user, ok := ctx.Value(authenticatedUserKey).(*domain.User)
	return user, ok && user != nil
}

func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := CurrentUser(r.Context())
			if !ok || user == nil {
				response.Error(w, http.StatusUnauthorized, "authentication required")
				return
			}

			if user.Role != role {
				response.Error(w, http.StatusForbidden, fmt.Sprintf("forbidden: %s role required", role))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func RequireAdmin(next http.Handler) http.Handler {
	return RequireRole("admin")(next)
}

func IsAdmin(ctx context.Context) bool {
	user, ok := CurrentUser(ctx)
	return ok && user != nil && user.Role == "admin"
}

var _ Authenticator = (*usecase.AuthUseCase)(nil)
