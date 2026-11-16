package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/repository"
)

// Characterization of how ProductUseCase.Create and Update treat their input
// (Step 5.2): SKU and name are trimmed before the empty and length checks, and
// the repository receives the trimmed values.
//
// Create takes a dto.CreateProductInput value since Step 5.4A and Update a
// dto.UpdateProductInput value since Step 5.4B; both must leave the caller's
// input untouched (TestCreate_DoesNotMutateCallerInput and
// TestUpdate_DoesNotMutateCallerInput, which replaced
// TestCreate_MutatesCallerProduct and TestUpdate_MutatesCallerProduct on
// purpose).
//
// Already covered elsewhere and not repeated here: invalid input never
// reaching the repository (TestCreateInvalidProduct, TestUpdateInvalidProduct,
// TestUpdateInvalidMinimumStock), input errors before the actor check
// (TestStockChangesValidateInputBeforeActor), minimum stock semantics
// (TestCreateMinimumStock, TestUpdateMinimumStock) and repository values in
// the response DTO (product_dto_test.go).

// updateRecordingRepository records a copy of each product passed to Update,
// as it was when the repository received it.
type updateRecordingRepository struct {
	fakeProductRepository
	updated []domain.Product
}

func (r *updateRecordingRepository) Update(ctx context.Context, p *domain.Product, minimumStock *int) error {
	r.updated = append(r.updated, *p)
	return r.fakeProductRepository.Update(ctx, p, minimumStock)
}

// Lengths are measured after trimming, in bytes (len), against 50 for the SKU
// and 200 for the name. Byte counting is the current rule (see the Step 5.1
// audit: the columns limit characters); change these cases deliberately if
// that rule changes.
func TestCreate_TrimsBeforeLengthChecks(t *testing.T) {
	pad := func(s string) string { return "  " + s + "\t " }
	sku50, sku51 := strings.Repeat("S", 50), strings.Repeat("S", 51)
	name200, name201 := strings.Repeat("N", 200), strings.Repeat("N", 201)
	name100Accented := strings.Repeat("à", 100) // 200 bytes
	name101Accented := strings.Repeat("à", 101) // 202 bytes, 101 characters

	tests := []struct {
		name     string
		sku      string
		pName    string
		wantErr  error
		wantSKU  string
		wantName string
	}{
		{"padded 50-byte SKU is accepted", pad(sku50), "Keyboard", nil, sku50, "Keyboard"},
		{"51-byte SKU is rejected", sku51, "Keyboard", ErrInvalidProduct, "", ""},
		{"padded 200-byte name is accepted", "SKU001", pad(name200), nil, "SKU001", name200},
		{"201-byte name is rejected", "SKU001", name201, ErrInvalidProduct, "", ""},
		{"100 two-byte characters are accepted", "SKU001", name100Accented, nil, "SKU001", name100Accented},
		{"101 two-byte characters are rejected by byte length", "SKU001", name101Accented, ErrInvalidProduct, "", ""},
		{"whitespace-only name is rejected", "SKU001", " \t ", ErrInvalidProduct, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProductRepository{createID: 5}
			_, err := NewProductUseCase(repo).Create(context.Background(),
				dto.CreateProductInput{SKU: tt.sku, Name: tt.pName, Price: 10, Quantity: 1}, testActorID)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if repo.createCalled {
					t.Fatal("repository Create must not be called for an invalid product")
				}
				return
			}
			if repo.createdProduct.SKU != tt.wantSKU || repo.createdProduct.Name != tt.wantName {
				t.Errorf("repository received SKU %q name %q, want the trimmed %q / %q",
					repo.createdProduct.SKU, repo.createdProduct.Name, tt.wantSKU, tt.wantName)
			}
		})
	}
}

