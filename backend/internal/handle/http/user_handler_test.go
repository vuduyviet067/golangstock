package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/usecase"
)

type mockUserRepoForHandler struct {
	createFn  func(ctx context.Context, user *domain.User) error
	getByIDFn func(ctx context.Context, id int64) (*domain.User, error)
	listFn    func(ctx context.Context, query string, page, limit int) ([]domain.User, int64, error)
	updateFn  func(ctx context.Context, user *domain.User) error
}

func (m *mockUserRepoForHandler) Create(ctx context.Context, user *domain.User) error {
	return nil
}
func (m *mockUserRepoForHandler) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return nil, domain.ErrUserNotFound
}
func (m *mockUserRepoForHandler) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, domain.ErrUserNotFound
}
func (m *mockUserRepoForHandler) List(ctx context.Context, query string, page, limit int) ([]domain.User, int64, error) {
	if m.listFn != nil {
		return m.listFn(ctx, query, page, limit)
	}
	return []domain.User{}, 0, nil
}
func (m *mockUserRepoForHandler) Update(ctx context.Context, user *domain.User) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, user)
	}
	return nil
}
func (m *mockUserRepoForHandler) UpdateAndRevokeTokens(ctx context.Context, user *domain.User, revokedAt time.Time) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, user)
	}
	return nil
}

var _ domain.UserManagementRepository = (*mockUserRepoForHandler)(nil)

func setupUserHandler(repo *mockUserRepoForHandler) *UserHandler {
	uc := usecase.NewUserUseCase(repo)
	return NewUserHandler(uc)
}

func TestUserHandler_List(t *testing.T) {
	t.Run("success returns paginated users without password hash", func(t *testing.T) {
		repo := &mockUserRepoForHandler{
			listFn: func(ctx context.Context, query string, page, limit int) ([]domain.User, int64, error) {
				return []domain.User{
					{
						ID:           1,
						Email:        "admin@example.com",
						PasswordHash: "$2a$10$supersecret",
						Role:         "admin",
						CreatedAt:    time.Now().Add(-2 * time.Hour),
						UpdatedAt:    time.Now().Add(-1 * time.Hour),
					},
					{
						ID:           2,
						Email:        "user@example.com",
						PasswordHash: "$2a$10$supersecret",
						Role:         "user",
						CreatedAt:    time.Now().Add(-4 * time.Hour),
						UpdatedAt:    time.Now().Add(-3 * time.Hour),
					},
				}, 2, nil
			},
		}
		handler := setupUserHandler(repo)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/users?page=1&limit=10&q=example", nil)
		rec := httptest.NewRecorder()
		handler.List(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d; body: %s", rec.Code, rec.Body.String())
		}

		bodyStr := rec.Body.String()
		if strings.Contains(bodyStr, "password_hash") || strings.Contains(bodyStr, "supersecret") {
			t.Errorf("response must never expose password hash, got: %s", bodyStr)
		}

		var resp struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Data    struct {
				Items []struct {
					ID    int64  `json:"id"`
					Email string `json:"email"`
					Role  string `json:"role"`
				} `json:"items"`
				Pagination struct {
					Page       int   `json:"page"`
					Limit      int   `json:"limit"`
					Total      int64 `json:"total"`
					TotalPages int   `json:"total_pages"`
				} `json:"pagination"`
			} `json:"data"`
		}

		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response JSON: %v", err)
		}
		if !resp.Success {
			t.Errorf("expected success true, got false")
		}
		if len(resp.Data.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(resp.Data.Items))
		}
		if resp.Data.Pagination.Total != 2 || resp.Data.Pagination.Page != 1 || resp.Data.Pagination.Limit != 10 {
			t.Errorf("unexpected pagination metadata: %+v", resp.Data.Pagination)
		}
	})

	t.Run("wrong HTTP method", func(t *testing.T) {
		handler := setupUserHandler(&mockUserRepoForHandler{})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/users", nil)
		rec := httptest.NewRecorder()
		handler.List(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 Method Not Allowed, got %d", rec.Code)
		}
		if rec.Header().Get("Allow") != http.MethodGet {
			t.Errorf("expected Allow header GET, got %q", rec.Header().Get("Allow"))
		}
	})

	t.Run("invalid page query param", func(t *testing.T) {
		handler := setupUserHandler(&mockUserRepoForHandler{})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users?page=invalid", nil)
		rec := httptest.NewRecorder()
		handler.List(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("invalid limit query param", func(t *testing.T) {
		handler := setupUserHandler(&mockUserRepoForHandler{})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users?limit=notanumber", nil)
		rec := httptest.NewRecorder()
		handler.List(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
	})
}

func TestUserHandler_GetByID(t *testing.T) {
	t.Run("success returns user without password hash", func(t *testing.T) {
		repo := &mockUserRepoForHandler{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				return &domain.User{
					ID:           id,
					Email:        "user@example.com",
					PasswordHash: "$2a$10$supersecret",
					Role:         "user",
					CreatedAt:    time.Now().Add(-2 * time.Hour),
					UpdatedAt:    time.Now().Add(-1 * time.Hour),
				}, nil
			},
		}
		handler := setupUserHandler(repo)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/42", nil)
		rec := httptest.NewRecorder()
		handler.GetByID(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d; body: %s", rec.Code, rec.Body.String())
		}

		bodyStr := rec.Body.String()
		if strings.Contains(bodyStr, "password_hash") || strings.Contains(bodyStr, "supersecret") {
			t.Errorf("response must never expose password hash, got: %s", bodyStr)
		}

		var resp struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Data    struct {
				ID    int64  `json:"id"`
				Email string `json:"email"`
				Role  string `json:"role"`
			} `json:"data"`
		}

		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response JSON: %v", err)
		}
		if !resp.Success {
			t.Errorf("expected success true, got false")
		}
		if resp.Data.ID != 42 || resp.Data.Email != "user@example.com" {
			t.Errorf("unexpected user data: %+v", resp.Data)
		}
	})

	t.Run("invalid ID maps to 400", func(t *testing.T) {
		handler := setupUserHandler(&mockUserRepoForHandler{})

		for _, badPath := range []string{"/api/v1/users/abc", "/api/v1/users/-1", "/api/v1/users/0", "/api/v1/users/1/extra"} {
			req := httptest.NewRequest(http.MethodGet, badPath, nil)
			rec := httptest.NewRecorder()
			handler.GetByID(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("for path %q: expected 400 Bad Request, got %d", badPath, rec.Code)
			}
		}
	})

	t.Run("missing user maps to 404", func(t *testing.T) {
		repo := &mockUserRepoForHandler{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				return nil, domain.ErrUserNotFound
			},
		}
		handler := setupUserHandler(repo)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/999", nil)
		rec := httptest.NewRecorder()
		handler.GetByID(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong HTTP method", func(t *testing.T) {
		handler := setupUserHandler(&mockUserRepoForHandler{})
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/1", nil)
		rec := httptest.NewRecorder()
		handler.GetByID(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 Method Not Allowed, got %d", rec.Code)
		}
	})
}

