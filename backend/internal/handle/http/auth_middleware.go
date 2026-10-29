package http

import (
	"context"
	"fmt"
	"net/http"

	"gostock/internal/domain"
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
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value == "" {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			user, err := auth.Authenticate(r.Context(), cookie.Value)
			if err != nil || user == nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired session")
				return
			}

			ctx := context.WithValue(r.Context(), authenticatedUserKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}

			if user.Role != role {
				writeError(w, http.StatusForbidden, fmt.Sprintf("forbidden: %s role required", role))
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