func TestUpdate_TrimsBeforeLengthChecksAndRepository(t *testing.T) {
	sku50 := strings.Repeat("S", 50)
	name200 := strings.Repeat("N", 200)

	t.Run("repository receives trimmed values", func(t *testing.T) {
		repo := &updateRecordingRepository{fakeProductRepository: fakeProductRepository{storedQuantity: 3}}
		_, err := NewProductUseCase(repo).Update(context.Background(),
			dto.UpdateProductInput{ID: 7, SKU: "  " + sku50 + " ", Name: "\t" + name200 + "  ", Price: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(repo.updated) != 1 {
			t.Fatalf("repository Update called %d times, want 1", len(repo.updated))
		}
		got := repo.updated[0]
		if got.ID != 7 || got.SKU != sku50 || got.Name != name200 || got.Price != 10 {
			t.Errorf("repository received %+v, want ID 7 with the trimmed SKU and name", got)
		}
	})

	for name, in := range map[string]dto.UpdateProductInput{
		"51-byte SKU":         {ID: 7, SKU: sku50 + "S", Name: "Keyboard", Price: 10},
		"201-byte name":       {ID: 7, SKU: "SKU007", Name: name200 + "N", Price: 10},
		"whitespace-only SKU": {ID: 7, SKU: "   ", Name: "Keyboard", Price: 10},
	} {
		t.Run(name+" is rejected", func(t *testing.T) {
			repo := &updateRecordingRepository{}
			if _, err := NewProductUseCase(repo).Update(context.Background(), in); !errors.Is(err, ErrInvalidProduct) {
				t.Fatalf("error = %v, want ErrInvalidProduct", err)
			}
			if len(repo.updated) != 0 {
				t.Fatal("repository Update must not be called for an invalid product")
			}
		})
	}
}

// TestCreate_DoesNotMutateCallerInput replaces TestCreate_MutatesCallerProduct
// (Step 5.4A). Create used to trim the caller's *domain.Product in place and
// let the repository write the ID into it. It now takes a
// dto.CreateProductInput value: the caller's input, including what its
// MinimumStock points to, stays exactly as passed in, on success and on every
// error, while the repository still receives the normalized product (trimmed,
// minimum stock resolved, ID 0) and the response carries the repository's ID.
func TestCreate_DoesNotMutateCallerInput(t *testing.T) {
	assertUnchanged := func(t *testing.T, in, before dto.CreateProductInput, minimumBefore *int) {
		t.Helper()
		if in != before {
			t.Errorf("caller's input = %+v, want it unchanged %+v", in, before)
		}
		if minimumBefore != nil && *in.MinimumStock != *minimumBefore {
			t.Errorf("caller's minimum stock = %d, want it unchanged %d", *in.MinimumStock, *minimumBefore)
		}
	}

	t.Run("success", func(t *testing.T) {
		minimum := 2
		in := dto.CreateProductInput{SKU: " SKU010 ", Name: " Mouse ", Price: 25, Quantity: 3, MinimumStock: &minimum}
		before, minimumBefore := in, minimum
		repo := &fakeProductRepository{createID: 123}

		got, err := NewProductUseCase(repo).Create(context.Background(), in, testActorID)
		if err != nil {
			t.Fatal(err)
		}

		assertUnchanged(t, in, before, &minimumBefore)
		want := domain.Product{ID: 0, SKU: "SKU010", Name: "Mouse", Price: 25, Quantity: 3, MinimumStock: 2}
		if !repo.createCalled || repo.createdProduct != want {
			t.Errorf("repository received %+v, want %+v", repo.createdProduct, want)
		}
		if got.ID != 123 {
			t.Errorf("response ID = %d, want the repository's 123", got.ID)
		}
	})

	t.Run("success with omitted minimum stock", func(t *testing.T) {
		in := dto.CreateProductInput{SKU: "SKU011", Name: "Cable", Price: 1, Quantity: 0}
		repo := &fakeProductRepository{createID: 124}

		if _, err := NewProductUseCase(repo).Create(context.Background(), in, testActorID); err != nil {
			t.Fatal(err)
		}

		if in.MinimumStock != nil {
			t.Errorf("caller's minimum stock = %v, want it still nil", in.MinimumStock)
		}
		if repo.createdProduct.MinimumStock != 0 || repo.createdProduct.ID != 0 {
			t.Errorf("repository received %+v, want minimum stock 0 and ID 0", repo.createdProduct)
		}
	})

	t.Run("validation failure after trimming", func(t *testing.T) {
		in := dto.CreateProductInput{SKU: "   ", Name: " Mouse ", Price: 25}
		before := in
		repo := &fakeProductRepository{}

		if _, err := NewProductUseCase(repo).Create(context.Background(), in, testActorID); !errors.Is(err, ErrInvalidProduct) {
			t.Fatalf("error = %v, want ErrInvalidProduct", err)
		}

		assertUnchanged(t, in, before, nil)
		if repo.createCalled {
			t.Error("repository Create must not be called for an invalid product")
		}
	})

	t.Run("repository failure", func(t *testing.T) {
		minimum := 0
		in := dto.CreateProductInput{SKU: " SKU010 ", Name: "Mouse", Price: 25, MinimumStock: &minimum}
		before, minimumBefore := in, minimum
		repo := &fakeProductRepository{createErr: repository.ErrProductSKUExists, createID: 123}

		if _, err := NewProductUseCase(repo).Create(context.Background(), in, testActorID); !errors.Is(err, ErrProductSKUExists) {
			t.Fatalf("error = %v, want ErrProductSKUExists", err)
		}

		assertUnchanged(t, in, before, &minimumBefore)
		want := domain.Product{ID: 0, SKU: "SKU010", Name: "Mouse", Price: 25, MinimumStock: 0}
		if repo.createdProduct != want {
			t.Errorf("repository received %+v, want %+v", repo.createdProduct, want)
		}
	})
}

// TestUpdate_DoesNotMutateCallerInput replaces TestUpdate_MutatesCallerProduct
// (Step 5.4B). Update used to trim the caller's *domain.Product in place and
// let the repository write the stored quantity and minimum stock into it. It
// now takes a dto.UpdateProductInput value: the caller's input, including what
// its MinimumStock points to, stays exactly as passed in, on success and on
// every error, while the repository still receives the normalized product
// (trimmed, quantity 0) with the caller's minimum stock pointer passed
// separately, and the response carries the repository's values.
func TestUpdate_DoesNotMutateCallerInput(t *testing.T) {
	assertUnchanged := func(t *testing.T, in, before dto.UpdateProductInput, minimumBefore *int) {
		t.Helper()
		if in != before {
			t.Errorf("caller's input = %+v, want it unchanged %+v", in, before)
		}
		if minimumBefore != nil && *in.MinimumStock != *minimumBefore {
			t.Errorf("caller's minimum stock = %d, want it unchanged %d", *in.MinimumStock, *minimumBefore)
		}
	}

	t.Run("success", func(t *testing.T) {
		minimum := 1
		in := dto.UpdateProductInput{ID: 7, SKU: " SKU007 ", Name: " Keyboard ", Price: 120, MinimumStock: &minimum}
		before, minimumBefore := in, minimum
		repo := &updateRecordingRepository{fakeProductRepository: fakeProductRepository{storedQuantity: 5, storedMinimumStock: 4}}

		got, err := NewProductUseCase(repo).Update(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}

		assertUnchanged(t, in, before, &minimumBefore)
		want := domain.Product{ID: 7, SKU: "SKU007", Name: "Keyboard", Price: 120}
		if len(repo.updated) != 1 || repo.updated[0] != want {
			t.Errorf("repository received %+v, want %+v", repo.updated, want)
		}
		if repo.updateMinimumStock != in.MinimumStock {
			t.Errorf("repository received minimum stock %v, want the caller's pointer %v", repo.updateMinimumStock, in.MinimumStock)
		}
		wantResp := dto.ProductResponse{ID: 7, SKU: "SKU007", Name: "Keyboard", Price: 120, Quantity: 5, MinimumStock: 1}
		if got != wantResp {
			t.Errorf("response = %+v, want the repository's values %+v", got, wantResp)
		}
	})

	t.Run("success with omitted minimum stock", func(t *testing.T) {
		in := dto.UpdateProductInput{ID: 7, SKU: "SKU007", Name: "Keyboard", Price: 120}
		repo := &updateRecordingRepository{fakeProductRepository: fakeProductRepository{storedQuantity: 5, storedMinimumStock: 4}}

		got, err := NewProductUseCase(repo).Update(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}

		if in.MinimumStock != nil {
			t.Errorf("caller's minimum stock = %v, want it still nil", in.MinimumStock)
		}
		if repo.updateMinimumStock != nil {
			t.Errorf("repository received minimum stock %v, want nil (keep the stored value)", repo.updateMinimumStock)
		}
		if got.Quantity != 5 || got.MinimumStock != 4 {
			t.Errorf("response = %+v, want the stored quantity 5 and minimum stock 4", got)
		}
	})

	for name, tc := range map[string]struct {
		in      dto.UpdateProductInput
		wantErr error
	}{
		"invalid ID":                        {dto.UpdateProductInput{ID: 0, SKU: " SKU007 ", Name: " Keyboard ", Price: 120, MinimumStock: intPtr(2)}, ErrInvalidID},
		"validation failure after trimming": {dto.UpdateProductInput{ID: 7, SKU: "   ", Name: " Keyboard ", Price: 120, MinimumStock: intPtr(2)}, ErrInvalidProduct},
	} {
		t.Run(name, func(t *testing.T) {
			in := tc.in
			before, minimumBefore := in, *in.MinimumStock
			repo := &updateRecordingRepository{}

			if _, err := NewProductUseCase(repo).Update(context.Background(), in); !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}

			assertUnchanged(t, in, before, &minimumBefore)
			if len(repo.updated) != 0 {
				t.Error("repository Update must not be called for an invalid input")
			}
		})
	}

	t.Run("repository failure", func(t *testing.T) {
		minimum := 0
		in := dto.UpdateProductInput{ID: 7, SKU: " SKU007 ", Name: "Keyboard", Price: 120, MinimumStock: &minimum}
		before, minimumBefore := in, minimum
		repo := &updateRecordingRepository{fakeProductRepository: fakeProductRepository{updateErr: repository.ErrProductNotFound, storedQuantity: 5}}

		if _, err := NewProductUseCase(repo).Update(context.Background(), in); !errors.Is(err, ErrProductNotFound) {
			t.Fatalf("error = %v, want ErrProductNotFound", err)
		}

		assertUnchanged(t, in, before, &minimumBefore)
		want := domain.Product{ID: 7, SKU: "SKU007", Name: "Keyboard", Price: 120}
		if len(repo.updated) != 1 || repo.updated[0] != want {
			t.Errorf("repository received %+v, want %+v", repo.updated, want)
		}
	})
}
