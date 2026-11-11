package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/repository"
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
	// storedQuantity simulates the quantity the database returns from
	// Update (RETURNING quantity); Update never writes it.
	storedQuantity int
	// storedMinimumStock simulates the threshold already in the database;
	// Update keeps it when minimumStock is nil.
	storedMinimumStock int
	// updateMinimumStock records the minimumStock argument passed to Update.
	updateMinimumStock *int
	// createdProduct records the product passed to Create.
	createdProduct *domain.Product
	deleteErr      error

	stockInErr     error
	stockInProduct *domain.Product

	stockOutErr     error
	stockOutProduct *domain.Product

	// actorID records the actor passed to Create, StockIn or StockOut;
	// stockCalls counts those calls.
	actorID    int64
	stockCalls int
}

// testActorID is the authenticated user attached by withTestUser.
const testActorID int64 = 42

// withUserID attaches an authenticated user with the given ID to req, as
// AuthMiddleware does for a valid token.
func withUserID(req *http.Request, id int64) *http.Request {
	user := &domain.User{ID: id, Email: "actor@example.test", Role: usecase.RoleUser}
	return req.WithContext(context.WithValue(req.Context(), authenticatedUserKey, user))
}

// withTestUser attaches the default test user (testActorID) to req.
func withTestUser(req *http.Request) *http.Request {
	return withUserID(req, testActorID)
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
	actorID int64,
) error {
	f.actorID = actorID
	f.stockCalls++
	if f.createErr != nil {
		return f.createErr
	}

	created := *product
	f.createdProduct = &created

	product.ID = f.createID
	return nil
}

