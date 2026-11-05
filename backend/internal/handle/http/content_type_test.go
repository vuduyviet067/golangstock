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
	"gostock/internal/handle/http/response"
	"gostock/internal/usecase"
)

func TestEnforceJSONContentType(t *testing.T) {
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !EnforceJSONContentType(w, r) {
			return
		}
		handlerCalled = true
		response.Success(w, http.StatusOK, "success", map[string]string{"result": "ok"})
	})

	reset := func() {
		handlerCalled = false
	}

	t.Run("Valid application/json", func(t *testing.T) {
		reset()
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"key":"value"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		testHandler.ServeHTTP(rec, req)

		if !handlerCalled {
			t.Error("expected handler to be called for application/json")
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("Valid application/json; charset=utf-8", func(t *testing.T) {
		reset()
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"key":"value"}`))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		rec := httptest.NewRecorder()

		testHandler.ServeHTTP(rec, req)

		if !handlerCalled {
			t.Error("expected handler to be called for application/json; charset=utf-8")
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("Missing Content-Type rejected with 415", func(t *testing.T) {
		reset()
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"key":"value"}`))
		rec := httptest.NewRecorder()

		testHandler.ServeHTTP(rec, req)

		if handlerCalled {
			t.Fatal("handler must NOT be invoked when Content-Type is missing")
		}
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("expected 415 Unsupported Media Type, got %d", rec.Code)
		}

		var resp response.Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode JSON response: %v", err)
		}
		if resp.Success {
			t.Error("expected success to be false")
		}
		if resp.Message != "Content-Type must be application/json" {
			t.Errorf("unexpected message: %q", resp.Message)
		}
	})

	t.Run("text/plain rejected with 415", func(t *testing.T) {
		reset()
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"key":"value"}`))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()

		testHandler.ServeHTTP(rec, req)

		if handlerCalled {
			t.Fatal("handler must NOT be invoked for text/plain")
		}
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("expected 415 Unsupported Media Type, got %d", rec.Code)
		}
	})

	t.Run("application/x-www-form-urlencoded rejected with 415", func(t *testing.T) {
		reset()
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`key=value`))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		testHandler.ServeHTTP(rec, req)

		if handlerCalled {
			t.Fatal("handler must NOT be invoked for application/x-www-form-urlencoded")
		}
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("expected 415 Unsupported Media Type, got %d", rec.Code)
		}
	})
}

type spyProductRepoForContentType struct {
	createCalled   bool
	updateCalled   bool
	stockInCalled  bool
	stockOutCalled bool
	product        *domain.Product
}

func (s *spyProductRepoForContentType) Create(ctx context.Context, p *domain.Product) error {
	s.createCalled = true
	p.ID = 100
	return nil
}
func (s *spyProductRepoForContentType) GetByID(ctx context.Context, id int64) (*domain.Product, error) {
	if s.product != nil {
		return s.product, nil
	}
	return &domain.Product{ID: id, SKU: "TEST", Price: 10, Quantity: 10}, nil
}
func (s *spyProductRepoForContentType) List(ctx context.Context, query string, page, limit int, sortBy, order string) ([]domain.Product, int64, error) {
	return nil, 0, nil
}
func (s *spyProductRepoForContentType) Update(ctx context.Context, p *domain.Product) error {
	s.updateCalled = true
	return nil
}
func (s *spyProductRepoForContentType) Delete(ctx context.Context, id int64) error {
	return nil
}
func (s *spyProductRepoForContentType) StockIn(ctx context.Context, id int64, qty int) (*domain.Product, error) {
	s.stockInCalled = true
	return &domain.Product{ID: id, Quantity: 20}, nil
}
func (s *spyProductRepoForContentType) StockOut(ctx context.Context, id int64, qty int) (*domain.Product, error) {
	s.stockOutCalled = true
	return &domain.Product{ID: id, Quantity: 5}, nil
}

func TestHandlers_ContentMediaTypeValidation(t *testing.T) {
	t.Run("ProductHandler.Create rejects unsupported media type and does not call repo", func(t *testing.T) {
		repo := &spyProductRepoForContentType{}
		uc := usecase.NewProductUseCase(repo)
		handler := NewProductHandler(uc)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(`{"sku":"TEST-1","name":"Test","price":10,"quantity":5}`))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()

		handler.Create(rec, req)

		if repo.createCalled {
			t.Fatal("repository Create must NOT be invoked when Content-Type is invalid")
		}
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("expected 415, got %d", rec.Code)
		}
	})

	t.Run("ProductHandler.Update rejects missing content-type", func(t *testing.T) {
		repo := &spyProductRepoForContentType{}
		uc := usecase.NewProductUseCase(repo)
		handler := NewProductHandler(uc)

		req := httptest.NewRequest(http.MethodPut, "/api/v1/products/1", strings.NewReader(`{"name":"Updated"}`))
		rec := httptest.NewRecorder()

		handler.Update(rec, req)

		if repo.updateCalled {
			t.Fatal("repository Update must NOT be invoked when Content-Type is missing")
		}
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("expected 415, got %d", rec.Code)
		}
	})

	t.Run("ProductHandler.StockIn accepts application/json; charset=utf-8", func(t *testing.T) {
		repo := &spyProductRepoForContentType{}
		uc := usecase.NewProductUseCase(repo)
		handler := NewProductHandler(uc)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/products/1/stock-in", strings.NewReader(`{"quantity":5}`))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		rec := httptest.NewRecorder()

		handler.StockIn(rec, req)

		if !repo.stockInCalled {
			t.Fatal("repository StockIn must be invoked when Content-Type is valid")
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("AuthHandler.Login rejects missing content-type without calling usecase", func(t *testing.T) {
		uRepo := &mockUserRepo{}
		rRepo := &mockRefreshRepo{}
		handler := setupTestAuthHandler(uRepo, rRepo)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader([]byte(`{"email":"user@example.com","password":"password123"}`)))
		rec := httptest.NewRecorder()

		handler.Login(rec, req)

		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("expected 415, got %d", rec.Code)
		}
	})

	t.Run("AuthHandler.Register rejects text/plain", func(t *testing.T) {
		uRepo := &mockUserRepo{}
		rRepo := &mockRefreshRepo{}
		handler := setupTestAuthHandler(uRepo, rRepo)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader([]byte(`{"email":"user@example.com","password":"password123"}`)))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()

		handler.Register(rec, req)

		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("expected 415, got %d", rec.Code)
		}
	})

	t.Run("UserHandler.Update rejects form-urlencoded", func(t *testing.T) {
		repo := &mockUserRepoForHandler{}
		handler := setupUserHandler(repo)

		req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/1", strings.NewReader("email=new@example.com"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()

		handler.Update(rec, req)

		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("expected 415, got %d", rec.Code)
		}
	})

	t.Run("AuthHandler.Logout does NOT require JSON content type", func(t *testing.T) {
		uRepo := &mockUserRepo{}
		rRepo := &mockRefreshRepo{}
		handler := setupTestAuthHandler(uRepo, rRepo)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		rec := httptest.NewRecorder()

		handler.Logout(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Logout must not require Content-Type: expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("AuthHandler.Refresh does NOT require JSON content type", func(t *testing.T) {
		uRepo := &mockUserRepo{
			user: &domain.User{ID: 1, Email: "user@example.com", Role: "user"},
		}
		rRepo := &mockRefreshRepo{
			token: &domain.RefreshToken{ID: 1, UserID: 1, ExpiresAt: time.Now().Add(24 * time.Hour)},
		}
		handler := setupTestAuthHandler(uRepo, rRepo)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: RefreshTokenCookieName, Value: "active-token"})
		rec := httptest.NewRecorder()

		handler.Refresh(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Refresh must not require Content-Type: expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("ProductHandler.Delete does NOT require JSON content type", func(t *testing.T) {
		repo := &fakeProductRepository{}
		handler := newTestHandler(repo)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/products/1", nil)
		rec := httptest.NewRecorder()

		handler.Delete(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("Delete must not require Content-Type: expected 204 NoContent, got %d", rec.Code)
		}
	})
}
