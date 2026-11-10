package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gostock/internal/domain"
)

type fakeProductRepository struct {
	product *domain.Product
	err     error

	called         bool
	createCalled   bool
	updateCalled   bool
	listCalled     bool
	stockInCalled  bool
	stockOutCalled bool

	// storedQuantity simulates the stock the database reports from Update.
	storedQuantity int
}

func (f *fakeProductRepository) GetByID(
	ctx context.Context,
	id int64,
) (*domain.Product, error) {
	f.called = true
	return f.product, f.err
}

func (f *fakeProductRepository) Create(
	ctx context.Context,
	product *domain.Product,
) error {
	f.createCalled = true
	return nil
}

func (f *fakeProductRepository) Update(
	ctx context.Context,
	product *domain.Product,
) error {
	f.updateCalled = true
	product.Quantity = f.storedQuantity
	return nil
}

func (f *fakeProductRepository) Delete(
	ctx context.Context,
	id int64,
) error {
	return nil
}

func (f *fakeProductRepository) List(
	ctx context.Context,
	query string,
	page, limit int,
	sortBy, order string,
) ([]domain.Product, int64, error) {
	f.listCalled = true
	return nil, 0, nil
}

func (f *fakeProductRepository) StockIn(
	ctx context.Context,
	id int64,
	quantity int,
) (*domain.Product, error) {
	f.stockInCalled = true
	return nil, nil
}

func (f *fakeProductRepository) StockOut(
	ctx context.Context,
	id int64,
	quantity int,
) (*domain.Product, error) {
	f.stockOutCalled = true
	return nil, nil
}
func TestGetByIDSuccess(t *testing.T) {
	expected := &domain.Product{
		ID:       1,
		SKU:      "SKU001",
		Name:     "Keyboard",
		Price:    500000,
		Quantity: 10,
	}

	repo := &fakeProductRepository{product: expected}
	uc := NewProductUseCase(repo)

	got, err := uc.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got != expected {
		t.Fatalf("expected product %v, got %v", expected, got)
	}

	if !repo.called {
		t.Fatal("expected repository to be called")
	}
}

func TestGetByIDInvalidID(t *testing.T) {
	repo := &fakeProductRepository{}
	uc := NewProductUseCase(repo)

	_, err := uc.GetByID(context.Background(), 0)
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("expected ErrInvalidID, got %v", err)
	}

	if repo.called {
		t.Fatal("repository should not be called for invalid ID")
	}
}

func TestCreateInvalidProduct(t *testing.T) {
	tests := []struct {
		name    string
		product *domain.Product
	}{
		{
			name:    "nil product",
			product: nil,
		},
		{
			name: "empty SKU",
			product: &domain.Product{
				SKU:      " ",
				Name:     "Keyboard",
				Price:    100,
				Quantity: 10,
			},
		},
		{
			name: "negative price",
			product: &domain.Product{
				SKU:      "SKU001",
				Name:     "Keyboard",
				Price:    -1,
				Quantity: 10,
			},
		},
		{
			name: "negative quantity",
			product: &domain.Product{
				SKU:      "SKU001",
				Name:     "Keyboard",
				Price:    100,
				Quantity: -1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{}
			uc := NewProductUseCase(repo)

			err := uc.Create(context.Background(), tt.product)

			if !errors.Is(err, ErrInvalidProduct) {
				t.Fatalf("expected ErrInvalidProduct, got %v", err)
			}
			if repo.createCalled {
				t.Fatal("repository Create should not be called for invalid product")
			}
		})
	}
}

func TestUpdateInvalidID(t *testing.T) {
	repo := &fakeProductRepository{}
	uc := NewProductUseCase(repo)

	product := &domain.Product{
		ID:       0,
		SKU:      "SKU001",
		Name:     "Keyboard",
		Price:    100,
		Quantity: 10,
	}

	err := uc.Update(context.Background(), product)

	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("expected ErrInvalidID, got %v", err)
	}
	if repo.updateCalled {
		t.Fatal("repository Update should not be called for invalid ID")
	}
}

