package main

// khai báo
type HealthResponse struct {
	Status   string `json:"status"`
	App      string `json:"app"`
	Database string `json:"database"`
}

type CreateProductRequest struct {
	SKU      string  `json:"sku"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}
type Product struct {
	ID       int64   `json:"id"`
	SKU      string  `json:"sku"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}
type StockRequest struct {
	Quantity int `json:"quantity"`
}