func (f *fakeProductRepository) Update(
	ctx context.Context,
	product *domain.Product,
	minimumStock *int,
) error {
	if f.updateErr != nil {
		return f.updateErr
	}

	copiedProduct := *product
	f.updatedProduct = &copiedProduct
	f.updateMinimumStock = minimumStock

	product.Quantity = f.storedQuantity
	product.MinimumStock = f.storedMinimumStock
	if minimumStock != nil {
		product.MinimumStock = *minimumStock
	}
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
	actorID int64,
) (*domain.Product, error) {
	f.actorID = actorID
	f.stockCalls++
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
	actorID int64,
) (*domain.Product, error) {
	f.actorID = actorID
	f.stockCalls++
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

func newTestHandler(repo repository.ProductRepository) *ProductHandler {
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
			wantBodyString: `"message":"invalid product ID"`,
		},
		{
			name:           "zero ID",
			method:         http.MethodGet,
			path:           "/api/v1/products/0",
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"message":"invalid product ID"`,
		},
		{
			name:           "product not found",
			method:         http.MethodGet,
			path:           "/api/v1/products/99",
			repoErr:        repository.ErrProductNotFound,
			wantStatus:     http.StatusNotFound,
			wantBodyString: `"message":"Product not found"`,
		},
		{
			name:           "repository internal error",
			method:         http.MethodGet,
			path:           "/api/v1/products/1",
			repoErr:        errors.New("database connection failed"),
			wantStatus:     http.StatusInternalServerError,
			wantBodyString: `"message":"Internal server error"`,
		},
		{
			name:           "wrong HTTP method",
			method:         http.MethodPost,
			path:           "/api/v1/products/1",
			wantStatus:     http.StatusMethodNotAllowed,
			wantBodyString: `"message":"method not allowed"`,
			wantAllow:      http.MethodGet,
		},
		{
			name:           "extra path segment",
			method:         http.MethodGet,
			path:           "/api/v1/products/1/stock-in",
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"message":"invalid product ID"`,
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
			wantBody:   `"message":"Invalid JSON request"`,
		},
		{
			name:       "unknown field",
			method:     http.MethodPost,
			body:       `{"sku":"SKU001","name":"Keyboard","unknown":"value"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"message":"Invalid JSON request"`,
		},
		{
			name:       "trailing JSON object",
			method:     http.MethodPost,
			body:       `{"sku":"SKU001","name":"Keyboard"} {"sku":"SKU002"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"message":"Request must contain one JSON object"`,
		},
		{
			name:       "missing SKU",
			method:     http.MethodPost,
			body:       `{"name":"Keyboard","price":100,"quantity":5}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"message":"Invalid product"`,
		},
		{
			name:       "negative price",
			method:     http.MethodPost,
			body:       `{"sku":"SKU001","name":"Keyboard","price":-1,"quantity":5}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"message":"Invalid product"`,
		},
		{
			name:       "duplicate SKU",
			method:     http.MethodPost,
			body:       `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			createErr:  repository.ErrProductSKUExists,
			wantStatus: http.StatusConflict,
			wantBody:   `"message":"SKU already exists"`,
		},
		{
			name:       "repository error",
			method:     http.MethodPost,
			body:       `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			createErr:  errors.New("database connection failed"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `"message":"Internal server error"`,
		},
		{
			name:       "wrong HTTP method",
			method:     http.MethodGet,
			body:       `{}`,
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   `"message":"method not allowed"`,
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
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.Create(rec, withTestUser(req))

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
			body:       `{"sku":"SKU001","name":"Keyboard","price":100}`,
			wantStatus: http.StatusOK,
		},
		{
			name:           "invalid ID format",
			method:         http.MethodPut,
			path:           "/api/v1/products/abc",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"message":"invalid product ID"`,
		},
		{
			name:           "zero ID",
			method:         http.MethodPut,
			path:           "/api/v1/products/0",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"message":"invalid product ID"`,
		},
		{
			name:           "invalid JSON",
			method:         http.MethodPut,
			path:           "/api/v1/products/1",
			body:           `{"sku":`,
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"message":"Invalid JSON request"`,
		},
		{
			name:           "invalid product",
			method:         http.MethodPut,
			path:           "/api/v1/products/1",
			body:           `{"sku":"","name":"Keyboard","price":100}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"message":"Invalid product"`,
		},
		{
			name:           "product not found",
			method:         http.MethodPut,
			path:           "/api/v1/products/99",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100}`,
			repoErr:        repository.ErrProductNotFound,
			wantStatus:     http.StatusNotFound,
			wantBodyString: `"message":"Product not found"`,
		},
		{
			name:           "duplicate SKU",
			method:         http.MethodPut,
			path:           "/api/v1/products/1",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100}`,
			repoErr:        repository.ErrProductSKUExists,
			wantStatus:     http.StatusConflict,
			wantBodyString: `"message":"SKU already exists"`,
		},
		{
			name:           "repository internal error",
			method:         http.MethodPut,
			path:           "/api/v1/products/1",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100}`,
			repoErr:        errors.New("database connection failed"),
			wantStatus:     http.StatusInternalServerError,
			wantBodyString: `"message":"Internal server error"`,
		},
		{
			name:           "wrong HTTP method",
			method:         http.MethodPost,
			path:           "/api/v1/products/1",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100}`,
			wantStatus:     http.StatusMethodNotAllowed,
			wantBodyString: `"message":"method not allowed"`,
			wantAllow:      http.MethodPut,
		},
		{
			name:           "extra path segment",
			method:         http.MethodPut,
			path:           "/api/v1/products/1/stock-in",
			body:           `{"sku":"SKU001","name":"Keyboard","price":100}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyString: `"message":"invalid product ID"`,
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
			req.Header.Set("Content-Type", "application/json")
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

// TestUpdateRejectsQuantity guards CR-15: PUT must never carry a stock value,
// whatever it is, and a rejected request must not reach the repository.
func TestUpdateRejectsQuantity(t *testing.T) {
	bodies := map[string]string{
		"positive quantity": `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":10}`,
		"zero quantity":     `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":0}`,
		"null quantity":     `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":null}`,
		"negative quantity": `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":-1}`,
		"quantity only":     `{"quantity":7}`,
	}

	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			repo := &fakeProductRepository{}
			handler := newTestHandler(repo)

			req := httptest.NewRequest(http.MethodPut, "/api/v1/products/1", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.Update(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			wantBody := `{"success":false,"message":"quantity cannot be updated; use stock-in or stock-out","data":null}`
			if got := strings.TrimSpace(rec.Body.String()); got != wantBody {
				t.Errorf("body = %s, want %s", got, wantBody)
			}
			if repo.updatedProduct != nil {
				t.Fatal("repository Update must not be called when quantity is present")
			}
		})
	}
}

