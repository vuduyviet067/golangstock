package dto

type CreateProductRequest struct {
	ID       int64   `json:"id"`
	SKU      string  `json:"sku"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

type ProductResponse struct {
	ID       int64   `json:"id"`
	SKU      string  `json:"sku"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

type StockQuantityRequest struct {
	Quantity int `json:"quantity"`
}
