package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/usecase"
)

type fakeProductRepository struct {
	product  *domain.Product
	products []domain.Product
	total    int64
	err      error

	createErr error
	createID  int64

	updateErr      error
	updatedProduct *domain.Product
	deleteErr      error

	stockInErr     error
	stockInProduct *domain.Product

	stockOutErr     error
	stockOutProduct *domain.Product
}

func (f *fakeProductRepository) GetByID(
	ctx context.Context,
	id int64,
) (*domain.Product, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.product, nil
}

func (f *fakeProductRepository) Create(
	ctx context.Context,
	product *domain.Product,
) error {
	if f.createErr != nil {
		return f.createErr
	}

	product.ID = f.createID
	return nil
}

func (f *fakeProductRepository) Update(
	ctx context.Context,
	product *domain.Product,
) error {
	if f.updateErr != nil {
		return f.updateErr
	}

	copiedProduct := *product
	f.updatedProduct = &copiedProduct

	return nil
}

func (f *fakeProductRepository) Delete(
	ctx context.Context,
	id int64,
) error {
	return f.deleteErr
}

func (f *fakeProductRepository) List(
	ctx context.Context,
	query string,
	page, limit int,
	sortBy, order string,
) ([]domain.Product, int64, error) {
	if f.err != nil {
		return nil, 0, f.err
	}

	return f.products, f.total, nil
}

func (f *fakeProductRepository) StockIn(
	ctx context.Context,
	id int64,
	quantity int,
) (*domain.Product, error) {
	if f.stockInErr != nil {
		return nil, f.stockInErr
	}

	if f.stockInProduct == nil {
		return nil, errors.New("stock-in product not configured")
	}

	product := *f.stockInProduct
	product.ID = id
	product.Quantity += quantity

	return &product, nil
}

func (f *fakeProductRepository) StockOut(
	ctx context.Context,
	id int64,
	quantity int,
) (*domain.Product, error) {
	if f.stockOutErr != nil {
		return nil, f.stockOutErr
	}

	if f.stockOutProduct == nil {
		return nil, errors.New("stock-out product not configured")
	}

	product := *f.stockOutProduct
	product.ID = id
	product.Quantity -= quantity

	return &product, nil
}

func newTestHandler(repo domain.ProductRepository) *ProductHandler {
	uc := usecase.NewProductUseCase(repo)
	return NewProductHandler(uc)
}

