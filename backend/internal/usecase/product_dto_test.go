package usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/repository"
)

// Product use case results as response DTOs (Mapper refactor Step 3.3).
// Expected DTOs are written out by hand.

func TestProductCreate_ReturnsStoredProduct(t *testing.T) {
	repo := &fakeProductRepository{createID: 123}
	uc := NewProductUseCase(repo)

	minimum := 2
	in := dto.CreateProductInput{SKU: "  SKU010  ", Name: " Mouse ", Price: 25, Quantity: 3, MinimumStock: &minimum}
	got, err := uc.Create(context.Background(), in, testActorID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The ID is the repository's (the input has none); SKU and name are trimmed.
	want := dto.ProductResponse{ID: 123, SKU: "SKU010", Name: "Mouse", Price: 25, Quantity: 3, MinimumStock: 2}
	if got != want {
		t.Errorf("Create = %+v, want %+v", got, want)
	}
}

func TestProductUpdate_ReturnsStoredProduct(t *testing.T) {
	repo := &fakeProductRepository{storedQuantity: 5, storedMinimumStock: 4}
	uc := NewProductUseCase(repo)

	in := dto.UpdateProductInput{ID: 7, SKU: " SKU007 ", Name: "Keyboard Pro", Price: 120}
	got, err := uc.Update(context.Background(), in)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Quantity and minimum stock are what the repository stores (the input
	// has no quantity, and a nil minimum stock keeps the stored one).
	want := dto.ProductResponse{ID: 7, SKU: "SKU007", Name: "Keyboard Pro", Price: 120, Quantity: 5, MinimumStock: 4}
	if got != want {
		t.Errorf("Update = %+v, want %+v", got, want)
	}
}

// Create and Update return the zero DTO with every error, and keep the error
// mapping.
func TestProductCreateUpdate_ErrorResults(t *testing.T) {
	dbErr := errors.New("connection reset")
	validCreate := dto.CreateProductInput{SKU: "SKU007", Name: "Keyboard", Price: 10, Quantity: 1}
	validUpdate := dto.UpdateProductInput{ID: 7, SKU: "SKU007", Name: "Keyboard", Price: 10}

	check := func(name string, got dto.ProductResponse, err, wantErr error) {
		t.Helper()
		if !errors.Is(err, wantErr) {
			t.Errorf("%s: error = %v, want %v", name, err, wantErr)
		}
		if got != (dto.ProductResponse{}) {
			t.Errorf("%s: result = %+v, want the zero DTO", name, got)
		}
	}

	got, err := NewProductUseCase(&fakeProductRepository{createErr: repository.ErrProductSKUExists}).Create(context.Background(), validCreate, testActorID)
	check("Create duplicate SKU", got, err, ErrProductSKUExists)
	got, err = NewProductUseCase(&fakeProductRepository{createErr: dbErr}).Create(context.Background(), validCreate, testActorID)
	check("Create repository error", got, err, dbErr)
	got, err = NewProductUseCase(&fakeProductRepository{}).Create(context.Background(), dto.CreateProductInput{SKU: "", Name: "x"}, testActorID)
	check("Create invalid product", got, err, ErrInvalidProduct)
	got, err = NewProductUseCase(&fakeProductRepository{}).Create(context.Background(), validCreate, 0)
	check("Create without actor", got, err, ErrInvalidActor)

	got, err = NewProductUseCase(&fakeProductRepository{updateErr: repository.ErrProductNotFound}).Update(context.Background(), validUpdate)
	check("Update missing product", got, err, ErrProductNotFound)
	got, err = NewProductUseCase(&fakeProductRepository{updateErr: repository.ErrProductSKUExists}).Update(context.Background(), validUpdate)
	check("Update duplicate SKU", got, err, ErrProductSKUExists)
	got, err = NewProductUseCase(&fakeProductRepository{updateErr: dbErr}).Update(context.Background(), validUpdate)
	check("Update repository error", got, err, dbErr)
	got, err = NewProductUseCase(&fakeProductRepository{}).Update(context.Background(), dto.UpdateProductInput{ID: 0, SKU: "a", Name: "b"})
	check("Update invalid ID", got, err, ErrInvalidID)
}

func TestProductGetByID_DTOResults(t *testing.T) {
	t.Run("all fields, including zero values", func(t *testing.T) {
		stored := &domain.Product{ID: 3, SKU: "SKU003", Name: "Cable", Price: 0, Quantity: 0, MinimumStock: 0}
		got, err := NewProductUseCase(&fakeProductRepository{product: stored}).GetByID(context.Background(), 3)
		if err != nil {
			t.Fatal(err)
		}
		want := dto.ProductResponse{ID: 3, SKU: "SKU003", Name: "Cable"}
		if got == nil || *got != want {
			t.Fatalf("GetByID = %+v, want %+v", got, want)
		}
		// The DTO is a copy: later changes to the domain value do not leak in.
		stored.Name = "changed"
		if got.Name != "Cable" {
			t.Errorf("DTO changed with the domain value: %q", got.Name)
		}
	})

	t.Run("nil product stays nil", func(t *testing.T) {
		got, err := NewProductUseCase(&fakeProductRepository{}).GetByID(context.Background(), 3)
		if err != nil || got != nil {
			t.Fatalf("GetByID = %+v, %v; want nil, nil", got, err)
		}
	})

	for name, tc := range map[string][2]error{
		"not found":        {repository.ErrProductNotFound, ErrProductNotFound},
		"repository error": {errors.New("connection reset"), nil},
	} {
		t.Run(name, func(t *testing.T) {
			wantErr := tc[1]
			if wantErr == nil {
				wantErr = tc[0]
			}
			got, err := NewProductUseCase(&fakeProductRepository{product: &domain.Product{ID: 3}, err: tc[0]}).GetByID(context.Background(), 3)
			if !errors.Is(err, wantErr) || got != nil {
				t.Fatalf("GetByID = %+v, %v; want nil, %v", got, err, wantErr)
			}
		})
	}
}

// List keeps the Product list rule (nil -> nil/JSON null, empty -> []/JSON
// []), the repository order and the repository total, including for a page
// past the end.
func TestProductList_DTOResults(t *testing.T) {
	list := func(repo *fakeProductRepository) ([]dto.ProductResponse, int64, error) {
		return NewProductUseCase(repo).List(context.Background(), "", 3, 10, "", "")
	}

	t.Run("nil page stays nil", func(t *testing.T) {
		got, total, err := list(&fakeProductRepository{listProducts: nil, listTotal: 0})
		if err != nil || got != nil || total != 0 {
			t.Fatalf("List = %#v, %d, %v; want nil, 0, nil", got, total, err)
		}
	})

	t.Run("empty page past the end keeps the total", func(t *testing.T) {
		got, total, err := list(&fakeProductRepository{listProducts: []domain.Product{}, listTotal: 25})
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || len(got) != 0 || total != 25 {
			t.Fatalf("List = %#v, %d; want a non-nil empty slice and total 25", got, total)
		}
	})

	t.Run("order, fields and total", func(t *testing.T) {
		got, total, err := list(&fakeProductRepository{listProducts: []domain.Product{
			{ID: 2, SKU: "SKU002", Name: "Monitor", Price: 199.99, Quantity: 12, MinimumStock: 4},
			{ID: 1, SKU: "SKU001", Name: "Mouse", Price: 25, Quantity: 0},
		}, listTotal: 40})
		if err != nil {
			t.Fatal(err)
		}
		want := []dto.ProductResponse{
			{ID: 2, SKU: "SKU002", Name: "Monitor", Price: 199.99, Quantity: 12, MinimumStock: 4},
			{ID: 1, SKU: "SKU001", Name: "Mouse", Price: 25, Quantity: 0, MinimumStock: 0},
		}
		if !reflect.DeepEqual(got, want) || total != 40 {
			t.Fatalf("List = %+v, %d; want %+v, 40", got, total, want)
		}
	})

	t.Run("repository error", func(t *testing.T) {
		dbErr := errors.New("connection reset")
		got, total, err := list(&fakeProductRepository{listProducts: []domain.Product{{ID: 1}}, listTotal: 1, listErr: dbErr})
		if !errors.Is(err, dbErr) || got != nil || total != 0 {
			t.Fatalf("List = %#v, %d, %v; want nil, 0, the repository error", got, total, err)
		}
	})

	t.Run("invalid parameters", func(t *testing.T) {
		got, total, err := NewProductUseCase(&fakeProductRepository{listProducts: []domain.Product{{ID: 1}}, listTotal: 1}).
			List(context.Background(), "", 1, 10, "sku", "")
		if !errors.Is(err, ErrInvalidProduct) || got != nil || total != 0 {
			t.Fatalf("List = %#v, %d, %v; want nil, 0, ErrInvalidProduct", got, total, err)
		}
	})
}

func TestProductStockChanges_DTOResults(t *testing.T) {
	operations := map[string]func(uc *ProductUseCase) (*dto.ProductResponse, error){
		"StockIn": func(uc *ProductUseCase) (*dto.ProductResponse, error) {
			return uc.StockIn(context.Background(), 7, 3, testActorID)
		},
		"StockOut": func(uc *ProductUseCase) (*dto.ProductResponse, error) {
			return uc.StockOut(context.Background(), 7, 3, testActorID)
		},
	}

	for name, run := range operations {
		t.Run(name+" returns the stored product", func(t *testing.T) {
			stored := &domain.Product{ID: 7, SKU: "SKU007", Name: "Keyboard", Price: 99.5, Quantity: 8, MinimumStock: 2}
			got, err := run(NewProductUseCase(&fakeProductRepository{stockProduct: stored}))
			if err != nil {
				t.Fatal(err)
			}
			want := dto.ProductResponse{ID: 7, SKU: "SKU007", Name: "Keyboard", Price: 99.5, Quantity: 8, MinimumStock: 2}
			if got == nil || *got != want {
				t.Fatalf("%s = %+v, want %+v", name, got, want)
			}
		})

		t.Run(name+" nil product stays nil", func(t *testing.T) {
			got, err := run(NewProductUseCase(&fakeProductRepository{}))
			if err != nil || got != nil {
				t.Fatalf("%s = %+v, %v; want nil, nil", name, got, err)
			}
		})

		t.Run(name+" missing product", func(t *testing.T) {
			got, err := run(NewProductUseCase(&fakeProductRepository{stockProduct: &domain.Product{ID: 7}, stockErr: repository.ErrProductNotFound}))
			if !errors.Is(err, ErrProductNotFound) || got != nil {
				t.Fatalf("%s = %+v, %v; want nil, ErrProductNotFound", name, got, err)
			}
		})
	}

	got, err := NewProductUseCase(&fakeProductRepository{stockProduct: &domain.Product{ID: 7}, stockErr: repository.ErrInsufficientStock}).
		StockOut(context.Background(), 7, 3, testActorID)
	if !errors.Is(err, ErrInsufficientStock) || got != nil {
		t.Fatalf("StockOut = %+v, %v; want nil, ErrInsufficientStock", got, err)
	}
}
