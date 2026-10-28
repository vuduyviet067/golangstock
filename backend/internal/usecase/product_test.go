package usecase

import (
	"context"
	"errors"
	"testing"

	"gostock/internal/domain"
)

type fakeProductRepository struct {
	product *domain.Product
	err     error
	called  bool
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
	return nil
}

func (f *fakeProductRepository) Update(
	ctx context.Context,
	product *domain.Product,
) error {
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
	return nil, 0, nil
}

func (f *fakeProductRepository) StockIn(
	ctx context.Context,
	id int64,
	quantity int,
) (*domain.Product, error) {
	return nil, nil
}

func (f *fakeProductRepository) StockOut(
	ctx context.Context,
	id int64,
	quantity int,
) (*domain.Product, error) {
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
		})
	}
}