// TestUpdateReportsStoredQuantity checks that the PUT response carries the
// quantity reported by the repository (the stored stock), not a client value.
func TestUpdateReportsStoredQuantity(t *testing.T) {
	repo := &fakeProductRepository{storedQuantity: 7}
	handler := newTestHandler(repo)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/products/1",
		strings.NewReader(`{"id":99,"sku":"SKU001","name":"Renamed","price":100}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if repo.updatedProduct == nil {
		t.Fatal("repository Update was not called")
	}
	if repo.updatedProduct.ID != 1 {
		t.Errorf("updated product ID = %d, want 1 (path ID wins over body id)", repo.updatedProduct.ID)
	}
	if repo.updatedProduct.Quantity != 0 {
		t.Errorf("handler passed quantity %d to the repository, want 0 (not an update field)", repo.updatedProduct.Quantity)
	}
	if !strings.Contains(rec.Body.String(), `"quantity":7`) {
		t.Errorf("body = %s, want stored quantity 7", rec.Body.String())
	}
}

// TestUpdateMinimumStock covers the optional PUT field: a missing or null
// minimum_stock reaches the repository as nil (keep the stored value), 0 and
// positive values are passed through, and negative values are rejected.
func TestUpdateMinimumStock(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		// wantArg is the minimumStock the repository must receive; nil means
		// a nil pointer. Ignored when the repository must not be called.
		wantArg      *int
		wantResponse string
	}{
		{
			name:         "missing keeps stored value",
			body:         `{"sku":"SKU001","name":"Keyboard","price":100}`,
			wantStatus:   http.StatusOK,
			wantResponse: `"minimum_stock":8`,
		},
		{
			name:         "null keeps stored value",
			body:         `{"sku":"SKU001","name":"Keyboard","price":100,"minimum_stock":null}`,
			wantStatus:   http.StatusOK,
			wantResponse: `"minimum_stock":8`,
		},
		{
			name:         "zero is a valid value",
			body:         `{"sku":"SKU001","name":"Keyboard","price":100,"minimum_stock":0}`,
			wantStatus:   http.StatusOK,
			wantArg:      intPtr(0),
			wantResponse: `"minimum_stock":0`,
		},
		{
			name:         "positive value",
			body:         `{"sku":"SKU001","name":"Keyboard","price":100,"minimum_stock":5}`,
			wantStatus:   http.StatusOK,
			wantArg:      intPtr(5),
			wantResponse: `"minimum_stock":5`,
		},
		{
			name:         "negative value is rejected",
			body:         `{"sku":"SKU001","name":"Keyboard","price":100,"minimum_stock":-1}`,
			wantStatus:   http.StatusBadRequest,
			wantResponse: `"message":"Invalid product"`,
		},
		{
			name:         "non-integer value is rejected",
			body:         `{"sku":"SKU001","name":"Keyboard","price":100,"minimum_stock":"5"}`,
			wantStatus:   http.StatusBadRequest,
			wantResponse: `"message":"Invalid JSON request"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{storedMinimumStock: 8, storedQuantity: 4}
			handler := newTestHandler(repo)

			req := httptest.NewRequest(http.MethodPut, "/api/v1/products/1", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.Update(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantResponse) {
				t.Errorf("body = %s, want it to contain %s", rec.Body.String(), tt.wantResponse)
			}

			if tt.wantStatus != http.StatusOK {
				if repo.updatedProduct != nil {
					t.Fatal("repository Update must not be called for a rejected request")
				}
				return
			}
			if repo.updatedProduct == nil {
				t.Fatal("repository Update was not called")
			}
			switch {
			case tt.wantArg == nil && repo.updateMinimumStock != nil:
				t.Errorf("repository received minimumStock %d, want nil", *repo.updateMinimumStock)
			case tt.wantArg != nil && repo.updateMinimumStock == nil:
				t.Errorf("repository received nil minimumStock, want %d", *tt.wantArg)
			case tt.wantArg != nil && *repo.updateMinimumStock != *tt.wantArg:
				t.Errorf("repository received minimumStock %d, want %d", *repo.updateMinimumStock, *tt.wantArg)
			}
			if !strings.Contains(rec.Body.String(), `"quantity":4`) {
				t.Errorf("body = %s, want stored quantity 4", rec.Body.String())
			}
		})
	}
}