func TestGetByID(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		product        *domain.Product
		repoErr        error
		wantStatus     int
		wantBodyString string
		wantAllow      string
	}{
		{
			name:   "success",
			method: http.MethodGet,
			path:   "/api/v1/products/1",
			product: &domain.Product{
				ID:       1,
				SKU:      "SKU001",
				Name:     "Keyboard",
				Price:    100,
				Quantity: 5,
			},
			wantStatus:     http.StatusOK,
			wantBodyString: `"name":"Keyboard"`,
		},
		{
			name:           "invalid ID format",
			method:         http.MethodGet,
			path:           "/api/v1/products/abc",
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"error":"invalid product ID"`,
		},
		{
			name:           "zero ID",
			method:         http.MethodGet,
			path:           "/api/v1/products/0",
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"error":"invalid product ID"`,
		},
		{
			name:           "product not found",
			method:         http.MethodGet,
			path:           "/api/v1/products/99",
			repoErr:        domain.ErrProductNotFound,
			wantStatus:     http.StatusNotFound,
			wantBodyString: `"error":"Product not found"`,
		},
		{
			name:           "repository internal error",
			method:         http.MethodGet,
			path:           "/api/v1/products/1",
			repoErr:        errors.New("database connection failed"),
			wantStatus:     http.StatusInternalServerError,
			wantBodyString: `"error":"Internal server error"`,
		},
		{
			name:           "wrong HTTP method",
			method:         http.MethodPost,
			path:           "/api/v1/products/1",
			wantStatus:     http.StatusMethodNotAllowed,
			wantBodyString: `"error":"method not allowed"`,
			wantAllow:      http.MethodGet,
		},
		{
			name:           "extra path segment",
			method:         http.MethodGet,
			path:           "/api/v1/products/1/stock-in",
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"error":"invalid product ID"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{
				product: tt.product,
				err:     tt.repoErr,
			}
			handler := newTestHandler(repo)

			req := httptest.NewRequest(
				tt.method,
				tt.path,
				strings.NewReader(""),
			)
			rec := httptest.NewRecorder()

			handler.GetByID(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf(
					"status = %d, want %d; body = %s",
					rec.Code,
					tt.wantStatus,
					rec.Body.String(),
				)
			}

			if tt.wantBodyString != "" &&
				!strings.Contains(rec.Body.String(), tt.wantBodyString) {
				t.Errorf(
					"body = %s, want it to contain %q",
					rec.Body.String(),
					tt.wantBodyString,
				)
			}

			if tt.wantAllow != "" &&
				rec.Header().Get("Allow") != tt.wantAllow {
				t.Errorf(
					"Allow = %q, want %q",
					rec.Header().Get("Allow"),
					tt.wantAllow,
				)
			}

			if tt.wantStatus == http.StatusOK &&
				rec.Header().Get("Content-Type") != "application/json" {
				t.Errorf(
					"Content-Type = %q, want application/json",
					rec.Header().Get("Content-Type"),
				)
			}
		})
	}
}
func TestList(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		repoErr    error
		wantStatus int
	}{
		{
			name:       "success",
			method:     http.MethodGet,
			path:       "/api/v1/products",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid page",
			method:     http.MethodGet,
			path:       "/api/v1/products?page=abc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid limit",
			method:     http.MethodGet,
			path:       "/api/v1/products?limit=101",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid sort",
			method:     http.MethodGet,
			path:       "/api/v1/products?sortBy=unknown",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "wrong method",
			method:     http.MethodPost,
			path:       "/api/v1/products",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "repository error",
			method:     http.MethodGet,
			path:       "/api/v1/products",
			repoErr:    errors.New("database error"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{
				products: []domain.Product{},
				total:    0,
				err:      tt.repoErr,
			}
			handler := newTestHandler(repo)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.List(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"expected status %d, got %d; body=%s",
					tt.wantStatus,
					rec.Code,
					rec.Body.String(),
				)
			}

			if tt.wantStatus == http.StatusOK &&
				rec.Header().Get("Content-Type") != "application/json" {
				t.Errorf("expected application/json content type")
			}
		})
	}
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       string
		createErr  error
		wantStatus int
		wantBody   string
		wantAllow  string
	}{
		{
			name:       "success",
			method:     http.MethodPost,
			body:       `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			wantStatus: http.StatusCreated,
			wantBody:   `"id":123`,
		},
		{
			name:       "invalid JSON",
			method:     http.MethodPost,
			body:       `{"sku":`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"error":"Invalid JSON request"`,
		},
		{
			name:       "unknown field",
			method:     http.MethodPost,
			body:       `{"sku":"SKU001","name":"Keyboard","unknown":"value"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"error":"Invalid JSON request"`,
		},
		{
			name:       "trailing JSON object",
			method:     http.MethodPost,
			body:       `{"sku":"SKU001","name":"Keyboard"} {"sku":"SKU002"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"error":"Request must contain one JSON object"`,
		},
		{
			name:       "missing SKU",
			method:     http.MethodPost,
			body:       `{"name":"Keyboard","price":100,"quantity":5}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"error":"Invalid product"`,
		},
		{
			name:       "negative price",
			method:     http.MethodPost,
			body:       `{"sku":"SKU001","name":"Keyboard","price":-1,"quantity":5}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"error":"Invalid product"`,
		},
		{
			name:       "duplicate SKU",
			method:     http.MethodPost,
			body:       `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			createErr:  domain.ErrProductSKUExists,
			wantStatus: http.StatusConflict,
			wantBody:   `"error":"SKU already exists"`,
		},
		{
			name:       "repository error",
			method:     http.MethodPost,
			body:       `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			createErr:  errors.New("database connection failed"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `"error":"Internal server error"`,
		},
		{
			name:       "wrong HTTP method",
			method:     http.MethodGet,
			body:       `{}`,
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   `"error":"method not allowed"`,
			wantAllow:  http.MethodPost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{
				createErr: tt.createErr,
				createID:  123,
			}
			handler := newTestHandler(repo)

			req := httptest.NewRequest(
				tt.method,
				"/api/v1/products",
				strings.NewReader(tt.body),
			)
			rec := httptest.NewRecorder()

			handler.Create(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"status = %d, want %d; body = %s",
					rec.Code,
					tt.wantStatus,
					rec.Body.String(),
				)
			}

			if tt.wantBody != "" &&
				!strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf(
					"body = %s, want it to contain %s",
					rec.Body.String(),
					tt.wantBody,
				)
			}

			if tt.wantAllow != "" &&
				rec.Header().Get("Allow") != tt.wantAllow {
				t.Errorf(
					"Allow = %q, want %q",
					rec.Header().Get("Allow"),
					tt.wantAllow,
				)
			}

			if tt.wantStatus == http.StatusCreated &&
				!strings.Contains(
					rec.Header().Get("Content-Type"),
					"application/json",
				) {
				t.Errorf(
					"Content-Type = %q, want application/json",
					rec.Header().Get("Content-Type"),
				)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		repoErr        error
		wantStatus     int
		wantBodyString string
		wantAllow      string
	}{
		{
			name:       "success",
			method:     http.MethodPut,
			path:       "/api/v1/products/1",
			body:       `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			wantStatus: http.StatusOK,
		},
		{
			name:           "invalid ID format",
			method:         http.MethodPut,
			path:           "/api/v1/products/abc",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"error":"invalid product ID"`,
		},
		{
			name:           "zero ID",
			method:         http.MethodPut,
			path:           "/api/v1/products/0",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"error":"invalid product ID"`,
		},
		{
			name:           "invalid JSON",
			method:         http.MethodPut,
			path:           "/api/v1/products/1",
			body:           `{"sku":`,
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"error":"Invalid JSON request"`,
		},
		{
			name:           "invalid product",
			method:         http.MethodPut,
			path:           "/api/v1/products/1",
			body:           `{"sku":"","name":"Keyboard","price":100,"quantity":5}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"error":"Invalid product"`,
		},
		{
			name:           "product not found",
			method:         http.MethodPut,
			path:           "/api/v1/products/99",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			repoErr:        domain.ErrProductNotFound,
			wantStatus:     http.StatusNotFound,
			wantBodyString: `"error":"Product not found"`,
		},
		{
			name:           "duplicate SKU",
			method:         http.MethodPut,
			path:           "/api/v1/products/1",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			repoErr:        domain.ErrProductSKUExists,
			wantStatus:     http.StatusConflict,
			wantBodyString: `"error":"SKU already exists"`,
		},
		{
			name:           "repository internal error",
			method:         http.MethodPut,
			path:           "/api/v1/products/1",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			repoErr:        errors.New("database connection failed"),
			wantStatus:     http.StatusInternalServerError,
			wantBodyString: `"error":"Internal server error"`,
		},
		{
			name:           "wrong HTTP method",
			method:         http.MethodPost,
			path:           "/api/v1/products/1",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			wantStatus:     http.StatusMethodNotAllowed,
			wantBodyString: `"error":"method not allowed"`,
			wantAllow:      http.MethodPut,
		},
		{
			name:           "extra path segment",
			method:         http.MethodPut,
			path:           "/api/v1/products/1/stock-in",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"error":"invalid product ID"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{
				updateErr: tt.repoErr,
			}
			handler := newTestHandler(repo)

			req := httptest.NewRequest(
				tt.method,
				tt.path,
				strings.NewReader(tt.body),
			)
			rec := httptest.NewRecorder()

			handler.Update(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"status = %d, want %d; body = %s",
					rec.Code,
					tt.wantStatus,
					rec.Body.String(),
				)
			}

			if tt.wantBodyString != "" &&
				!strings.Contains(rec.Body.String(), tt.wantBodyString) {
				t.Errorf(
					"body = %s, want it to contain %s",
					rec.Body.String(),
					tt.wantBodyString,
				)
			}

			if tt.wantAllow != "" &&
				rec.Header().Get("Allow") != tt.wantAllow {
				t.Errorf(
					"Allow = %q, want %q",
					rec.Header().Get("Allow"),
					tt.wantAllow,
				)
			}

			if tt.name == "success" {
				if repo.updatedProduct == nil {
					t.Fatal("repository did not receive updated product")
				}

				if repo.updatedProduct.ID != 1 {
					t.Errorf(
						"updated product ID = %d, want 1",
						repo.updatedProduct.ID,
					)
				}

				if repo.updatedProduct.Name != "Keyboard" {
					t.Errorf(
						"updated product name = %q, want Keyboard",
						repo.updatedProduct.Name,
					)
				}

				if rec.Header().Get("Content-Type") != "application/json" {
					t.Errorf(
						"Content-Type = %q, want application/json",
						rec.Header().Get("Content-Type"),
					)
				}
			}
		})
	}
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		deleteErr  error
		wantStatus int
		wantBody   string
		wantAllow  string
	}{
		{
			name:       "success",
			method:     http.MethodDelete,
			path:       "/api/v1/products/1",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "invalid ID format",
			method:     http.MethodDelete,
			path:       "/api/v1/products/abc",
			wantStatus: http.StatusBadRequest,
			wantBody:   `"error":"invalid product ID"`,
		},
		{
			name:       "zero ID",
			method:     http.MethodDelete,
			path:       "/api/v1/products/0",
			wantStatus: http.StatusBadRequest,
			wantBody:   `"error":"invalid product ID"`,
		},
		{
			name:       "product not found",
			method:     http.MethodDelete,
			path:       "/api/v1/products/99",
			deleteErr:  domain.ErrProductNotFound,
			wantStatus: http.StatusNotFound,
			wantBody:   `"error":"Product not found"`,
		},
		{
			name:       "repository internal error",
			method:     http.MethodDelete,
			path:       "/api/v1/products/1",
			deleteErr:  errors.New("database connection failed"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `"error":"Internal server error"`,
		},
		{
			name:       "wrong HTTP method",
			method:     http.MethodPost,
			path:       "/api/v1/products/1",
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   `"error":"method not allowed"`,
			wantAllow:  http.MethodDelete,
		},
		{
			name:       "extra path segment",
			method:     http.MethodDelete,
			path:       "/api/v1/products/1/stock-in",
			wantStatus: http.StatusBadRequest,
			wantBody:   `"error":"invalid product ID"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{
				deleteErr: tt.deleteErr,
			}
			handler := newTestHandler(repo)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.Delete(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"status = %d, want %d; body = %s",
					rec.Code,
					tt.wantStatus,
					rec.Body.String(),
				)
			}

			if tt.wantBody != "" &&
				!strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf(
					"body = %s, want it to contain %s",
					rec.Body.String(),
					tt.wantBody,
				)
			}

			if tt.wantAllow != "" &&
				rec.Header().Get("Allow") != tt.wantAllow {
				t.Errorf(
					"Allow = %q, want %q",
					rec.Header().Get("Allow"),
					tt.wantAllow,
				)
			}

			if tt.name == "success" && rec.Body.Len() != 0 {
				t.Errorf(
					"success response body = %q, want empty body",
					rec.Body.String(),
				)
			}
		})
	}
}

func TestStockIn(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		repoErr    error
		wantStatus int
		wantBody   string
		wantAllow  string
	}{
		{
			name:       "success",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       `{"quantity":10}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid ID",
			method:     http.MethodPost,
			path:       "/api/v1/products/abc/stock-in",
			body:       `{"quantity":10}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"error":"invalid product ID"`,
		},
		{
			name:       "zero quantity",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       `{"quantity":0}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"error":"quantity must be greater than 0"`,
		},
		{
			name:       "negative quantity",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       `{"quantity":-2}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"error":"quantity must be greater than 0"`,
		},
		{
			name:       "invalid JSON",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       `{"quantity":`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"error":"Invalid JSON body"`,
		},
		{
			name:       "product not found",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       `{"quantity":10}`,
			repoErr:    domain.ErrProductNotFound,
			wantStatus: http.StatusNotFound,
			wantBody:   `"error":"Product not found"`,
		},
		{
			name:       "repository error",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       `{"quantity":10}`,
			repoErr:    errors.New("database connection failed"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `"error":"Internal server error"`,
		},
		{
			name:       "wrong HTTP method",
			method:     http.MethodPut,
			path:       "/api/v1/products/1/stock-in",
			body:       `{"quantity":10}`,
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   `"error":"method not allowed"`,
			wantAllow:  http.MethodPost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{
				stockInErr: tt.repoErr,
				stockInProduct: &domain.Product{
					ID:       1,
					SKU:      "SKU001",
					Name:     "Keyboard",
					Price:    100,
					Quantity: 5,
				},
			}

			handler := newTestHandler(repo)
			req := httptest.NewRequest(
				tt.method,
				tt.path,
				strings.NewReader(tt.body),
			)
			rec := httptest.NewRecorder()

			handler.StockIn(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"status = %d, want %d; body = %s",
					rec.Code,
					tt.wantStatus,
					rec.Body.String(),
				)
			}

			if tt.wantBody != "" &&
				!strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf(
					"body = %s, want it to contain %s",
					rec.Body.String(),
					tt.wantBody,
				)
			}

			if tt.wantAllow != "" &&
				rec.Header().Get("Allow") != tt.wantAllow {
				t.Errorf(
					"Allow = %q, want %q",
					rec.Header().Get("Allow"),
					tt.wantAllow,
				)
			}

			if tt.name == "success" {
				if !strings.Contains(rec.Body.String(), `"quantity":15`) {
					t.Errorf(
						"success body = %s, want quantity 15",
						rec.Body.String(),
					)
				}
			}
		})
	}
}

func TestStockOut(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		repoErr        error
		product        *domain.Product
		wantStatus     int
		wantBodyString string
	}{
		{
			name:           "success",
			method:         http.MethodPost,
			path:           "/api/v1/products/1/stock-out",
			body:           `{"quantity":3}`,
			product:        &domain.Product{ID: 1, SKU: "SKU-001", Name: "Keyboard", Price: 100, Quantity: 10},
			wantStatus:     http.StatusOK,
			wantBodyString: `"quantity":7`,
		},
		{
			name:           "invalid product ID",
			method:         http.MethodPost,
			path:           "/api/v1/products/abc/stock-out",
			body:           `{"quantity":3}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyString: "invalid product ID",
		},
		{
			name:           "invalid JSON",
			method:         http.MethodPost,
			path:           "/api/v1/products/1/stock-out",
			body:           `{"quantity":`,
			wantStatus:     http.StatusBadRequest,
			wantBodyString: "Invalid JSON body",
		},
		{
			name:           "zero quantity",
			method:         http.MethodPost,
			path:           "/api/v1/products/1/stock-out",
			body:           `{"quantity":0}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyString: "quantity must be greater than 0",
		},
		{
			name:           "insufficient stock",
			method:         http.MethodPost,
			path:           "/api/v1/products/1/stock-out",
			body:           `{"quantity":11}`,
			repoErr:        domain.ErrInsufficientStock,
			wantStatus:     http.StatusConflict,
			wantBodyString: "Insufficient stock",
		},
		{
			name:           "product not found",
			method:         http.MethodPost,
			path:           "/api/v1/products/1/stock-out",
			body:           `{"quantity":3}`,
			repoErr:        domain.ErrProductNotFound,
			wantStatus:     http.StatusNotFound,
			wantBodyString: "Product not found",
		},
		{
			name:           "repository error",
			method:         http.MethodPost,
			path:           "/api/v1/products/1/stock-out",
			body:           `{"quantity":3}`,
			repoErr:        errors.New("database error"),
			wantStatus:     http.StatusInternalServerError,
			wantBodyString: "Internal server error",
		},
		{
			name:           "wrong method",
			method:         http.MethodGet,
			path:           "/api/v1/products/1/stock-out",
			body:           `{"quantity":3}`,
			wantStatus:     http.StatusMethodNotAllowed,
			wantBodyString: "method not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{
				stockOutErr:     tt.repoErr,
				stockOutProduct: tt.product,
			}
			handler := newTestHandler(repo)

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			handler.StockOut(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s",
					rec.Code, tt.wantStatus, rec.Body.String())
			}

			if !strings.Contains(rec.Body.String(), tt.wantBodyString) {
				t.Errorf("body = %q, want it to contain %q",
					rec.Body.String(), tt.wantBodyString)
			}
		})
	}
}
