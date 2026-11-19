package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/dto"
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
	// storedMinimumStock simulates the threshold already in the database.
	storedMinimumStock int
	// updateMinimumStock records the minimumStock argument passed to Update.
	updateMinimumStock *int
	// createdProduct records the product passed to Create.
	createdProduct domain.Product
	// actorID records the actor passed to Create, StockIn or StockOut.
	actorID int64

	// createID is the ID Create assigns, as the database does; createErr,
	// updateErr and stockErr are returned by Create, Update and
	// StockIn/StockOut.
	createID  int64
	createErr error
	updateErr error
	stockErr  error
	// listProducts and listTotal are returned by List, with listErr.
	listProducts []domain.Product
	listTotal    int64
	listErr      error
	// stockProduct is returned by StockIn and StockOut.
	stockProduct *domain.Product
}

// testActorID is the authenticated user used by stock tests.
const testActorID int64 = 42

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
	actorID int64,
) error {
	f.createCalled = true
	f.createdProduct = *product
	f.actorID = actorID
	if f.createErr != nil {
		return f.createErr
	}
	product.ID = f.createID
	return nil
}

func (f *fakeProductRepository) Update(
	ctx context.Context,
	product *domain.Product,
	minimumStock *int,
) error {
	f.updateCalled = true
	f.updateMinimumStock = minimumStock
	if f.updateErr != nil {
		return f.updateErr
	}
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
	return nil
}

func (f *fakeProductRepository) List(
	ctx context.Context,
	query string,
	page, limit int,
	sortBy, order string,
) ([]domain.Product, int64, error) {
	f.listCalled = true
	return f.listProducts, f.listTotal, f.listErr
}

func (f *fakeProductRepository) StockIn(
	ctx context.Context,
	id int64,
	quantity int,
	actorID int64,
) (*domain.Product, error) {
	f.stockInCalled = true
	f.actorID = actorID
	return f.stockProduct, f.stockErr
}

