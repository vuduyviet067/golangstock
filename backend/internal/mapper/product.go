package mapper

import (
	"gostock/internal/domain"
	"gostock/internal/dto"
)

// ToProductResponse maps a product to its response body.
func ToProductResponse(p domain.Product) dto.ProductResponse {
	return dto.ProductResponse{
		ID:           p.ID,
		SKU:          p.SKU,
		Name:         p.Name,
		Price:        p.Price,
		Quantity:     p.Quantity,
		MinimumStock: p.MinimumStock,
	}
}

// ToProductResponsePtr keeps a nil product as nil (JSON null).
func ToProductResponsePtr(p *domain.Product) *dto.ProductResponse {
	if p == nil {
		return nil
	}
	resp := ToProductResponse(*p)
	return &resp
}

// ToProductResponses keeps a nil slice as nil (JSON null) and an empty slice
// as an empty slice (JSON []), and keeps the order.
func ToProductResponses(products []domain.Product) []dto.ProductResponse {
	if products == nil {
		return nil
	}
	items := make([]dto.ProductResponse, len(products))
	for i, p := range products {
		items[i] = ToProductResponse(p)
	}
	return items
}

// ProductFromCreateInput maps a create input to a new product. It converts
// only: no trimming, no validation, no defaults. The product has no ID (the
// repository assigns it). minimumStock is the threshold the use case has
// already resolved from in.MinimumStock, which this function does not read.
func ProductFromCreateInput(in dto.CreateProductInput, minimumStock int) domain.Product {
	return domain.Product{
		SKU:          in.SKU,
		Name:         in.Name,
		Price:        in.Price,
		Quantity:     in.Quantity,
		MinimumStock: minimumStock,
	}
}

// ProductFromUpdateInput maps an update input to the two arguments of the
// repository's Update: the product with the edited details, and the optional
// minimum stock. It converts only: no trimming, no validation.
//
// The product's Quantity and MinimumStock stay 0: quantity is never an update
// field, and the threshold travels as the separate pointer so that nil (keep
// the stored value) stays distinct from 0. That pointer is in.MinimumStock
// itself, returned unchanged.
func ProductFromUpdateInput(in dto.UpdateProductInput) (domain.Product, *int) {
	return domain.Product{
		ID:    in.ID,
		SKU:   in.SKU,
		Name:  in.Name,
		Price: in.Price,
	}, in.MinimumStock
}
