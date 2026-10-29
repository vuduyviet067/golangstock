package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	productHTTP "gostock/internal/handle/http"
	"gostock/internal/usecase"
)

type fakeProductRepo struct{}

func (f *fakeProductRepo) GetByID(ctx context.Context, id int64) (*domain.Product, error) {
	return &domain.Product{ID: id, SKU: "SKU-001", Name: "Test Product", Price: 50, Quantity: 20}, nil
}

func (f *fakeProductRepo) Create(ctx context.Context, p *domain.Product) error {
	p.ID = 1
	return nil
}

func (f *fakeProductRepo) Update(ctx context.Context, p *domain.Product) error {
	return nil
}

func (f *fakeProductRepo) Delete(ctx context.Context, id int64) error {
	return nil
}

func (f *fakeProductRepo) List(ctx context.Context, query string, page, limit int, sortBy, order string) ([]domain.Product, int64, error) {
	return []domain.Product{
		{ID: 1, SKU: "SKU-001", Name: "Test Product", Price: 50, Quantity: 20},
	}, 1, nil
}

func (f *fakeProductRepo) StockIn(ctx context.Context, id int64, quantity int) (*domain.Product, error) {
	return &domain.Product{ID: id, SKU: "SKU-001", Name: "Test Product", Price: 50, Quantity: 20 + quantity}, nil
}

func (f *fakeProductRepo) StockOut(ctx context.Context, id int64, quantity int) (*domain.Product, error) {
	return &domain.Product{ID: id, SKU: "SKU-001", Name: "Test Product", Price: 50, Quantity: 20 - quantity}, nil
}

type fakeAuth struct{}

func (f *fakeAuth) Authenticate(ctx context.Context, token string) (*domain.User, error) {
	switch token {
	case "admin-token":
		return &domain.User{ID: 1, Email: "admin@example.com", Role: "admin"}, nil
	case "user-token":
		return &domain.User{ID: 2, Email: "user@example.com", Role: "user"}, nil
	default:
		return nil, errors.New("invalid or expired session")
	}
}

type fakeUserRepo struct{}

func (f *fakeUserRepo) Create(ctx context.Context, user *domain.User) error { return nil }
func (f *fakeUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return nil, domain.ErrUserNotFound
}
func (f *fakeUserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	return nil, domain.ErrUserNotFound
}

type fakeSessionRepo struct{}

func (f *fakeSessionRepo) Create(ctx context.Context, s *domain.Session) error { return nil }
func (f *fakeSessionRepo) GetByTokenHash(ctx context.Context, h string) (*domain.Session, error) {
	return nil, errors.New("not found")
}
func (f *fakeSessionRepo) DeleteByTokenHash(ctx context.Context, h string) error  { return nil }
func (f *fakeSessionRepo) DeleteExpired(ctx context.Context, now time.Time) error { return nil }

func setupTestRouter() *http.ServeMux {
	productRepo := &fakeProductRepo{}
	productUC := usecase.NewProductUseCase(productRepo)
	productHandler := productHTTP.NewProductHandler(productUC)

	userRepo := &fakeUserRepo{}
	sessionRepo := &fakeSessionRepo{}
	authUC := usecase.NewAuthUseCase(userRepo, sessionRepo, time.Hour)
	authHandler := productHTTP.NewAuthHandler(authUC)

	authenticator := &fakeAuth{}

	return NewRouter(nil, productHandler, authHandler, authenticator)
}

func TestProductRoutes_AuthorizationMatrix(t *testing.T) {
	router := setupTestRouter()

	validCreateBody := `{"sku":"NEW-SKU","name":"New Product","price":99,"quantity":10}`
	validUpdateBody := `{"sku":"UPD-SKU","name":"Updated Product","price":120,"quantity":15}`
	validStockBody := `{"quantity":5}`

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string // empty for anonymous, "user-token", or "admin-token"
		wantStatus int
	}{
		// 1. Anonymous requests (Must return 401 Unauthorized for all product endpoints)
		{
			name:       "Anonymous - GET /products",
			method:     http.MethodGet,
			path:       "/api/v1/products",
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Anonymous - POST /products",
			method:     http.MethodPost,
			path:       "/api/v1/products",
			body:       validCreateBody,
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Anonymous - GET /products/1",
			method:     http.MethodGet,
			path:       "/api/v1/products/1",
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Anonymous - PUT /products/1",
			method:     http.MethodPut,
			path:       "/api/v1/products/1",
			body:       validUpdateBody,
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Anonymous - DELETE /products/1",
			method:     http.MethodDelete,
			path:       "/api/v1/products/1",
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Anonymous - POST /products/1/stock-in",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       validStockBody,
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Anonymous - POST /products/1/stock-out",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-out",
			body:       validStockBody,
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},

		// 2. Regular User (role: "user")
		// Allowed: GET /products, GET /products/1, stock-in, stock-out
		// Forbidden: POST /products, PUT /products/1, DELETE /products/1
		{
			name:       "User - GET /products (allowed)",
			method:     http.MethodGet,
			path:       "/api/v1/products",
			token:      "user-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "User - GET /products/1 (allowed)",
			method:     http.MethodGet,
			path:       "/api/v1/products/1",
			token:      "user-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "User - POST /products/1/stock-in (allowed)",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       validStockBody,
			token:      "user-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "User - POST /products/1/stock-out (allowed)",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-out",
			body:       validStockBody,
			token:      "user-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "User - POST /products (forbidden)",
			method:     http.MethodPost,
			path:       "/api/v1/products",
			body:       validCreateBody,
			token:      "user-token",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "User - PUT /products/1 (forbidden)",
			method:     http.MethodPut,
			path:       "/api/v1/products/1",
			body:       validUpdateBody,
			token:      "user-token",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "User - DELETE /products/1 (forbidden)",
			method:     http.MethodDelete,
			path:       "/api/v1/products/1",
			token:      "user-token",
			wantStatus: http.StatusForbidden,
		},

		// 3. Admin (role: "admin")
		// Allowed for all operations
		{
			name:       "Admin - GET /products (allowed)",
			method:     http.MethodGet,
			path:       "/api/v1/products",
			token:      "admin-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Admin - GET /products/1 (allowed)",
			method:     http.MethodGet,
			path:       "/api/v1/products/1",
			token:      "admin-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Admin - POST /products (allowed)",
			method:     http.MethodPost,
			path:       "/api/v1/products",
			body:       validCreateBody,
			token:      "admin-token",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "Admin - PUT /products/1 (allowed)",
			method:     http.MethodPut,
			path:       "/api/v1/products/1",
			body:       validUpdateBody,
			token:      "admin-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Admin - DELETE /products/1 (allowed)",
			method:     http.MethodDelete,
			path:       "/api/v1/products/1",
			token:      "admin-token",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "Admin - POST /products/1/stock-in (allowed)",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       validStockBody,
			token:      "admin-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Admin - POST /products/1/stock-out (allowed)",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-out",
			body:       validStockBody,
			token:      "admin-token",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader *strings.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			} else {
				bodyReader = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if tt.token != "" {
				req.AddCookie(&http.Cookie{Name: "session", Value: tt.token})
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d (%s), want %d",
					tt.method, tt.path, rec.Code, rec.Body.String(), tt.wantStatus)
			}
		})
	}
}
