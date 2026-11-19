package mapper_test

import (
	"reflect"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/mapper"
)

func int64Ptr(v int64) *int64 { return &v }

func TestToStockMovementResponse(t *testing.T) {
	movement := domain.StockMovement{
		ID: 11, ProductID: int64Ptr(7), ProductSKU: "SKU007", Type: "stock_in",
		QuantityChange: 5, QuantityBefore: 2, QuantityAfter: 7,
		UserID: int64Ptr(3), CreatedAt: createdAt.In(ict),
	}

	got := mapper.ToStockMovementResponse(movement)

	if got.ID != 11 || got.ProductSKU != "SKU007" || got.MovementType != "stock_in" ||
		got.QuantityChange != 5 || got.QuantityBefore != 2 || got.QuantityAfter != 7 {
		t.Errorf("ToStockMovementResponse = %+v", got)
	}
	if got.ProductID == nil || *got.ProductID != 7 || got.UserID == nil || *got.UserID != 3 {
		t.Errorf("ProductID/UserID = %v/%v, want 7/3", got.ProductID, got.UserID)
	}
	if !got.CreatedAt.Equal(createdAt) || got.CreatedAt.Location() != ict {
		t.Errorf("CreatedAt = %v, want %v in ICT", got.CreatedAt, createdAt.In(ict))
	}
	assertJSON(t, got, `{"id":11,"product_id":7,"product_sku":"SKU007","movement_type":"stock_in","quantity_change":5,"quantity_before":2,"quantity_after":7,"user_id":3,"created_at":"2026-03-04T12:06:07.891234567+07:00"}`)
}

// A deleted product or a missing actor stays null; a negative change (stock
// out) and the zero time are written as they are.
func TestToStockMovementResponse_NullsAndZero(t *testing.T) {
	got := mapper.ToStockMovementResponse(domain.StockMovement{
		ID: 12, ProductSKU: "SKU-DELETED", Type: "stock_out",
		QuantityChange: -4, QuantityBefore: 4, QuantityAfter: 0,
	})

	if got.ProductID != nil || got.UserID != nil {
		t.Errorf("ProductID/UserID = %v/%v, want nil/nil", got.ProductID, got.UserID)
	}
	assertJSON(t, got, `{"id":12,"product_id":null,"product_sku":"SKU-DELETED","movement_type":"stock_out","quantity_change":-4,"quantity_before":4,"quantity_after":0,"user_id":null,"created_at":"0001-01-01T00:00:00Z"}`)
}

// The DTO gets its own copies of ProductID and UserID: changing the domain
// value afterwards does not change the response, and movements that share
// one pointer get separate pointers.
func TestToStockMovementResponse_IndependentPointers(t *testing.T) {
	productID, userID := int64(7), int64(3)
	movement := domain.StockMovement{ID: 1, ProductID: &productID, UserID: &userID, ProductSKU: "SKU007", Type: "in"}

	got := mapper.ToStockMovementResponse(movement)

	if got.ProductID == movement.ProductID || got.UserID == movement.UserID {
		t.Fatal("DTO pointers alias the domain pointers")
	}
	productID, userID = 99, 98
	if *got.ProductID != 7 || *got.UserID != 3 {
		t.Errorf("ProductID/UserID = %d/%d after changing the source, want 7/3", *got.ProductID, *got.UserID)
	}

	*got.ProductID = 55
	if productID != 99 {
		t.Errorf("changing the DTO changed the domain value to %d", productID)
	}

	shared := int64(5)
	list := mapper.ToStockMovementResponses([]domain.StockMovement{
		{ID: 1, ProductID: &shared, UserID: &shared},
		{ID: 2, ProductID: &shared, UserID: &shared},
	})
	if list[0].ProductID == list[1].ProductID || list[0].ProductID == list[0].UserID {
		t.Error("DTOs built from one shared pointer must not share pointers")
	}
	assertJSON(t, list, `[`+
		`{"id":1,"product_id":5,"product_sku":"","movement_type":"","quantity_change":0,"quantity_before":0,"quantity_after":0,"user_id":5,"created_at":"0001-01-01T00:00:00Z"},`+
		`{"id":2,"product_id":5,"product_sku":"","movement_type":"","quantity_change":0,"quantity_before":0,"quantity_after":0,"user_id":5,"created_at":"0001-01-01T00:00:00Z"}`+
		`]`)
}

// Stock movement lists are never nil: nil and empty both become [].
func TestToStockMovementResponses(t *testing.T) {
	for name, in := range map[string][]domain.StockMovement{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			got := mapper.ToStockMovementResponses(in)
			if got == nil || len(got) != 0 {
				t.Fatalf("ToStockMovementResponses(%s) = %#v, want a non-nil empty slice", name, got)
			}
			assertJSON(t, got, `[]`)
		})
	}

	t.Run("order kept", func(t *testing.T) {
		movements := []domain.StockMovement{
			{ID: 3, ProductID: int64Ptr(1), ProductSKU: "SKU001", Type: "stock_out", QuantityChange: -1, QuantityBefore: 5, QuantityAfter: 4, UserID: int64Ptr(2), CreatedAt: updatedAt},
			{ID: 1, ProductSKU: "SKU009", Type: "stock_in", QuantityChange: 10, QuantityBefore: 0, QuantityAfter: 10, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		}
		before := append([]domain.StockMovement(nil), movements...)

		got := mapper.ToStockMovementResponses(movements)

		assertJSON(t, got, `[`+
			`{"id":3,"product_id":1,"product_sku":"SKU001","movement_type":"stock_out","quantity_change":-1,"quantity_before":5,"quantity_after":4,"user_id":2,"created_at":"2026-03-05T06:07:08.12Z"},`+
			`{"id":1,"product_id":null,"product_sku":"SKU009","movement_type":"stock_in","quantity_change":10,"quantity_before":0,"quantity_after":10,"user_id":null,"created_at":"2026-01-02T03:04:05Z"}`+
			`]`)
		if !reflect.DeepEqual(movements, before) {
			t.Errorf("input was modified")
		}
	})
}