// TestCreateMinimumStock covers the optional POST field: missing or null
// means 0, a value is passed through, and a negative value is rejected.
func TestCreateMinimumStock(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantStatus   int
		wantStored   int
		wantResponse string
	}{
		{"missing means zero", `{"sku":"SKU010","name":"Mouse","price":25,"quantity":3}`, http.StatusCreated, 0, `"minimum_stock":0`},
		{"null means zero", `{"sku":"SKU010","name":"Mouse","price":25,"quantity":3,"minimum_stock":null}`, http.StatusCreated, 0, `"minimum_stock":0`},
		{"explicit zero", `{"sku":"SKU010","name":"Mouse","price":25,"quantity":3,"minimum_stock":0}`, http.StatusCreated, 0, `"minimum_stock":0`},
		{"positive value", `{"sku":"SKU010","name":"Mouse","price":25,"quantity":3,"minimum_stock":2}`, http.StatusCreated, 2, `"minimum_stock":2`},
		{"negative value is rejected", `{"sku":"SKU010","name":"Mouse","price":25,"quantity":3,"minimum_stock":-1}`, http.StatusBadRequest, 0, `"message":"Invalid product"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{createID: 10}
			handler := newTestHandler(repo)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/products", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.Create(rec, withTestUser(req))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantResponse) {
				t.Errorf("body = %s, want it to contain %s", rec.Body.String(), tt.wantResponse)
			}
			if tt.wantStatus != http.StatusCreated {
				if repo.createdProduct != nil {
					t.Fatal("repository Create must not be called for a rejected request")
				}
				return
			}
			if repo.createdProduct == nil {
				t.Fatal("repository Create was not called")
			}
			if repo.createdProduct.MinimumStock != tt.wantStored {
				t.Errorf("repository received minimum stock %d, want %d", repo.createdProduct.MinimumStock, tt.wantStored)
			}
		})
	}
}

func intPtr(v int) *int { return &v }

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
			wantBody:   `"message":"invalid product ID"`,
		},
		{
			name:       "zero ID",
			method:     http.MethodDelete,
			path:       "/api/v1/products/0",
			wantStatus: http.StatusBadRequest,
			wantBody:   `"message":"invalid product ID"`,
		},
		{
			name:       "product not found",
			method:     http.MethodDelete,
			path:       "/api/v1/products/99",
			deleteErr:  repository.ErrProductNotFound,
			wantStatus: http.StatusNotFound,
			wantBody:   `"message":"Product not found"`,
		},
		{
			name:       "repository internal error",
			method:     http.MethodDelete,
			path:       "/api/v1/products/1",
			deleteErr:  errors.New("database connection failed"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `"message":"Internal server error"`,
		},
		{
			name:       "wrong HTTP method",
			method:     http.MethodPost,
			path:       "/api/v1/products/1",
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   `"message":"method not allowed"`,
			wantAllow:  http.MethodDelete,
		},
		{
			name:       "extra path segment",
			method:     http.MethodDelete,
			path:       "/api/v1/products/1/stock-in",
			wantStatus: http.StatusBadRequest,
			wantBody:   `"message":"invalid product ID"`,
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
			wantBody:   `"message":"invalid product ID"`,
		},
		{
			name:       "zero quantity",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       `{"quantity":0}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"message":"quantity must be greater than 0"`,
		},
		{
			name:       "negative quantity",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       `{"quantity":-2}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"message":"quantity must be greater than 0"`,
		},
		{
			name:       "invalid JSON",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       `{"quantity":`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `"message":"Invalid JSON body"`,
		},
		{
			name:       "product not found",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       `{"quantity":10}`,
			repoErr:    repository.ErrProductNotFound,
			wantStatus: http.StatusNotFound,
			wantBody:   `"message":"Product not found"`,
		},
		{
			name:       "repository error",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       `{"quantity":10}`,
			repoErr:    errors.New("database connection failed"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `"message":"Internal server error"`,
		},
		{
			name:       "wrong HTTP method",
			method:     http.MethodPut,
			path:       "/api/v1/products/1/stock-in",
			body:       `{"quantity":10}`,
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   `"message":"method not allowed"`,
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
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.StockIn(rec, withTestUser(req))

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
			repoErr:        repository.ErrInsufficientStock,
			wantStatus:     http.StatusConflict,
			wantBodyString: "Insufficient stock",
		},
		{
			name:           "product not found",
			method:         http.MethodPost,
			path:           "/api/v1/products/1/stock-out",
			body:           `{"quantity":3}`,
			repoErr:        repository.ErrProductNotFound,
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
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.StockOut(rec, withTestUser(req))

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

// TestStockChangesUseAuthenticatedActor covers how Create, StockIn and
// StockOut obtain the actor: from the user AuthMiddleware put in the context,
// passed unchanged to the repository; without a user the request is rejected
// with 401 before any repository call.
func TestStockChangesUseAuthenticatedActor(t *testing.T) {
	product := func() *domain.Product {
		return &domain.Product{ID: 1, SKU: "SKU001", Name: "Keyboard", Price: 100, Quantity: 10}
	}

	operations := []struct {
		name       string
		path       string
		body       string
		serve      func(h *ProductHandler) http.HandlerFunc
		wantStatus int
	}{
		{"Create", "/api/v1/products", `{"sku":"SKU001","name":"Keyboard","price":100,"quantity":5}`,
			func(h *ProductHandler) http.HandlerFunc { return h.Create }, http.StatusCreated},
		{"StockIn", "/api/v1/products/1/stock-in", `{"quantity":3}`,
			func(h *ProductHandler) http.HandlerFunc { return h.StockIn }, http.StatusOK},
		{"StockOut", "/api/v1/products/1/stock-out", `{"quantity":3}`,
			func(h *ProductHandler) http.HandlerFunc { return h.StockOut }, http.StatusOK},
	}

	newRequest := func(path, body string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	for _, op := range operations {
		t.Run(op.name+" without authenticated user", func(t *testing.T) {
			repo := &fakeProductRepository{createID: 9, stockInProduct: product(), stockOutProduct: product()}
			rec := httptest.NewRecorder()

			op.serve(newTestHandler(repo))(rec, newRequest(op.path, op.body))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body = %s", rec.Code, rec.Body.String())
			}
			wantBody := `{"success":false,"message":"authentication required","data":null}`
			if got := strings.TrimSpace(rec.Body.String()); got != wantBody {
				t.Errorf("body = %s, want %s", got, wantBody)
			}
			if repo.stockCalls != 0 {
				t.Fatal("repository must not be called without an authenticated user")
			}
		})

		t.Run(op.name+" passes the authenticated user as actor", func(t *testing.T) {
			repo := &fakeProductRepository{createID: 9, stockInProduct: product(), stockOutProduct: product()}
			rec := httptest.NewRecorder()

			op.serve(newTestHandler(repo))(rec, withUserID(newRequest(op.path, op.body), 7))

			if rec.Code != op.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, op.wantStatus, rec.Body.String())
			}
			if repo.stockCalls != 1 || repo.actorID != 7 {
				t.Errorf("repository calls = %d with actor %d, want 1 call with actor 7", repo.stockCalls, repo.actorID)
			}
		})

		t.Run(op.name+" maps a vanished actor to 401", func(t *testing.T) {
			repo := &fakeProductRepository{
				createErr:   repository.ErrInvalidActor,
				stockInErr:  repository.ErrInvalidActor,
				stockOutErr: repository.ErrInvalidActor,
			}
			rec := httptest.NewRecorder()

			op.serve(newTestHandler(repo))(rec, withTestUser(newRequest(op.path, op.body)))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body = %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"message":"authentication required"`) {
				t.Errorf("body = %s, want authentication required", rec.Body.String())
			}
		})
	}
}

// TestStockChangesKeepInputErrorsBeforeAuth keeps the existing error order:
// malformed requests are rejected with 400 even when no user is present.
func TestStockChangesKeepInputErrorsBeforeAuth(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		body  string
		serve func(h *ProductHandler) http.HandlerFunc
		want  string
	}{
		{"Create invalid JSON", "/api/v1/products", `{"sku":`,
			func(h *ProductHandler) http.HandlerFunc { return h.Create }, `"message":"Invalid JSON request"`},
		{"StockIn invalid ID", "/api/v1/products/abc/stock-in", `{"quantity":3}`,
			func(h *ProductHandler) http.HandlerFunc { return h.StockIn }, `"message":"invalid product ID"`},
		{"StockOut invalid JSON", "/api/v1/products/1/stock-out", `{"quantity":`,
			func(h *ProductHandler) http.HandlerFunc { return h.StockOut }, `"message":"Invalid JSON body"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{}
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			tt.serve(newTestHandler(repo))(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Errorf("body = %s, want it to contain %s", rec.Body.String(), tt.want)
			}
		})
	}
}
