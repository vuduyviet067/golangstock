package mapper_test

import (
	"reflect"
	"strings"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/mapper"
)

func TestToProductResponse(t *testing.T) {
	product := domain.Product{ID: 7, SKU: "SKU007", Name: "Keyboard", Price: 99.5, Quantity: 5, MinimumStock: 2}

	got := mapper.ToProductResponse(product)

	want := dto.ProductResponse{ID: 7, SKU: "SKU007", Name: "Keyboard", Price: 99.5, Quantity: 5, MinimumStock: 2}
	if got != want {
		t.Fatalf("ToProductResponse = %+v, want %+v", got, want)
	}
	assertJSON(t, got, `{"id":7,"sku":"SKU007","name":"Keyboard","price":99.5,"quantity":5,"minimum_stock":2}`)
}

func TestToProductResponse_Values(t *testing.T) {
	// Zero values are written, not omitted.
	assertJSON(t, mapper.ToProductResponse(domain.Product{}),
		`{"id":0,"sku":"","name":"","price":0,"quantity":0,"minimum_stock":0}`)
	// Whole prices have no decimal point; others keep their digits.
	assertJSON(t, mapper.ToProductResponse(domain.Product{ID: 1, SKU: "SKU001", Name: "Mouse", Price: 25}),
		`{"id":1,"sku":"SKU001","name":"Mouse","price":25,"quantity":0,"minimum_stock":0}`)
	assertJSON(t, mapper.ToProductResponse(domain.Product{ID: 2, SKU: "SKU002", Name: "Monitor", Price: 199.99, Quantity: 12, MinimumStock: 4}),
		`{"id":2,"sku":"SKU002","name":"Monitor","price":199.99,"quantity":12,"minimum_stock":4}`)
	// Names are copied verbatim (JSON escaping is the encoder's job).
	assertJSON(t, mapper.ToProductResponse(domain.Product{ID: 3, SKU: "SKU-3", Name: `Bàn phím "cơ"`, Price: 0.1}),
		`{"id":3,"sku":"SKU-3","name":"Bàn phím \"cơ\"","price":0.1,"quantity":0,"minimum_stock":0}`)
}

func TestToProductResponsePtr(t *testing.T) {
	if got := mapper.ToProductResponsePtr(nil); got != nil {
		t.Fatalf("ToProductResponsePtr(nil) = %+v, want nil", got)
	}
	assertJSON(t, mapper.ToProductResponsePtr(nil), `null`)

	product := &domain.Product{ID: 1, SKU: "SKU001", Name: "Mouse", Price: 25, Quantity: 3, MinimumStock: 1}
	got := mapper.ToProductResponsePtr(product)

	want := dto.ProductResponse{ID: 1, SKU: "SKU001", Name: "Mouse", Price: 25, Quantity: 3, MinimumStock: 1}
	if got == nil || *got != want {
		t.Fatalf("ToProductResponsePtr = %+v, want %+v", got, want)
	}
	if *product != (domain.Product{ID: 1, SKU: "SKU001", Name: "Mouse", Price: 25, Quantity: 3, MinimumStock: 1}) {
		t.Errorf("input was modified: %+v", *product)
	}
}