func TestUserHandler_Update(t *testing.T) {
	existing := &domain.User{
		ID:           10,
		Email:        "existing@example.com",
		PasswordHash: "$2a$10$supersecret",
		Role:         "user",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}

	t.Run("success updates user and returns 200 without password hash", func(t *testing.T) {
		repo := &mockUserRepoForHandler{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				u := *existing
				return &u, nil
			},
			updateFn: func(ctx context.Context, user *domain.User) error {
				user.UpdatedAt = time.Now()
				return nil
			},
		}
		handler := setupUserHandler(repo)

		reqBody := `{"email":"newemail@example.com","role":"admin"}`
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/10", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.Update(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d; body: %s", rec.Code, rec.Body.String())
		}

		bodyStr := rec.Body.String()
		if strings.Contains(bodyStr, "password_hash") || strings.Contains(bodyStr, "supersecret") {
			t.Errorf("response must never expose password hash, got: %s", bodyStr)
		}

		var resp struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Data    struct {
				ID    int64  `json:"id"`
				Email string `json:"email"`
				Role  string `json:"role"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		if resp.Data.Email != "newemail@example.com" || resp.Data.Role != "admin" {
			t.Errorf("unexpected updated data: %+v", resp.Data)
		}
	})

	t.Run("invalid JSON / unknown fields rejected with 400", func(t *testing.T) {
		handler := setupUserHandler(&mockUserRepoForHandler{})

		for _, badBody := range []string{
			`{invalid-json}`,
			`{"email":"valid@example.com","unknown_field":123}`,
			`{"password":"newpassword"}`,
		} {
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/10", bytes.NewBufferString(badBody))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.Update(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("for body %s: expected 400 Bad Request, got %d", badBody, rec.Code)
			}
		}
	})

	t.Run("invalid user ID", func(t *testing.T) {
		handler := setupUserHandler(&mockUserRepoForHandler{})
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/abc", bytes.NewBufferString(`{"role":"admin"}`))
		rec := httptest.NewRecorder()
		handler.Update(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("empty update payload rejected with 400", func(t *testing.T) {
		repo := &mockUserRepoForHandler{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				u := *existing
				return &u, nil
			},
		}
		handler := setupUserHandler(repo)

		req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/10", bytes.NewBufferString(`{}`))
		rec := httptest.NewRecorder()
		handler.Update(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid request fields: invalid email -> 400, invalid role -> 400", func(t *testing.T) {
		repo := &mockUserRepoForHandler{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				u := *existing
				return &u, nil
			},
		}
		handler := setupUserHandler(repo)

		// Invalid email
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/10", bytes.NewBufferString(`{"email":"bad-email"}`))
		rec := httptest.NewRecorder()
		handler.Update(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("invalid email: expected 400 Bad Request, got %d", rec.Code)
		}

		// Invalid role
		req = httptest.NewRequest(http.MethodPatch, "/api/v1/users/10", bytes.NewBufferString(`{"role":"superadmin"}`))
		rec = httptest.NewRecorder()
		handler.Update(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("invalid role: expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("duplicate email maps to 409 Conflict", func(t *testing.T) {
		repo := &mockUserRepoForHandler{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				u := *existing
				return &u, nil
			},
			updateFn: func(ctx context.Context, user *domain.User) error {
				return domain.ErrEmailExists
			},
		}
		handler := setupUserHandler(repo)

		req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/10", bytes.NewBufferString(`{"email":"taken@example.com"}`))
		rec := httptest.NewRecorder()
		handler.Update(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing user maps to 404 Not Found", func(t *testing.T) {
		repo := &mockUserRepoForHandler{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				return nil, domain.ErrUserNotFound
			},
		}
		handler := setupUserHandler(repo)

		req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/99", bytes.NewBufferString(`{"role":"admin"}`))
		rec := httptest.NewRecorder()
		handler.Update(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong HTTP method", func(t *testing.T) {
		handler := setupUserHandler(&mockUserRepoForHandler{})
		req := httptest.NewRequest(http.MethodPut, "/api/v1/users/10", bytes.NewBufferString(`{"role":"admin"}`))
		rec := httptest.NewRecorder()
		handler.Update(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 Method Not Allowed, got %d", rec.Code)
		}
		if rec.Header().Get("Allow") != http.MethodPatch {
			t.Errorf("expected Allow header PATCH, got %q", rec.Header().Get("Allow"))
		}
	})
}