func (f *fakeProductRepository) StockOut(
	ctx context.Context,
	id int64,
	quantity int,
	actorID int64,
) (*domain.Product, error) {
	f.stockOutCalled = true
	f.actorID = actorID
	return f.stockProduct, f.stockErr
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

	want := dto.ProductResponse{ID: 1, SKU: "SKU001", Name: "Keyboard", Price: 500000, Quantity: 10, MinimumStock: 0}
	if got == nil || *got != want {
		t.Fatalf("expected product %+v, got %+v", want, got)
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

// The input is a value since Step 5.4A, so the former "nil product" case
// cannot occur any more.
func TestCreateInvalidProduct(t *testing.T) {
	tests := []struct {
		name string
		in   dto.CreateProductInput
	}{
		{
			name: "empty SKU",
			in: dto.CreateProductInput{
				SKU:      " ",
				Name:     "Keyboard",
				Price:    100,
				Quantity: 10,
			},
		},
		{
			name: "negative price",
			in: dto.CreateProductInput{
				SKU:      "SKU001",
				Name:     "Keyboard",
				Price:    -1,
				Quantity: 10,
			},
		},
		{
			name: "negative quantity",
			in: dto.CreateProductInput{
				SKU:      "SKU001",
				Name:     "Keyboard",
				Price:    100,
				Quantity: -1,
			},
		},
		{
			name: "negative minimum stock",
			in: dto.CreateProductInput{
				SKU:          "SKU001",
				Name:         "Keyboard",
				Price:        100,
				Quantity:     10,
				MinimumStock: intPtr(-1),
			},
		},
		{
			name: "minimum stock above INTEGER range",
			in: dto.CreateProductInput{
				SKU:          "SKU001",
				Name:         "Keyboard",
				Price:        100,
				Quantity:     10,
				MinimumStock: intPtr(math.MaxInt32 + 1),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{}
			uc := NewProductUseCase(repo)

			_, err := uc.Create(context.Background(), tt.in, testActorID)

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

	in := dto.UpdateProductInput{
		ID:    0,
		SKU:   "SKU001",
		Name:  "Keyboard",
		Price: 100,
	}

	_, err := uc.Update(context.Background(), in)

	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("expected ErrInvalidID, got %v", err)
	}
	if repo.updateCalled {
		t.Fatal("repository Update should not be called for invalid ID")
	}

	// The ID is checked first: with an invalid ID and invalid details the error
	// is still ErrInvalidID (Step 5.4B pins this order).
	_, err = uc.Update(context.Background(), dto.UpdateProductInput{ID: -1, SKU: " ", Name: "", Price: -1, MinimumStock: intPtr(-1)})
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("invalid ID and invalid details: expected ErrInvalidID, got %v", err)
	}
}

// TestUpdatePreservesStoredQuantity guards CR-15: Update can never carry a
// quantity. Since Step 5.4B the input has no Quantity field at all (a stale
// form value used to be passable on the *domain.Product), so the product
// handed to the repository has quantity 0, which the repository does not
// write, and the result carries the stock the repository reports.
func TestUpdatePreservesStoredQuantity(t *testing.T) {
	repo := &updateRecordingRepository{fakeProductRepository: fakeProductRepository{storedQuantity: 7}}
	uc := NewProductUseCase(repo)

	in := dto.UpdateProductInput{
		ID:    1,
		SKU:   "SKU001",
		Name:  "Renamed",
		Price: 100,
	}

	got, err := uc.Update(context.Background(), in)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if len(repo.updated) != 1 {
		t.Fatalf("repository Update called %d times, want 1", len(repo.updated))
	}
	if repo.updated[0].Quantity != 0 {
		t.Errorf("repository received quantity %d, want 0 (Update carries no quantity)", repo.updated[0].Quantity)
	}
	if got.Quantity != 7 {
		t.Errorf("quantity after Update = %d, want stored 7", got.Quantity)
	}
}

func TestUpdateInvalidProduct(t *testing.T) {
	tests := []struct {
		name string
		in   dto.UpdateProductInput
	}{
		{"empty SKU", dto.UpdateProductInput{ID: 1, SKU: " ", Name: "Keyboard", Price: 100}},
		{"empty name", dto.UpdateProductInput{ID: 1, SKU: "SKU001", Name: "", Price: 100}},
		{"SKU too long", dto.UpdateProductInput{ID: 1, SKU: strings.Repeat("A", 51), Name: "Keyboard", Price: 100}},
		{"name too long", dto.UpdateProductInput{ID: 1, SKU: "SKU001", Name: strings.Repeat("N", 201), Price: 100}},
		{"negative price", dto.UpdateProductInput{ID: 1, SKU: "SKU001", Name: "Keyboard", Price: -0.01}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{}
			uc := NewProductUseCase(repo)

			_, err := uc.Update(context.Background(), tt.in)

			if !errors.Is(err, ErrInvalidProduct) {
				t.Fatalf("expected ErrInvalidProduct, got %v", err)
			}
			if repo.updateCalled {
				t.Fatal("repository Update should not be called for an invalid product")
			}
		})
	}
}

func intPtr(v int) *int { return &v }

// TestCreateMinimumStock: an omitted minimum stock means 0 (decided by the use
// case since Step 5.4A); 0 and other valid values are stored as given.
func TestCreateMinimumStock(t *testing.T) {
	cases := map[string]*int{"omitted": nil, "zero": intPtr(0), "positive": intPtr(5), "INTEGER maximum": intPtr(math.MaxInt32)}
	for label, input := range cases {
		minimum := 0
		if input != nil {
			minimum = *input
		}
		repo := &fakeProductRepository{}
		uc := NewProductUseCase(repo)

		in := dto.CreateProductInput{SKU: "SKU001", Name: "Keyboard", Price: 100, Quantity: 10, MinimumStock: input}
		got, err := uc.Create(context.Background(), in, testActorID)
		if err != nil {
			t.Fatalf("%s minimum stock: Create returned error: %v", label, err)
		}
		if !repo.createCalled {
			t.Fatalf("%s minimum stock: repository Create was not called", label)
		}
		if repo.createdProduct.MinimumStock != minimum {
			t.Errorf("%s: repository received minimum stock %d, want %d", label, repo.createdProduct.MinimumStock, minimum)
		}
		if got.MinimumStock != minimum {
			t.Errorf("%s: returned minimum stock %d, want %d", label, got.MinimumStock, minimum)
		}
	}
}

// TestUpdateMinimumStock covers the optional threshold: nil keeps the stored
// value, 0 is a valid explicit value, and the repository result is reported.
func TestUpdateMinimumStock(t *testing.T) {
	tests := []struct {
		name         string
		minimumStock *int
		wantStored   int
	}{
		{"nil keeps stored value", nil, 8},
		{"zero is a valid value", intPtr(0), 0},
		{"positive value replaces stored value", intPtr(3), 3},
		{"INTEGER maximum is accepted", intPtr(math.MaxInt32), math.MaxInt32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{storedMinimumStock: 8}
			uc := NewProductUseCase(repo)

			in := dto.UpdateProductInput{ID: 1, SKU: "SKU001", Name: "Keyboard", Price: 100, MinimumStock: tt.minimumStock}
			got, err := uc.Update(context.Background(), in)
			if err != nil {
				t.Fatalf("Update returned error: %v", err)
			}
			if !repo.updateCalled {
				t.Fatal("repository Update was not called")
			}
			if repo.updateMinimumStock != tt.minimumStock {
				t.Errorf("repository received minimumStock %v, want the caller's pointer %v", repo.updateMinimumStock, tt.minimumStock)
			}
			if got.MinimumStock != tt.wantStored {
				t.Errorf("minimum stock after Update = %d, want %d", got.MinimumStock, tt.wantStored)
			}
		})
	}
}

func TestUpdateInvalidMinimumStock(t *testing.T) {
	for _, minimum := range []int{-1, math.MaxInt32 + 1} {
		repo := &fakeProductRepository{}
		uc := NewProductUseCase(repo)

		in := dto.UpdateProductInput{ID: 1, SKU: "SKU001", Name: "Keyboard", Price: 100, MinimumStock: intPtr(minimum)}
		_, err := uc.Update(context.Background(), in)

		if !errors.Is(err, ErrInvalidProduct) {
			t.Fatalf("minimum stock %d: expected ErrInvalidProduct, got %v", minimum, err)
		}
		if repo.updateCalled {
			t.Fatalf("minimum stock %d: repository Update must not be called", minimum)
		}
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
				testActorID,
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
				testActorID,
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

// TestStockChangesRequireActor checks that Create, StockIn and StockOut reject
// a missing actor before touching the repository, and pass a valid actor
// through unchanged.
func TestStockChangesRequireActor(t *testing.T) {
	newProduct := func() dto.CreateProductInput {
		return dto.CreateProductInput{SKU: "SKU001", Name: "Keyboard", Price: 100, Quantity: 5}
	}

	operations := []struct {
		name   string
		run    func(uc *ProductUseCase, actorID int64) error
		called func(repo *fakeProductRepository) bool
	}{
		{
			name: "Create",
			run: func(uc *ProductUseCase, actorID int64) error {
				_, err := uc.Create(context.Background(), newProduct(), actorID)
				return err
			},
			called: func(repo *fakeProductRepository) bool { return repo.createCalled },
		},
		{
			name: "StockIn",
			run: func(uc *ProductUseCase, actorID int64) error {
				_, err := uc.StockIn(context.Background(), 1, 3, actorID)
				return err
			},
			called: func(repo *fakeProductRepository) bool { return repo.stockInCalled },
		},
		{
			name: "StockOut",
			run: func(uc *ProductUseCase, actorID int64) error {
				_, err := uc.StockOut(context.Background(), 1, 3, actorID)
				return err
			},
			called: func(repo *fakeProductRepository) bool { return repo.stockOutCalled },
		},
	}

	for _, op := range operations {
		for _, actorID := range []int64{0, -1} {
			t.Run(fmt.Sprintf("%s rejects actor %d", op.name, actorID), func(t *testing.T) {
				repo := &fakeProductRepository{}
				err := op.run(NewProductUseCase(repo), actorID)

				if !errors.Is(err, ErrInvalidActor) {
					t.Fatalf("error = %v, want ErrInvalidActor", err)
				}
				if op.called(repo) {
					t.Fatal("repository must not be called without a valid actor")
				}
			})
		}

		t.Run(op.name+" passes the actor to the repository", func(t *testing.T) {
			repo := &fakeProductRepository{}
			if err := op.run(NewProductUseCase(repo), testActorID); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !op.called(repo) {
				t.Fatal("repository was not called")
			}
			if repo.actorID != testActorID {
				t.Errorf("repository received actor %d, want %d", repo.actorID, testActorID)
			}
		})
	}
}

// TestStockChangesValidateInputBeforeActor keeps the existing error order:
// invalid input is reported even when the actor is also missing.
func TestStockChangesValidateInputBeforeActor(t *testing.T) {
	uc := NewProductUseCase(&fakeProductRepository{})

	if _, err := uc.Create(context.Background(), dto.CreateProductInput{SKU: "", Name: "Keyboard"}, 0); !errors.Is(err, ErrInvalidProduct) {
		t.Errorf("Create error = %v, want ErrInvalidProduct", err)
	}
	if _, err := uc.StockIn(context.Background(), 0, 3, 0); !errors.Is(err, ErrInvalidID) {
		t.Errorf("StockIn error = %v, want ErrInvalidID", err)
	}
	if _, err := uc.StockOut(context.Background(), 1, 0, 0); !errors.Is(err, ErrInvalidQuantity) {
		t.Errorf("StockOut error = %v, want ErrInvalidQuantity", err)
	}
}
