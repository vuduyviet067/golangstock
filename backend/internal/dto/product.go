package dto

type CreateProductRequest struct {
	ID       int64   `json:"id"`
	SKU      string  `json:"sku"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
	// MinimumStock is optional; omitted or null means 0.
	MinimumStock *int `json:"minimum_stock"`
}

type ProductResponse struct {
	ID           int64   `json:"id"`
	SKU          string  `json:"sku"`
	Name         string  `json:"name"`
	Price        float64 `json:"price"`
	Quantity     int     `json:"quantity"`
	MinimumStock int     `json:"minimum_stock"`
}

type StockQuantityRequest struct {
	Quantity int `json:"quantity"`
}

// CreateProductInput is the use case input for creating a product. It is not
// an HTTP request body: it has no JSON tags and no client-supplied ID.
// MinimumStock is optional; the use case decides what nil means.
type CreateProductInput struct {
	SKU          string
	Name         string
	Price        float64
	Quantity     int
	MinimumStock *int
}

// UpdateProductInput is the use case input for editing a product's details.
// It has no Quantity: stock changes only through stock-in and stock-out.
// MinimumStock is optional: nil keeps the stored threshold, any other value
// (including 0) replaces it.
type UpdateProductInput struct {
	ID           int64
	SKU          string
	Name         string
	Price        float64
	MinimumStock *int
}