// Products keep the existing list rule: nil stays nil (JSON null), empty
// stays empty (JSON []).
func TestToProductResponses(t *testing.T) {
	if got := mapper.ToProductResponses(nil); got != nil {
		t.Fatalf("ToProductResponses(nil) = %#v, want nil", got)
	}
	assertJSON(t, mapper.ToProductResponses(nil), `null`)

	empty := mapper.ToProductResponses([]domain.Product{})
	if empty == nil || len(empty) != 0 {
		t.Fatalf("ToProductResponses(empty) = %#v, want a non-nil empty slice", empty)
	}
	assertJSON(t, empty, `[]`)

	products := []domain.Product{
		{ID: 2, SKU: "SKU002", Name: "Monitor", Price: 199.99, Quantity: 12, MinimumStock: 4},
		{ID: 1, SKU: "SKU001", Name: "Mouse", Price: 25, Quantity: 0},
	}
	got := mapper.ToProductResponses(products)

	want := []dto.ProductResponse{
		{ID: 2, SKU: "SKU002", Name: "Monitor", Price: 199.99, Quantity: 12, MinimumStock: 4},
		{ID: 1, SKU: "SKU001", Name: "Mouse", Price: 25, Quantity: 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToProductResponses = %+v, want %+v", got, want)
	}
	assertJSON(t, got, `[{"id":2,"sku":"SKU002","name":"Monitor","price":199.99,"quantity":12,"minimum_stock":4},{"id":1,"sku":"SKU001","name":"Mouse","price":25,"quantity":0,"minimum_stock":0}]`)
}

func intPtr(v int) *int { return &v }

// ProductFromCreateInput converts every field, uses the minimum stock the use
// case resolved (never in.MinimumStock), assigns no ID, and neither trims nor
// validates.
func TestProductFromCreateInput(t *testing.T) {
	t.Run("all fields, resolved minimum stock wins over the input pointer", func(t *testing.T) {
		in := dto.CreateProductInput{SKU: "SKU010", Name: "Mouse", Price: 25.5, Quantity: 3, MinimumStock: intPtr(9)}

		got := mapper.ProductFromCreateInput(in, 2)

		want := domain.Product{ID: 0, SKU: "SKU010", Name: "Mouse", Price: 25.5, Quantity: 3, MinimumStock: 2}
		if got != want {
			t.Fatalf("ProductFromCreateInput = %+v, want %+v", got, want)
		}
		if *in.MinimumStock != 9 {
			t.Errorf("input minimum stock changed to %d", *in.MinimumStock)
		}
	})

	t.Run("nil minimum stock is not defaulted by the mapper", func(t *testing.T) {
		in := dto.CreateProductInput{SKU: "SKU010", Name: "Mouse", Price: 25, Quantity: 3}
		for _, resolved := range []int{0, 7} {
			if got := mapper.ProductFromCreateInput(in, resolved); got.MinimumStock != resolved {
				t.Errorf("resolved %d: MinimumStock = %d, want %d", resolved, got.MinimumStock, resolved)
			}
		}
	})

	t.Run("whitespace is kept", func(t *testing.T) {
		got := mapper.ProductFromCreateInput(dto.CreateProductInput{SKU: "  SKU 1\t", Name: " Mouse  "}, 0)
		if got.SKU != "  SKU 1\t" || got.Name != " Mouse  " {
			t.Errorf("SKU %q name %q, want them untrimmed", got.SKU, got.Name)
		}
	})

	t.Run("invalid values are passed through unchanged", func(t *testing.T) {
		long := strings.Repeat("N", 300)
		in := dto.CreateProductInput{SKU: "", Name: long, Price: -1.5, Quantity: -3}

		got := mapper.ProductFromCreateInput(in, -4)

		want := domain.Product{SKU: "", Name: long, Price: -1.5, Quantity: -3, MinimumStock: -4}
		if got != want {
			t.Errorf("ProductFromCreateInput = %+v, want the values unchanged", got)
		}
	})
}

// ProductFromUpdateInput returns the repository's two Update arguments: the
// product (ID, SKU, name, price; Quantity and MinimumStock stay 0) and the
// optional minimum stock pointer, unchanged so that nil, 0 and a positive
// value keep their meaning.
func TestProductFromUpdateInput(t *testing.T) {
	base := dto.UpdateProductInput{ID: 7, SKU: " SKU007 ", Name: "Keyboard Pro", Price: 120.5}
	wantProduct := domain.Product{ID: 7, SKU: " SKU007 ", Name: "Keyboard Pro", Price: 120.5, Quantity: 0, MinimumStock: 0}

	cases := map[string]*int{"nil keeps the stored value": nil, "pointer to 0": intPtr(0), "pointer to 5": intPtr(5)}
	for name, minimum := range cases {
		t.Run(name, func(t *testing.T) {
			in := base
			in.MinimumStock = minimum

			gotProduct, gotMinimum := mapper.ProductFromUpdateInput(in)

			if gotProduct != wantProduct {
				t.Errorf("product = %+v, want %+v (no trimming, threshold not copied in)", gotProduct, wantProduct)
			}
			if gotMinimum != minimum {
				t.Errorf("minimum stock pointer = %v, want the input pointer %v", gotMinimum, minimum)
			}
			if minimum != nil && *gotMinimum != *minimum {
				t.Errorf("minimum stock = %d, want %d", *gotMinimum, *minimum)
			}
		})
	}

	t.Run("invalid values are passed through unchanged", func(t *testing.T) {
		gotProduct, gotMinimum := mapper.ProductFromUpdateInput(dto.UpdateProductInput{ID: 0, SKU: "", Name: "\t", Price: -0.01, MinimumStock: intPtr(-1)})

		if gotProduct != (domain.Product{ID: 0, SKU: "", Name: "\t", Price: -0.01}) {
			t.Errorf("product = %+v, want the values unchanged", gotProduct)
		}
		if gotMinimum == nil || *gotMinimum != -1 {
			t.Errorf("minimum stock = %v, want a pointer to -1", gotMinimum)
		}
	})
}

// The input DTOs are use case inputs, not HTTP bodies: no JSON tags, no
// client ID on create, and no Quantity on update.
func TestProductInputDTOShape(t *testing.T) {
	fields := func(v any) map[string]reflect.StructField {
		typ := reflect.TypeOf(v)
		m := map[string]reflect.StructField{}
		for i := 0; i < typ.NumField(); i++ {
			m[typ.Field(i).Name] = typ.Field(i)
		}
		return m
	}
	create, update := fields(dto.CreateProductInput{}), fields(dto.UpdateProductInput{})

	for _, f := range []map[string]reflect.StructField{create, update} {
		for name, field := range f {
			if field.Tag != "" {
				t.Errorf("input field %s has tag %q; input DTOs carry no JSON tags", name, field.Tag)
			}
		}
	}
	if _, ok := update["Quantity"]; ok {
		t.Error("UpdateProductInput must not have a Quantity field")
	}
	if _, ok := create["ID"]; ok {
		t.Error("CreateProductInput must not have an ID field")
	}
	intPointer := reflect.TypeOf((*int)(nil))
	if create["MinimumStock"].Type != intPointer || update["MinimumStock"].Type != intPointer {
		t.Error("MinimumStock must be *int on both inputs")
	}
	if create["Quantity"].Type != reflect.TypeOf(0) {
		t.Error("CreateProductInput must keep Quantity int")
	}
}
