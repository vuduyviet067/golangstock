package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gostock/internal/domain"
)

// TestProductEndpoints_JSONContract pins the exact response bodies of every
// product endpoint: the envelope, the JSON field names and order, number
// formatting, and status codes. It guards the HTTP contract against changes
// in how products are mapped to responses.
func TestProductEndpoints_JSONContract(t *testing.T) {
	stored := func() *domain.Product {
		return &domain.Product{ID: 7, SKU: "SKU007", Name: "Keyboard", Price: 99.5, Quantity: 5}
	}

	tests := []struct {
		name       string
		repo       *fakeProductRepository
		method     string
		path       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "get by id",
			repo:       &fakeProductRepository{product: stored()},
			method:     http.MethodGet,
			path:       "/api/v1/products/7",
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"Product retrieved successfully","data":{"id":7,"sku":"SKU007","name":"Keyboard","price":99.5,"quantity":5,"minimum_stock":0}}`,
		},
		{
			name: "list",
			repo: &fakeProductRepository{products: []domain.Product{
				{ID: 1, SKU: "SKU001", Name: "Mouse", Price: 25, Quantity: 0},
				{ID: 2, SKU: "SKU002", Name: "Monitor", Price: 199.99, Quantity: 12},
			}, total: 2},
			method:     http.MethodGet,
			path:       "/api/v1/products",
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"Products retrieved successfully","data":[{"id":1,"sku":"SKU001","name":"Mouse","price":25,"quantity":0,"minimum_stock":0},{"id":2,"sku":"SKU002","name":"Monitor","price":199.99,"quantity":12,"minimum_stock":0}]}`,
		},
		{
			name:       "list empty",
			repo:       &fakeProductRepository{products: []domain.Product{}},
			method:     http.MethodGet,
			path:       "/api/v1/products",
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"Products retrieved successfully","data":[]}`,
		},
		{
			name:       "list nil",
			repo:       &fakeProductRepository{},
			method:     http.MethodGet,
			path:       "/api/v1/products",
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"Products retrieved successfully","data":null}`,
		},
		{
			name:   "create trims input and ignores client id",
			repo:   &fakeProductRepository{createID: 123},
			method: http.MethodPost,
			path:   "/api/v1/products",
			body:   `{"id":99,"sku":"  SKU010  ","name":" Mouse ","price":25,"quantity":3}`,
			// The ID comes from the repository, never from the request.
			wantStatus: http.StatusCreated,
			wantBody:   `{"success":true,"message":"Product created successfully","data":{"id":123,"sku":"SKU010","name":"Mouse","price":25,"quantity":3,"minimum_stock":0}}`,
		},
		{
			name:       "update returns stored quantity",
			repo:       &fakeProductRepository{storedQuantity: 5},
			method:     http.MethodPut,
			path:       "/api/v1/products/7",
			body:       `{"sku":"SKU007","name":"Keyboard Pro","price":120}`,
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"Product updated successfully","data":{"id":7,"sku":"SKU007","name":"Keyboard Pro","price":120,"quantity":5,"minimum_stock":0}}`,
		},
		{
			name:       "delete",
			repo:       &fakeProductRepository{},
			method:     http.MethodDelete,
			path:       "/api/v1/products/7",
			wantStatus: http.StatusNoContent,
			wantBody:   "",
		},
		{
			name:       "stock in",
			repo:       &fakeProductRepository{stockInProduct: stored()},
			method:     http.MethodPost,
			path:       "/api/v1/products/7/stock-in",
			body:       `{"quantity":3}`,
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"Stock added successfully","data":{"id":7,"sku":"SKU007","name":"Keyboard","price":99.5,"quantity":8,"minimum_stock":0}}`,
		},
		{
			name:       "stock out",
			repo:       &fakeProductRepository{stockOutProduct: stored()},
			method:     http.MethodPost,
			path:       "/api/v1/products/7/stock-out",
			body:       `{"quantity":3}`,
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"Stock reduced successfully","data":{"id":7,"sku":"SKU007","name":"Keyboard","price":99.5,"quantity":2,"minimum_stock":0}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newTestHandler(tt.repo)
			routes := map[string]http.HandlerFunc{
				"GET /api/v1/products/7":            handler.GetByID,
				"GET /api/v1/products":              handler.List,
				"POST /api/v1/products":             handler.Create,
				"PUT /api/v1/products/7":            handler.Update,
				"DELETE /api/v1/products/7":         handler.Delete,
				"POST /api/v1/products/7/stock-in":  handler.StockIn,
				"POST /api/v1/products/7/stock-out": handler.StockOut,
			}
			serve, ok := routes[tt.method+" "+tt.path]
			if !ok {
				t.Fatalf("no handler for %s %s", tt.method, tt.path)
			}

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			// AuthMiddleware sets the user on every product route.
			serve(rec, withTestUser(req))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if got := strings.TrimSuffix(rec.Body.String(), "\n"); got != tt.wantBody {
				t.Errorf("body mismatch\n got: %s\nwant: %s", got, tt.wantBody)
			}
		})
	}
}
