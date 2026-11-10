package dto

import "encoding/json"

type UpdateProductRequest struct {
	ID int64 `json:"id"`

	SKU   string  `json:"sku"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`

	Quantity json.RawMessage `json:"quantity"`
}
