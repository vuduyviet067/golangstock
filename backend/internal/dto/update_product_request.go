package dto

import "encoding/json"

// UpdateProductRequest represents the PUT /api/v1/products/{id} body.
// Quantity is retained only to detect and reject attempts to update stock.
type UpdateProductRequest struct {
	// ID is accepted for backward compatibility and ignored; the path ID wins.
	ID int64 `json:"id"`

	SKU   string  `json:"sku"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`

	// RawMessage distinguishes an omitted field from an explicit null value.
	Quantity json.RawMessage `json:"quantity"`
}
