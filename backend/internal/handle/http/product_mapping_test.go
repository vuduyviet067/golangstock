package http

import (
	"reflect"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/dto"
)

func TestProductFromCreateRequest(t *testing.T) {
	req := dto.CreateProductRequest{ID: 99, SKU: "SKU001", Name: "Keyboard", Price: 100.5, Quantity: 5}

	got := productFromCreateRequest(req)

	want := domain.Product{SKU: "SKU001", Name: "Keyboard", Price: 100.5, Quantity: 5}
	if got != want {
		t.Fatalf("productFromCreateRequest = %+v, want %+v (client ID must be ignored)", got, want)
	}
}

func TestToProductResponse(t *testing.T) {
	product := domain.Product{ID: 7, SKU: "SKU007", Name: "Keyboard", Price: 99.5, Quantity: 5}

	got := toProductResponse(product)

	want := dto.ProductResponse{ID: 7, SKU: "SKU007", Name: "Keyboard", Price: 99.5, Quantity: 5}
	if got != want {
		t.Fatalf("toProductResponse = %+v, want %+v", got, want)
	}
}

func TestToProductResponsePtr(t *testing.T) {
	if got := toProductResponsePtr(nil); got != nil {
		t.Fatalf("toProductResponsePtr(nil) = %+v, want nil", got)
	}

	got := toProductResponsePtr(&domain.Product{ID: 1, SKU: "SKU001"})
	if got == nil || got.ID != 1 || got.SKU != "SKU001" {
		t.Fatalf("toProductResponsePtr = %+v, want ID 1 and SKU SKU001", got)
	}
}

func TestToProductResponses(t *testing.T) {
	if got := toProductResponses(nil); got != nil {
		t.Fatalf("toProductResponses(nil) = %#v, want nil", got)
	}

	empty := toProductResponses([]domain.Product{})
	if empty == nil || len(empty) != 0 {
		t.Fatalf("toProductResponses(empty) = %#v, want a non-nil empty slice", empty)
	}

	got := toProductResponses([]domain.Product{
		{ID: 1, SKU: "SKU001", Name: "Mouse", Price: 25, Quantity: 0},
		{ID: 2, SKU: "SKU002", Name: "Monitor", Price: 199.99, Quantity: 12},
	})
	want := []dto.ProductResponse{
		{ID: 1, SKU: "SKU001", Name: "Mouse", Price: 25, Quantity: 0},
		{ID: 2, SKU: "SKU002", Name: "Monitor", Price: 199.99, Quantity: 12},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("toProductResponses = %+v, want %+v", got, want)
	}
}
