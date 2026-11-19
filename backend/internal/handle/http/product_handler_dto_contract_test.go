package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/repository"
)

// TestProductEndpoints_ErrorAndEdgeContract complements
// TestProductEndpoints_JSONContract (Mapper refactor Step 3.3): it pins the
// exact bodies of the error paths of every product endpoint and of edge cases
// of the success paths, now that the use case returns the response DTOs.
//
// The product list is not paginated: data is the bare item array and the
// total from the repository is not part of the response.
func TestProductEndpoints_ErrorAndEdgeContract(t *testing.T) {
	dbErr := errors.New("connection reset")
	errorBody := func(code, message string) string {
		return `{"success":false,"code":"` + code + `","message":"` + message + `","data":null}`
	}

	tests := []struct {
		name       string
		repo       *fakeProductRepository
		serve      func(h *ProductHandler) http.HandlerFunc
		method     string
		path       string
		body       string
		wantStatus int
		wantBody   string
	}{
		// GetByID
		{
			name:       "get: non-zero minimum stock and zero price",
			repo:       &fakeProductRepository{product: &domain.Product{ID: 3, SKU: "SKU003", Name: "Cable", Price: 0, Quantity: 0, MinimumStock: 5}},
			serve:      func(h *ProductHandler) http.HandlerFunc { return h.GetByID },
			method:     http.MethodGet,
			path:       "/api/v1/products/3",
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"Product retrieved successfully","data":{"id":3,"sku":"SKU003","name":"Cable","price":0,"quantity":0,"minimum_stock":5}}`,
		},
		{"get: not found", &fakeProductRepository{err: repository.ErrProductNotFound}, func(h *ProductHandler) http.HandlerFunc { return h.GetByID },
			http.MethodGet, "/api/v1/products/99", "", http.StatusNotFound, errorBody("PRODUCT_NOT_FOUND", "Product not found")},
		{"get: repository error", &fakeProductRepository{err: dbErr}, func(h *ProductHandler) http.HandlerFunc { return h.GetByID },
			http.MethodGet, "/api/v1/products/7", "", http.StatusInternalServerError, errorBody("INTERNAL_ERROR", "Internal server error")},
		{"get: non-numeric id", &fakeProductRepository{}, func(h *ProductHandler) http.HandlerFunc { return h.GetByID },
			http.MethodGet, "/api/v1/products/abc", "", http.StatusBadRequest, errorBody("INVALID_PRODUCT_ID", "invalid product ID")},
		{"get: zero id", &fakeProductRepository{}, func(h *ProductHandler) http.HandlerFunc { return h.GetByID },
			http.MethodGet, "/api/v1/products/0", "", http.StatusBadRequest, errorBody("INVALID_PRODUCT_ID", "invalid product ID")},

		// List
		{
			name:       "list: page past the end is [] and the total is not exposed",
			repo:       &fakeProductRepository{products: []domain.Product{}, total: 25},
			serve:      func(h *ProductHandler) http.HandlerFunc { return h.List },
			method:     http.MethodGet,
			path:       "/api/v1/products?page=9&limit=10",
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"Products retrieved successfully","data":[]}`,
		},
		{
			name: "list: order, zero values and minimum stock",
			repo: &fakeProductRepository{products: []domain.Product{
				{ID: 3, SKU: "SKU003", Name: "Cable"},
				{ID: 1, SKU: "SKU001", Name: "Mouse", Price: 25.5, Quantity: 4, MinimumStock: 5},
			}, total: 40},
			serve:      func(h *ProductHandler) http.HandlerFunc { return h.List },
			method:     http.MethodGet,
			path:       "/api/v1/products?page=2&limit=2&sortBy=name&order=desc",
			wantStatus: http.StatusOK,
			wantBody: `{"success":true,"message":"Products retrieved successfully","data":[` +
				`{"id":3,"sku":"SKU003","name":"Cable","price":0,"quantity":0,"minimum_stock":0},` +
				`{"id":1,"sku":"SKU001","name":"Mouse","price":25.5,"quantity":4,"minimum_stock":5}]}`,
		},
		{"list: non-numeric page", &fakeProductRepository{}, func(h *ProductHandler) http.HandlerFunc { return h.List },
			http.MethodGet, "/api/v1/products?page=abc", "", http.StatusBadRequest, errorBody("INVALID_PAGE", "invalid page")},
		{"list: non-numeric limit", &fakeProductRepository{}, func(h *ProductHandler) http.HandlerFunc { return h.List },
			http.MethodGet, "/api/v1/products?limit=abc", "", http.StatusBadRequest, errorBody("INVALID_LIMIT", "invalid limit")},
		{"list: page below 1", &fakeProductRepository{}, func(h *ProductHandler) http.HandlerFunc { return h.List },
			http.MethodGet, "/api/v1/products?page=0", "", http.StatusBadRequest, errorBody("INVALID_PRODUCT", "Invalid product")},
		{"list: unsupported sort", &fakeProductRepository{}, func(h *ProductHandler) http.HandlerFunc { return h.List },
			http.MethodGet, "/api/v1/products?sortBy=sku", "", http.StatusBadRequest, errorBody("INVALID_PRODUCT", "Invalid product")},
		{"list: repository error", &fakeProductRepository{err: dbErr}, func(h *ProductHandler) http.HandlerFunc { return h.List },
			http.MethodGet, "/api/v1/products", "", http.StatusInternalServerError, errorBody("INTERNAL_ERROR", "Internal server error")},

		// Create
		{
			name:       "create: minimum stock is returned",
			repo:       &fakeProductRepository{createID: 50},
			serve:      func(h *ProductHandler) http.HandlerFunc { return h.Create },
			method:     http.MethodPost,
			path:       "/api/v1/products",
			body:       `{"sku":"SKU050","name":"Hub","price":0,"quantity":0,"minimum_stock":2}`,
			wantStatus: http.StatusCreated,
			wantBody:   `{"success":true,"message":"Product created successfully","data":{"id":50,"sku":"SKU050","name":"Hub","price":0,"quantity":0,"minimum_stock":2}}`,
		},
		{"create: duplicate SKU", &fakeProductRepository{createErr: repository.ErrProductSKUExists}, func(h *ProductHandler) http.HandlerFunc { return h.Create },
			http.MethodPost, "/api/v1/products", `{"sku":"SKU001","name":"Mouse","price":1,"quantity":1}`, http.StatusConflict, errorBody("SKU_EXISTS", "SKU already exists")},
		{"create: invalid product", &fakeProductRepository{}, func(h *ProductHandler) http.HandlerFunc { return h.Create },
			http.MethodPost, "/api/v1/products", `{"sku":"  ","name":"Mouse","price":1,"quantity":1}`, http.StatusBadRequest, errorBody("INVALID_PRODUCT", "Invalid product")},
		{"create: repository error", &fakeProductRepository{createErr: dbErr}, func(h *ProductHandler) http.HandlerFunc { return h.Create },
			http.MethodPost, "/api/v1/products", `{"sku":"SKU001","name":"Mouse","price":1,"quantity":1}`, http.StatusInternalServerError, errorBody("INTERNAL_ERROR", "Internal server error")},

		// Update
		{
			name:       "update: stored quantity and new minimum stock are returned",
			repo:       &fakeProductRepository{storedQuantity: 9, storedMinimumStock: 1},
			serve:      func(h *ProductHandler) http.HandlerFunc { return h.Update },
			method:     http.MethodPut,
			path:       "/api/v1/products/7",
			body:       `{"sku":" SKU007 ","name":"Keyboard","price":10,"minimum_stock":4}`,
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"Product updated successfully","data":{"id":7,"sku":"SKU007","name":"Keyboard","price":10,"quantity":9,"minimum_stock":4}}`,
		},
		{"update: quantity is rejected", &fakeProductRepository{}, func(h *ProductHandler) http.HandlerFunc { return h.Update },
			http.MethodPut, "/api/v1/products/7", `{"sku":"SKU007","name":"Keyboard","price":10,"quantity":0}`, http.StatusBadRequest,
			errorBody("VALIDATION_ERROR", "quantity cannot be updated; use stock-in or stock-out")},
		{"update: not found", &fakeProductRepository{updateErr: repository.ErrProductNotFound}, func(h *ProductHandler) http.HandlerFunc { return h.Update },
			http.MethodPut, "/api/v1/products/99", `{"sku":"SKU099","name":"Gone","price":1}`, http.StatusNotFound, errorBody("PRODUCT_NOT_FOUND", "Product not found")},
		{"update: duplicate SKU", &fakeProductRepository{updateErr: repository.ErrProductSKUExists}, func(h *ProductHandler) http.HandlerFunc { return h.Update },
			http.MethodPut, "/api/v1/products/7", `{"sku":"SKU001","name":"Keyboard","price":1}`, http.StatusConflict, errorBody("SKU_EXISTS", "SKU already exists")},
		{"update: invalid product", &fakeProductRepository{}, func(h *ProductHandler) http.HandlerFunc { return h.Update },
			http.MethodPut, "/api/v1/products/7", `{"sku":"SKU007","name":"","price":1}`, http.StatusBadRequest, errorBody("INVALID_PRODUCT", "Invalid product")},

		// Delete
		{"delete: not found", &fakeProductRepository{deleteErr: repository.ErrProductNotFound}, func(h *ProductHandler) http.HandlerFunc { return h.Delete },
			http.MethodDelete, "/api/v1/products/99", "", http.StatusNotFound, errorBody("PRODUCT_NOT_FOUND", "Product not found")},

		// Stock in / out
		{"stock in: zero quantity", &fakeProductRepository{}, func(h *ProductHandler) http.HandlerFunc { return h.StockIn },
			http.MethodPost, "/api/v1/products/7/stock-in", `{"quantity":0}`, http.StatusBadRequest, errorBody("INVALID_QUANTITY", "quantity must be greater than 0")},
		{"stock in: not found", &fakeProductRepository{stockInErr: repository.ErrProductNotFound}, func(h *ProductHandler) http.HandlerFunc { return h.StockIn },
			http.MethodPost, "/api/v1/products/99/stock-in", `{"quantity":1}`, http.StatusNotFound, errorBody("PRODUCT_NOT_FOUND", "Product not found")},
		{"stock out: insufficient stock", &fakeProductRepository{stockOutErr: repository.ErrInsufficientStock}, func(h *ProductHandler) http.HandlerFunc { return h.StockOut },
			http.MethodPost, "/api/v1/products/7/stock-out", `{"quantity":100}`, http.StatusConflict, errorBody("INSUFFICIENT_STOCK", "Insufficient stock")},
		{"stock out: repository error", &fakeProductRepository{stockOutErr: dbErr}, func(h *ProductHandler) http.HandlerFunc { return h.StockOut },
			http.MethodPost, "/api/v1/products/7/stock-out", `{"quantity":1}`, http.StatusInternalServerError, errorBody("INTERNAL_ERROR", "Internal server error")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			tt.serve(newTestHandler(tt.repo))(rec, withTestUser(req))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			if got := strings.TrimSuffix(rec.Body.String(), "\n"); got != tt.wantBody {
				t.Errorf("body mismatch\n got: %s\nwant: %s", got, tt.wantBody)
			}
		})
	}
}
