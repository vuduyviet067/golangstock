package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	productHTTP "gostock/internal/handle/http"
	"gostock/internal/usecase"
)

// fakeStockMovementRepo returns an empty history; the route tests only check
// authentication, authorization and routing.
type fakeStockMovementRepo struct{}

func (f *fakeStockMovementRepo) ListByProduct(ctx context.Context, productID int64, page, limit int) ([]domain.StockMovement, int64, error) {
	return []domain.StockMovement{}, 0, nil
}

func (f *fakeStockMovementRepo) List(ctx context.Context, page, limit int) ([]domain.StockMovement, int64, error) {
	return []domain.StockMovement{}, 0, nil
}

func TestStockMovementRoutes_AuthorizationMatrix(t *testing.T) {
	router := setupTestRouter()

	const (
		productHistory = "/api/v1/products/1/movements"
		globalHistory  = "/api/v1/stock-movements"
	)

	tests := []struct {
		name       string
		method     string
		path       string
		token      string // empty for anonymous, "user-token" or "admin-token"
		wantStatus int
		wantBody   string
	}{
		// Anonymous: authentication is required for both endpoints.
		{"Anonymous - product history", http.MethodGet, productHistory, "", http.StatusUnauthorized, `"message":"authentication required"`},
		{"Anonymous - global history", http.MethodGet, globalHistory, "", http.StatusUnauthorized, `"message":"authentication required"`},
		{"Invalid token - product history", http.MethodGet, productHistory, "bogus-token", http.StatusUnauthorized, `"message":"invalid or expired token"`},
		{"Invalid token - global history", http.MethodGet, globalHistory, "bogus-token", http.StatusUnauthorized, `"message":"invalid or expired token"`},

		// User: product history allowed, global history forbidden.
		{"User - product history (allowed)", http.MethodGet, productHistory, "user-token", http.StatusOK, `"message":"Stock movements retrieved successfully"`},
		{"User - global history (forbidden)", http.MethodGet, globalHistory, "user-token", http.StatusForbidden, `"message":"forbidden: admin role required"`},
		{"User - global history with query (forbidden)", http.MethodGet, globalHistory + "?page=1&limit=10", "user-token", http.StatusForbidden, `"message":"forbidden: admin role required"`},
		{"User - POST global history (forbidden before method check)", http.MethodPost, globalHistory, "user-token", http.StatusForbidden, ""},

		// Admin: both allowed.
		{"Admin - product history (allowed)", http.MethodGet, productHistory, "admin-token", http.StatusOK, `"pagination":{"page":1,"limit":10,"total":0,"total_pages":0}`},
		{"Admin - global history (allowed)", http.MethodGet, globalHistory, "admin-token", http.StatusOK, `"pagination":{"page":1,"limit":10,"total":0,"total_pages":0}`},

		// Pagination validation through the router.
		{"User - product history invalid page", http.MethodGet, productHistory + "?page=0", "user-token", http.StatusBadRequest, `"message":"invalid page"`},
		{"User - product history invalid limit", http.MethodGet, productHistory + "?limit=abc", "user-token", http.StatusBadRequest, `"message":"invalid limit"`},
		{"Admin - global history invalid page", http.MethodGet, globalHistory + "?page=abc", "admin-token", http.StatusBadRequest, `"message":"invalid page"`},
		{"Admin - global history invalid limit", http.MethodGet, globalHistory + "?limit=101", "admin-token", http.StatusBadRequest, `"message":"invalid limit"`},

		// History is read-only.
		{"User - POST product history", http.MethodPost, productHistory, "user-token", http.StatusMethodNotAllowed, `"message":"method not allowed"`},
		{"Admin - DELETE product history", http.MethodDelete, productHistory, "admin-token", http.StatusMethodNotAllowed, `"message":"method not allowed"`},
		{"Admin - POST global history", http.MethodPost, globalHistory, "admin-token", http.StatusMethodNotAllowed, `"message":"method not allowed"`},

		// Existing product routes are unaffected by the new suffix route.
		{"User - GET product still served", http.MethodGet, "/api/v1/products/1", "user-token", http.StatusOK, `"message":"Product retrieved successfully"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %s, want it to contain %s", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

// TestStockMovementRoutes_NotRegisteredWithoutHandler keeps NewRouter usable
// without the history handler, as it is without the user handler: the global
// route does not exist and product paths keep their previous handling.
func TestStockMovementRoutes_NotRegisteredWithoutHandler(t *testing.T) {
	productHandler := productHTTP.NewProductHandler(usecase.NewProductUseCase(&fakeProductRepo{}))
	authUC := usecase.NewAuthUseCase(&fakeUserRepo{}, &fakeRefreshTokenRepo{}, &fakeTokenManager{}, 15*time.Minute, 7*24*time.Hour, 15*time.Second)
	authHandler := productHTTP.NewAuthHandler(authUC, nil)
	router := NewRouter(nil, productHandler, authHandler, &fakeAuth{}, nil, nil, nil)

	tests := []struct {
		path       string
		wantStatus int
	}{
		{"/api/v1/stock-movements", http.StatusNotFound},
		// Falls through to GET /api/v1/products/{id}, which rejects the ID.
		{"/api/v1/products/1/movements", http.StatusBadRequest},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != tt.wantStatus {
			t.Errorf("%s: status = %d, want %d; body = %s", tt.path, rec.Code, tt.wantStatus, rec.Body.String())
		}
	}
}
