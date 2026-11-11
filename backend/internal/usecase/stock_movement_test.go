package usecase

import (
	"context"
	"errors"
	"math"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/repository"
)

type fakeStockMovementRepository struct {
	movements []domain.StockMovement
	total     int64
	err       error

	listByProductCalled bool
	listCalled          bool
	gotProductID        int64
	gotPage, gotLimit   int
}

func (f *fakeStockMovementRepository) ListByProduct(
	ctx context.Context,
	productID int64,
	page, limit int,
) ([]domain.StockMovement, int64, error) {
	f.listByProductCalled = true
	f.gotProductID, f.gotPage, f.gotLimit = productID, page, limit
	return f.movements, f.total, f.err
}

func (f *fakeStockMovementRepository) List(
	ctx context.Context,
	page, limit int,
) ([]domain.StockMovement, int64, error) {
	f.listCalled = true
	f.gotPage, f.gotLimit = page, limit
	return f.movements, f.total, f.err
}

func sampleMovements() []domain.StockMovement {
	productID, userID := int64(7), int64(3)
	return []domain.StockMovement{
		{ID: 2, ProductID: &productID, ProductSKU: "SKU007", Type: repository.MovementTypeOut, QuantityChange: -2, QuantityBefore: 5, QuantityAfter: 3, UserID: &userID},
		{ID: 1, ProductID: &productID, ProductSKU: "SKU007", Type: repository.MovementTypeInitial, QuantityChange: 5, QuantityBefore: 0, QuantityAfter: 5, UserID: &userID},
	}
}

func TestStockMovementListByProduct_ExistingProduct(t *testing.T) {
	products := &fakeProductRepository{product: &domain.Product{ID: 7, SKU: "SKU007"}}
	movements := &fakeStockMovementRepository{movements: sampleMovements(), total: 12}
	uc := NewStockMovementUseCase(products, movements)

	got, total, err := uc.ListByProduct(context.Background(), 7, 2, 5)
	if err != nil {
		t.Fatalf("ListByProduct: %v", err)
	}
	if !products.called {
		t.Error("product existence was not checked")
	}
	if !movements.listByProductCalled || movements.listCalled {
		t.Fatal("expected only StockMovementRepository.ListByProduct to be called")
	}
	if movements.gotProductID != 7 || movements.gotPage != 2 || movements.gotLimit != 5 {
		t.Errorf("repository received product=%d page=%d limit=%d, want 7, 2, 5",
			movements.gotProductID, movements.gotPage, movements.gotLimit)
	}
	if total != 12 || len(got) != 2 || got[0].ID != 2 || got[1].ID != 1 {
		t.Errorf("result = %+v total %d, want the repository page unchanged with total 12", got, total)
	}
}

func TestStockMovementListByProduct_ProductNotFound(t *testing.T) {
	products := &fakeProductRepository{err: repository.ErrProductNotFound}
	movements := &fakeStockMovementRepository{}
	uc := NewStockMovementUseCase(products, movements)

	_, _, err := uc.ListByProduct(context.Background(), 99, 1, 10)
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("error = %v, want ErrProductNotFound", err)
	}
	if movements.listByProductCalled || movements.listCalled {
		t.Fatal("movement repository must not be called for a missing product")
	}
}

func TestStockMovementListByProduct_InvalidInput(t *testing.T) {
	tests := []struct {
		name        string
		productID   int64
		page, limit int
		wantErr     error
	}{
		{"zero product ID", 0, 1, 10, ErrInvalidID},
		{"negative product ID", -1, 1, 10, ErrInvalidID},
		{"zero page", 7, 0, 10, ErrInvalidPage},
		{"negative page", 7, -1, 10, ErrInvalidPage},
		{"page beyond INTEGER range", 7, math.MaxInt32 + 1, 10, ErrInvalidPage},
		{"zero limit", 7, 1, 0, ErrInvalidLimit},
		{"limit above 100", 7, 1, 101, ErrInvalidLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			products := &fakeProductRepository{product: &domain.Product{ID: 7}}
			movements := &fakeStockMovementRepository{}
			uc := NewStockMovementUseCase(products, movements)

			_, _, err := uc.ListByProduct(context.Background(), tt.productID, tt.page, tt.limit)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if products.called || movements.listByProductCalled {
				t.Fatal("no repository may be called for invalid input")
			}
		})
	}
}

func TestStockMovementListByProduct_RepositoryError(t *testing.T) {
	dbErr := errors.New("connection reset")
	products := &fakeProductRepository{product: &domain.Product{ID: 7}}
	uc := NewStockMovementUseCase(products, &fakeStockMovementRepository{err: dbErr})

	if _, _, err := uc.ListByProduct(context.Background(), 7, 1, 10); !errors.Is(err, dbErr) {
		t.Fatalf("error = %v, want the repository error", err)
	}
}

func TestStockMovementList(t *testing.T) {
	products := &fakeProductRepository{}
	movements := &fakeStockMovementRepository{movements: sampleMovements(), total: 40}
	uc := NewStockMovementUseCase(products, movements)

	got, total, err := uc.List(context.Background(), 3, 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !movements.listCalled || movements.listByProductCalled {
		t.Fatal("expected only StockMovementRepository.List to be called")
	}
	if products.called {
		t.Error("global history must not look up a product")
	}
	if movements.gotPage != 3 || movements.gotLimit != 20 {
		t.Errorf("repository received page=%d limit=%d, want 3 and 20", movements.gotPage, movements.gotLimit)
	}
	if total != 40 || len(got) != 2 {
		t.Errorf("result = %d movements total %d, want 2 and 40", len(got), total)
	}
}

func TestStockMovementList_InvalidPagination(t *testing.T) {
	tests := []struct {
		name        string
		page, limit int
		wantErr     error
	}{
		{"zero page", 0, 10, ErrInvalidPage},
		{"page beyond INTEGER range", math.MaxInt32 + 1, 10, ErrInvalidPage},
		{"zero limit", 1, 0, ErrInvalidLimit},
		{"limit above 100", 1, 101, ErrInvalidLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			movements := &fakeStockMovementRepository{}
			uc := NewStockMovementUseCase(&fakeProductRepository{}, movements)

			if _, _, err := uc.List(context.Background(), tt.page, tt.limit); !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if movements.listCalled {
				t.Fatal("repository must not be called for invalid pagination")
			}
		})
	}
}

func TestStockMovementPaginationBoundsAccepted(t *testing.T) {
	movements := &fakeStockMovementRepository{}
	uc := NewStockMovementUseCase(&fakeProductRepository{}, movements)

	for _, p := range []struct{ page, limit int }{{1, 1}, {1, 100}, {math.MaxInt32, 100}} {
		if _, _, err := uc.List(context.Background(), p.page, p.limit); err != nil {
			t.Errorf("page=%d limit=%d: unexpected error %v", p.page, p.limit, err)
		}
	}
}
