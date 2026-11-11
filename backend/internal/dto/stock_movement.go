package dto

import "time"

type StockMovementResponse struct {
	ID int64 `json:"id"`

	ProductID *int64 `json:"product_id"`

	ProductSKU   string `json:"product_sku"`
	MovementType string `json:"movement_type"`

	QuantityChange int `json:"quantity_change"`
	QuantityBefore int `json:"quantity_before"`
	QuantityAfter  int `json:"quantity_after"`

	UserID    *int64    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}
