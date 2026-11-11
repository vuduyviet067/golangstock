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
