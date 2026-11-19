package mapper

import (
	"gostock/internal/domain"
	"gostock/internal/dto"
)

// ToStockMovementResponse maps a stock movement to its response body. A nil
// ProductID (deleted product) or UserID (missing actor) stays nil (JSON null).
// A non-nil ID is copied into a new pointer, so the DTO never shares memory
// with the domain value.
func ToStockMovementResponse(m domain.StockMovement) dto.StockMovementResponse {
	return dto.StockMovementResponse{
		ID:             m.ID,
		ProductID:      copyInt64Ptr(m.ProductID),
		ProductSKU:     m.ProductSKU,
		MovementType:   m.Type,
		QuantityChange: m.QuantityChange,
		QuantityBefore: m.QuantityBefore,
		QuantityAfter:  m.QuantityAfter,
		UserID:         copyInt64Ptr(m.UserID),
		CreatedAt:      m.CreatedAt,
	}
}

// copyInt64Ptr returns nil for nil, and otherwise a new pointer to the same
// value.
func copyInt64Ptr(p *int64) *int64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// ToStockMovementResponses maps a page of movements, keeping their order. It
// never returns nil, so an empty or nil page is serialized as [].
func ToStockMovementResponses(movements []domain.StockMovement) []dto.StockMovementResponse {
	items := make([]dto.StockMovementResponse, len(movements))
	for i, m := range movements {
		items[i] = ToStockMovementResponse(m)
	}
	return items
}
