package domain

import "time"

type StockMovement struct {
	ID int64

	ProductID *int64

	ProductSKU     string
	Type           string
	QuantityChange int
	QuantityBefore int
	QuantityAfter  int

	UserID    *int64
	CreatedAt time.Time
}
