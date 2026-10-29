package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"gostock/internal/domain"
)

type fakeAuthenticator struct {
	user  *domain.User
	err   error
	token string
}

func (f *fakeAuthenticator) Authenticate(
	ctx context.Context,
	token string,
) (*domain.User, error) {
	f.token = token
	return f.user, f.err
}

func TestAuthMiddlewareWithoutCookie(t *testing.T) {
	auth := &fakeAuthenticator{
		user: &domain.User{ID: 1, Email: "viet@example.com"},
	}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware(auth)(next)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if nextCalled {
		t.Fatal("next handler must not be called")
	}
	if auth.token != "" {
		t.Fatal("Authenticate must not be called without a cookie")
	}
}

func TestAuthMiddlewareInvalidSession(t *testing.T) {
	auth := &fakeAuthenticator{
		err: errors.New("invalid session"),
	}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware(auth)(next)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "bad-token"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if nextCalled {
		t.Fatal("next handler must not be called")
	}
	if auth.token != "bad-token" {
		t.Fatalf("token = %q, want %q", auth.token, "bad-token")
	}
}

func TestAuthMiddlewareValidSession(t *testing.T) {
	expectedUser := &domain.User{
		ID:    1,
		Email: "viet@example.com",
		Role:  "user",
	}
	auth := &fakeAuthenticator{user: expectedUser}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true

		user, ok := CurrentUser(r.Context())
		if !ok {
			t.Fatal("CurrentUser should return a user")
		}
		if user.ID != expectedUser.ID {
			t.Errorf("user ID = %d, want %d", user.ID, expectedUser.ID)
		}

		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware(auth)(next)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "valid-token"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !nextCalled {
		t.Fatal("next handler should be called")
	}
}

func TestRequireAdmin_Unauthenticated(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireAdmin(next)
	req := httptest.NewRequest(http.MethodPost, "/admin-only", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if nextCalled {
		t.Fatal("next handler must not be called when unauthenticated")
	}
}

func TestRequireAdmin_UserRoleForbidden(t *testing.T) {
	user := &domain.User{ID: 2, Email: "user@example.com", Role: "user"}
	auth := &fakeAuthenticator{user: user}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with AuthMiddleware and RequireAdmin
	handler := AuthMiddleware(auth)(RequireAdmin(next))
	req := httptest.NewRequest(http.MethodPost, "/admin-only", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "user-token"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if nextCalled {
		t.Fatal("next handler must not be called when user is not admin")
	}
}

func TestRequireAdmin_AdminRoleAllowed(t *testing.T) {
	admin := &domain.User{ID: 1, Email: "admin@example.com", Role: "admin"}
	auth := &fakeAuthenticator{user: admin}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware(auth)(RequireAdmin(next))
	req := httptest.NewRequest(http.MethodPost, "/admin-only", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "admin-token"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !nextCalled {
		t.Fatal("next handler should be called when user is admin")
	}
}

func TestIsAdmin(t *testing.T) {
	ctx := context.Background()
	if IsAdmin(ctx) {
		t.Error("expected IsAdmin(empty ctx) to be false")
	}

	userCtx := context.WithValue(ctx, authenticatedUserKey, &domain.User{Role: "user"})
	if IsAdmin(userCtx) {
		t.Error("expected IsAdmin(user) to be false")
	}

	adminCtx := context.WithValue(ctx, authenticatedUserKey, &domain.User{Role: "admin"})
	if !IsAdmin(adminCtx) {
		t.Error("expected IsAdmin(admin) to be true")
	}
}