// TestUpdatePreservesStoredQuantity guards CR-15: a stale quantity on the
// product passed to Update must not survive; the result carries the stock the
// repository reports, and quantity is not validated as an update field.
func TestUpdatePreservesStoredQuantity(t *testing.T) {
	repo := &fakeProductRepository{storedQuantity: 7}
	uc := NewProductUseCase(repo)

	product := &domain.Product{
		ID:       1,
		SKU:      "SKU001",
		Name:     "Renamed",
		Price:    100,
		Quantity: 10, // stale value from an old form
	}

	if err := uc.Update(context.Background(), product); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("repository Update was not called")
	}
	if product.Quantity != 7 {
		t.Errorf("quantity after Update = %d, want stored 7 (stale 10 must not survive)", product.Quantity)
	}
}

func TestUpdateInvalidProduct(t *testing.T) {
	tests := []struct {
		name    string
		product domain.Product
	}{
		{"empty SKU", domain.Product{ID: 1, SKU: " ", Name: "Keyboard", Price: 100}},
		{"empty name", domain.Product{ID: 1, SKU: "SKU001", Name: "", Price: 100}},
		{"SKU too long", domain.Product{ID: 1, SKU: strings.Repeat("A", 51), Name: "Keyboard", Price: 100}},
		{"name too long", domain.Product{ID: 1, SKU: "SKU001", Name: strings.Repeat("N", 201), Price: 100}},
		{"negative price", domain.Product{ID: 1, SKU: "SKU001", Name: "Keyboard", Price: -0.01}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{}
			uc := NewProductUseCase(repo)

			product := tt.product
			err := uc.Update(context.Background(), &product)

			if !errors.Is(err, ErrInvalidProduct) {
				t.Fatalf("expected ErrInvalidProduct, got %v", err)
			}
			if repo.updateCalled {
				t.Fatal("repository Update should not be called for an invalid product")
			}
		})
	}
}

func TestListInvalidParameters(t *testing.T) {
	tests := []struct {
		name   string
		page   int
		limit  int
		sortBy string
		order  string
	}{
		{
			name:  "page less than 1",
			page:  0,
			limit: 10,
		},
		{
			name:  "limit less than 1",
			page:  1,
			limit: 0,
		},
		{
			name:  "limit greater than 100",
			page:  1,
			limit: 101,
		},
		{
			name:   "invalid sort field",
			page:   1,
			limit:  10,
			sortBy: "sku",
			order:  "asc",
		},
		{
			name:   "invalid sort order",
			page:   1,
			limit:  10,
			sortBy: "id",
			order:  "random",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{}
			uc := NewProductUseCase(repo)

			_, _, err := uc.List(
				context.Background(),
				"",
				tt.page,
				tt.limit,
				tt.sortBy,
				tt.order,
			)

			if !errors.Is(err, ErrInvalidProduct) {
				t.Fatalf("expected ErrInvalidProduct, got %v", err)
			}
			if repo.listCalled {
				t.Fatal("repository List should not be called for invalid parameters")
			}
		})
	}
}

func TestStockInInvalidParameters(t *testing.T) {
	tests := []struct {
		name     string
		id       int64
		quantity int
		wantErr  error
	}{
		{
			name:     "invalid product ID",
			id:       0,
			quantity: 5,
			wantErr:  ErrInvalidID,
		},
		{
			name:     "zero quantity",
			id:       1,
			quantity: 0,
			wantErr:  ErrInvalidQuantity,
		},
		{
			name:     "negative quantity",
			id:       1,
			quantity: -5,
			wantErr:  ErrInvalidQuantity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{}
			uc := NewProductUseCase(repo)

			_, err := uc.StockIn(
				context.Background(),
				tt.id,
				tt.quantity,
			)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
			if repo.stockInCalled {
				t.Fatal("repository StockIn should not be called for invalid parameters")
			}
		})
	}
}

func TestStockOutInvalidParameters(t *testing.T) {
	tests := []struct {
		name     string
		id       int64
		quantity int
		wantErr  error
	}{
		{
			name:     "invalid product ID",
			id:       0,
			quantity: 5,
			wantErr:  ErrInvalidID,
		},
		{
			name:     "zero quantity",
			id:       1,
			quantity: 0,
			wantErr:  ErrInvalidQuantity,
		},
		{
			name:     "negative quantity",
			id:       1,
			quantity: -5,
			wantErr:  ErrInvalidQuantity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{}
			uc := NewProductUseCase(repo)

			_, err := uc.StockOut(
				context.Background(),
				tt.id,
				tt.quantity,
			)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
			if repo.stockOutCalled {
				t.Fatal("repository StockOut should not be called for invalid parameters")
			}
		})
	}
}
