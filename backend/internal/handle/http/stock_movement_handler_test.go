package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/repository"
	"gostock/internal/usecase"
)

type fakeStockMovementRepository struct {
	movements []domain.StockMovement
	total     int64
	err       error

	calls             int
	gotProductID      int64
	gotPage, gotLimit int
}

func (f *fakeStockMovementRepository) ListByProduct(
	ctx context.Context,
	productID int64,
	page, limit int,
) ([]domain.StockMovement, int64, error) {
	f.calls++
	f.gotProductID, f.gotPage, f.gotLimit = productID, page, limit
	return f.movements, f.total, f.err
}

func (f *fakeStockMovementRepository) List(
	ctx context.Context,
	page, limit int,
) ([]domain.StockMovement, int64, error) {
	f.calls++
	f.gotPage, f.gotLimit = page, limit
	return f.movements, f.total, f.err
}

func newTestStockMovementHandler(products *fakeProductRepository, movements *fakeStockMovementRepository) *StockMovementHandler {
	return NewStockMovementHandler(usecase.NewStockMovementUseCase(products, movements))
}

func int64Ptr(v int64) *int64 { return &v }

var movementTime = time.Date(2026, 11, 11, 9, 30, 0, 0, time.UTC)

// TestStockMovementEndpoints_JSONContract pins the exact response bodies of
// both history endpoints: envelope, pagination metadata, field names and
// order, and null for a deleted product or a missing actor.
func TestStockMovementEndpoints_JSONContract(t *testing.T) {
	product := &domain.Product{ID: 7, SKU: "SKU007"}

	tests := []struct {
		name      string
		path      string
		movements []domain.StockMovement
		total     int64
		serve     func(h *StockMovementHandler) http.HandlerFunc
		wantBody  string
	}{
		{
			name: "product history",
			path: "/api/v1/products/7/movements",
			movements: []domain.StockMovement{
				{ID: 2, ProductID: int64Ptr(7), ProductSKU: "SKU007", Type: "out", QuantityChange: -3, QuantityBefore: 5, QuantityAfter: 2, UserID: int64Ptr(4), CreatedAt: movementTime},
				{ID: 1, ProductID: int64Ptr(7), ProductSKU: "SKU007", Type: "initial", QuantityChange: 5, QuantityBefore: 0, QuantityAfter: 5, UserID: nil, CreatedAt: movementTime},
			},
			total: 2,
			serve: func(h *StockMovementHandler) http.HandlerFunc { return h.ListByProduct },
			wantBody: `{"success":true,"message":"Stock movements retrieved successfully","data":{"items":[` +
				`{"id":2,"product_id":7,"product_sku":"SKU007","movement_type":"out","quantity_change":-3,"quantity_before":5,"quantity_after":2,"user_id":4,"created_at":"2026-11-11T09:30:00Z"},` +
				`{"id":1,"product_id":7,"product_sku":"SKU007","movement_type":"initial","quantity_change":5,"quantity_before":0,"quantity_after":5,"user_id":null,"created_at":"2026-11-11T09:30:00Z"}` +
				`],"pagination":{"page":1,"limit":10,"total":2,"total_pages":1}}}`,
		},
		{
			name: "global history with a deleted product",
			path: "/api/v1/stock-movements?page=2&limit=1",
			movements: []domain.StockMovement{
				{ID: 9, ProductID: nil, ProductSKU: "GONE-1", Type: "in", QuantityChange: 4, QuantityBefore: 1, QuantityAfter: 5, UserID: int64Ptr(2), CreatedAt: movementTime},
			},
			total: 3,
			serve: func(h *StockMovementHandler) http.HandlerFunc { return h.List },
			wantBody: `{"success":true,"message":"Stock movements retrieved successfully","data":{"items":[` +
				`{"id":9,"product_id":null,"product_sku":"GONE-1","movement_type":"in","quantity_change":4,"quantity_before":1,"quantity_after":5,"user_id":2,"created_at":"2026-11-11T09:30:00Z"}` +
				`],"pagination":{"page":2,"limit":1,"total":3,"total_pages":3}}}`,
		},
		{
			name:     "empty product history",
			path:     "/api/v1/products/7/movements",
			serve:    func(h *StockMovementHandler) http.HandlerFunc { return h.ListByProduct },
			wantBody: `{"success":true,"message":"Stock movements retrieved successfully","data":{"items":[],"pagination":{"page":1,"limit":10,"total":0,"total_pages":0}}}`,
		},
		{
			name:     "empty global history",
			path:     "/api/v1/stock-movements",
			serve:    func(h *StockMovementHandler) http.HandlerFunc { return h.List },
			wantBody: `{"success":true,"message":"Stock movements retrieved successfully","data":{"items":[],"pagination":{"page":1,"limit":10,"total":0,"total_pages":0}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			movements := &fakeStockMovementRepository{movements: tt.movements, total: tt.total}
			h := newTestStockMovementHandler(&fakeProductRepository{product: product}, movements)

			rec := httptest.NewRecorder()
			tt.serve(h)(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
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

func TestStockMovementHandler_PassesPagination(t *testing.T) {
	tests := []struct {
		name                string
		path                string
		serve               func(h *StockMovementHandler) http.HandlerFunc
		wantPage, wantLimit int
		wantProductID       int64
	}{
		{"product defaults", "/api/v1/products/7/movements", func(h *StockMovementHandler) http.HandlerFunc { return h.ListByProduct }, 1, 10, 7},
		{"product explicit", "/api/v1/products/7/movements?page=3&limit=25", func(h *StockMovementHandler) http.HandlerFunc { return h.ListByProduct }, 3, 25, 7},
		{"global defaults", "/api/v1/stock-movements", func(h *StockMovementHandler) http.HandlerFunc { return h.List }, 1, 10, 0},
		{"global explicit", "/api/v1/stock-movements?page=2&limit=100", func(h *StockMovementHandler) http.HandlerFunc { return h.List }, 2, 100, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			movements := &fakeStockMovementRepository{}
			h := newTestStockMovementHandler(&fakeProductRepository{product: &domain.Product{ID: 7}}, movements)

			rec := httptest.NewRecorder()
			tt.serve(h)(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
			}
			if movements.gotPage != tt.wantPage || movements.gotLimit != tt.wantLimit || movements.gotProductID != tt.wantProductID {
				t.Errorf("repository received product=%d page=%d limit=%d, want %d, %d, %d",
					movements.gotProductID, movements.gotPage, movements.gotLimit, tt.wantProductID, tt.wantPage, tt.wantLimit)
			}
		})
	}
}

// TestStockMovementHandler_RejectsInvalidRequests covers malformed IDs and
// pagination (including SQL-looking input, which never reaches a query) and
// the error mapping of the use case and repository.
func TestStockMovementHandler_RejectsInvalidRequests(t *testing.T) {
	byProduct := func(h *StockMovementHandler) http.HandlerFunc { return h.ListByProduct }
	global := func(h *StockMovementHandler) http.HandlerFunc { return h.List }

	tests := []struct {
		name        string
		method      string
		path        string
		serve       func(h *StockMovementHandler) http.HandlerFunc
		productErr  error
		repoErr     error
		wantStatus  int
		wantMessage string
		wantAllow   string
	}{
		{"non-numeric product ID", http.MethodGet, "/api/v1/products/abc/movements", byProduct, nil, nil, http.StatusBadRequest, "invalid product ID", ""},
		{"zero product ID", http.MethodGet, "/api/v1/products/0/movements", byProduct, nil, nil, http.StatusBadRequest, "invalid product ID", ""},
		{"SQL in product ID", http.MethodGet, "/api/v1/products/1%3BDROP%20TABLE%20stock_movements/movements", byProduct, nil, nil, http.StatusBadRequest, "invalid product ID", ""},
		{"extra path segment", http.MethodGet, "/api/v1/products/1/x/movements", byProduct, nil, nil, http.StatusBadRequest, "invalid product ID", ""},
		{"product: non-numeric page", http.MethodGet, "/api/v1/products/7/movements?page=abc", byProduct, nil, nil, http.StatusBadRequest, "invalid page", ""},
		{"product: SQL in page", http.MethodGet, "/api/v1/products/7/movements?page=1%3BDROP%20TABLE%20stock_movements", byProduct, nil, nil, http.StatusBadRequest, "invalid page", ""},
		{"product: zero page", http.MethodGet, "/api/v1/products/7/movements?page=0", byProduct, nil, nil, http.StatusBadRequest, "invalid page", ""},
		{"product: non-numeric limit", http.MethodGet, "/api/v1/products/7/movements?limit=ten", byProduct, nil, nil, http.StatusBadRequest, "invalid limit", ""},
		{"product: limit above 100", http.MethodGet, "/api/v1/products/7/movements?limit=101", byProduct, nil, nil, http.StatusBadRequest, "invalid limit", ""},
		{"product not found", http.MethodGet, "/api/v1/products/99/movements", byProduct, repository.ErrProductNotFound, nil, http.StatusNotFound, "Product not found", ""},
		{"product: repository error", http.MethodGet, "/api/v1/products/7/movements", byProduct, nil, errors.New("connection reset"), http.StatusInternalServerError, "Internal server error", ""},
		{"product: wrong method", http.MethodPost, "/api/v1/products/7/movements", byProduct, nil, nil, http.StatusMethodNotAllowed, "method not allowed", http.MethodGet},
		{"global: non-numeric page", http.MethodGet, "/api/v1/stock-movements?page=x", global, nil, nil, http.StatusBadRequest, "invalid page", ""},
		{"global: SQL in limit", http.MethodGet, "/api/v1/stock-movements?limit=10%20OR%201%3D1", global, nil, nil, http.StatusBadRequest, "invalid limit", ""},
		{"global: negative page", http.MethodGet, "/api/v1/stock-movements?page=-1", global, nil, nil, http.StatusBadRequest, "invalid page", ""},
		{"global: zero limit", http.MethodGet, "/api/v1/stock-movements?limit=0", global, nil, nil, http.StatusBadRequest, "invalid limit", ""},
		{"global: repository error", http.MethodGet, "/api/v1/stock-movements", global, nil, errors.New("connection reset"), http.StatusInternalServerError, "Internal server error", ""},
		{"global: wrong method", http.MethodDelete, "/api/v1/stock-movements", global, nil, nil, http.StatusMethodNotAllowed, "method not allowed", http.MethodGet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			products := &fakeProductRepository{product: &domain.Product{ID: 7}, err: tt.productErr}
			movements := &fakeStockMovementRepository{err: tt.repoErr}
			h := newTestStockMovementHandler(products, movements)

			rec := httptest.NewRecorder()
			tt.serve(h)(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			wantBody := `{"success":false,"message":"` + tt.wantMessage + `","data":null}`
			if got := strings.TrimSpace(rec.Body.String()); got != wantBody {
				t.Errorf("body = %s, want %s", got, wantBody)
			}
			if tt.wantAllow != "" && rec.Header().Get("Allow") != tt.wantAllow {
				t.Errorf("Allow = %q, want %q", rec.Header().Get("Allow"), tt.wantAllow)
			}
			if tt.wantStatus == http.StatusBadRequest || tt.wantStatus == http.StatusMethodNotAllowed || tt.productErr != nil {
				if movements.calls != 0 {
					t.Error("movement repository must not be called for a rejected request")
				}
			}
		})
	}
}
