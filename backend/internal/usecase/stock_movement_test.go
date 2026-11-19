package usecase

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/dto"
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
	if total != 12 {
		t.Errorf("total = %d, want the repository total 12", total)
	}
	if !reflect.DeepEqual(got, wantSampleResponses()) {
		t.Errorf("result = %+v, want the repository page as DTOs in the same order", got)
	}
}

// wantSampleResponses is sampleMovements as response DTOs, written out by hand.
func wantSampleResponses() []dto.StockMovementResponse {
	productID, userID := int64(7), int64(3)
	return []dto.StockMovementResponse{
		{ID: 2, ProductID: &productID, ProductSKU: "SKU007", MovementType: "out", QuantityChange: -2, QuantityBefore: 5, QuantityAfter: 3, UserID: &userID},
		{ID: 1, ProductID: &productID, ProductSKU: "SKU007", MovementType: "initial", QuantityChange: 5, QuantityBefore: 0, QuantityAfter: 5, UserID: &userID},
	}
}

// Both list methods return DTOs with the same rules: nil or empty pages are
// [] with the repository total kept (a page past the end still reports the
// total), nil IDs stay nil, the order is kept, and the DTOs do not share ID
// pointers with the repository's movements.
func TestStockMovementLists_DTORules(t *testing.T) {
	lists := map[string]func(uc *StockMovementUseCase) ([]dto.StockMovementResponse, int64, error){
		"ListByProduct": func(uc *StockMovementUseCase) ([]dto.StockMovementResponse, int64, error) {
			return uc.ListByProduct(context.Background(), 7, 3, 10)
		},
		"List": func(uc *StockMovementUseCase) ([]dto.StockMovementResponse, int64, error) {
			return uc.List(context.Background(), 3, 10)
		},
	}
	products := &fakeProductRepository{product: &domain.Product{ID: 7}}

	for name, list := range lists {
		t.Run(name+" nil page", func(t *testing.T) {
			got, total, err := list(NewStockMovementUseCase(products, &fakeStockMovementRepository{movements: nil, total: 25}))
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || len(got) != 0 || total != 25 {
				t.Errorf("got %#v total %d, want a non-nil empty slice and total 25", got, total)
			}
		})

		t.Run(name+" empty page", func(t *testing.T) {
			got, total, err := list(NewStockMovementUseCase(products, &fakeStockMovementRepository{movements: []domain.StockMovement{}}))
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || len(got) != 0 || total != 0 {
				t.Errorf("got %#v total %d, want a non-nil empty slice and total 0", got, total)
			}
		})

		t.Run(name+" deleted product and missing actor", func(t *testing.T) {
			repo := &fakeStockMovementRepository{movements: []domain.StockMovement{
				{ID: 9, ProductSKU: "GONE-1", Type: repository.MovementTypeIn, QuantityChange: 4, QuantityBefore: 1, QuantityAfter: 5},
			}, total: 1}
			got, total, err := list(NewStockMovementUseCase(products, repo))
			if err != nil {
				t.Fatal(err)
			}
			want := []dto.StockMovementResponse{
				{ID: 9, ProductSKU: "GONE-1", MovementType: "in", QuantityChange: 4, QuantityBefore: 1, QuantityAfter: 5},
			}
			if !reflect.DeepEqual(got, want) || total != 1 {
				t.Errorf("got %+v total %d, want %+v total 1", got, total, want)
			}
			if got[0].ProductID != nil || got[0].UserID != nil {
				t.Errorf("ProductID/UserID = %v/%v, want nil/nil", got[0].ProductID, got[0].UserID)
			}
		})

		t.Run(name+" order and independent pointers", func(t *testing.T) {
			source := sampleMovements()
			got, _, err := list(NewStockMovementUseCase(products, &fakeStockMovementRepository{movements: source, total: 2}))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, wantSampleResponses()) {
				t.Fatalf("got %+v, want %+v", got, wantSampleResponses())
			}
			if got[0].ProductID == source[0].ProductID || got[0].UserID == source[0].UserID || got[0].ProductID == got[1].ProductID {
				t.Error("DTO ID pointers are shared with the repository data or with each other")
			}
			*source[0].ProductID = 100
			if *got[0].ProductID != 7 || *got[1].ProductID != 7 {
				t.Errorf("changing repository data changed the DTOs: %d, %d", *got[0].ProductID, *got[1].ProductID)
			}
		})
	}
}

// On every error both list methods return no items and a zero total.
func TestStockMovementLists_ErrorResults(t *testing.T) {
	dbErr := errors.New("connection reset")
	sample := &fakeStockMovementRepository{movements: sampleMovements(), total: 2}
	failing := &fakeStockMovementRepository{movements: sampleMovements(), total: 2, err: dbErr}

	check := func(name string, got []dto.StockMovementResponse, total int64, err, wantErr error) {
		t.Helper()
		if !errors.Is(err, wantErr) {
			t.Errorf("%s: error = %v, want %v", name, err, wantErr)
		}
		if got != nil || total != 0 {
			t.Errorf("%s: got %#v total %d, want nil and 0", name, got, total)
		}
	}

	got, total, err := NewStockMovementUseCase(&fakeProductRepository{product: &domain.Product{ID: 7}}, failing).ListByProduct(context.Background(), 7, 1, 10)
	check("ListByProduct repository error", got, total, err, dbErr)
	got, total, err = NewStockMovementUseCase(&fakeProductRepository{err: repository.ErrProductNotFound}, sample).ListByProduct(context.Background(), 7, 1, 10)
	check("ListByProduct missing product", got, total, err, ErrProductNotFound)
	got, total, err = NewStockMovementUseCase(&fakeProductRepository{}, sample).ListByProduct(context.Background(), 0, 1, 10)
	check("ListByProduct invalid ID", got, total, err, ErrInvalidID)
	got, total, err = NewStockMovementUseCase(&fakeProductRepository{}, failing).List(context.Background(), 1, 10)
	check("List repository error", got, total, err, dbErr)
	got, total, err = NewStockMovementUseCase(&fakeProductRepository{}, sample).List(context.Background(), 1, 0)
	check("List invalid limit", got, total, err, ErrInvalidLimit)
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
	if total != 40 {
		t.Errorf("total = %d, want the repository total 40", total)
	}
	if !reflect.DeepEqual(got, wantSampleResponses()) {
		t.Errorf("result = %+v, want the repository page as DTOs in the same order", got)
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
