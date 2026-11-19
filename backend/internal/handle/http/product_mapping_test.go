package http

import (
	"testing"

	"gostock/internal/dto"
)

// TestCreateProductInputFromRequest replaces TestProductFromCreateRequest
// (Step 5.4A): the handler no longer builds a domain product. It adapts the
// request to the use case input, copying the fields as sent; the client ID is
// dropped (the input has no ID field), and trimming and the minimum stock
// default now belong to the use case, so they must not happen here. The
// end-to-end results (ID 0 and default 0 at the repository) are pinned by
// TestCreateRequest_RepositoryReceivesNormalizedProduct and
// TestCreateMinimumStock.
func TestCreateProductInputFromRequest(t *testing.T) {
	req := dto.CreateProductRequest{ID: 99, SKU: "  SKU001 ", Name: " Keyboard", Price: 100.5, Quantity: 5}

	got := createProductInput(req)

	want := dto.CreateProductInput{SKU: "  SKU001 ", Name: " Keyboard", Price: 100.5, Quantity: 5, MinimumStock: nil}
	if got != want {
		t.Fatalf("createProductInput = %+v, want %+v (fields as sent, no trimming, no default)", got, want)
	}

	minimum := 3
	req.MinimumStock = &minimum
	if got := createProductInput(req); got.MinimumStock != &minimum {
		t.Fatalf("createProductInput minimum stock = %v, want the request's pointer %v", got.MinimumStock, &minimum)
	}
}

// TestUpdateProductInputFromRequest (Step 5.4B): the handler no longer builds a
// domain product for Update either. The adapter takes the ID parsed from the
// path, never the body's id, and copies the other fields as sent: no
// trimming, and a nil minimum stock stays nil (keep the stored threshold),
// distinct from a pointer to 0. The request's quantity has no counterpart in
// the input; Update rejects it before the adapter runs
// (TestUpdateRejectsQuantity).
func TestUpdateProductInputFromRequest(t *testing.T) {
	req := dto.UpdateProductRequest{ID: 99, SKU: " SKU007 ", Name: "Keyboard ", Price: 120.5}

	got := updateProductInput(7, req)

	want := dto.UpdateProductInput{ID: 7, SKU: " SKU007 ", Name: "Keyboard ", Price: 120.5, MinimumStock: nil}
	if got != want {
		t.Fatalf("updateProductInput = %+v, want %+v (path ID, fields as sent, nil minimum stock kept)", got, want)
	}

	zero := 0
	req.MinimumStock = &zero
	if got := updateProductInput(7, req); got.MinimumStock != &zero {
		t.Fatalf("updateProductInput minimum stock = %v, want the request's pointer to 0 %v", got.MinimumStock, &zero)
	}
}

// The response mapping tests that used to live here (toProductResponse,
// toProductResponsePtr, toProductResponses) moved with the mapping itself to
// internal/mapper/product_test.go (TestToProductResponse,
// TestToProductResponsePtr, TestToProductResponses), which covers the same
// inputs and also checks the JSON.
