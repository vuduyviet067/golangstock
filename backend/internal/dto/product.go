package dto

// StockQuantityRequest represents the body of the POST
// /api/v1/products/{id}/stock-in and /stock-out endpoints.
type StockQuantityRequest struct {
	Quantity int `json:"quantity"`
}
