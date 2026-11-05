package domain

type Product struct {
	ID       int64   `json:"id"`
	SKU      string  `json:"sku"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}
