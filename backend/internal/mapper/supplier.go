package mapper

import (
	"gostock/internal/domain"
	"gostock/internal/dto"
)

// ToSupplierResponse maps a supplier to its response body. The phone is
// copied, so the response does not share the domain value's string pointer.
func ToSupplierResponse(s domain.Supplier) dto.SupplierResponse {
	var phone *string
	if s.Phone != nil {
		p := *s.Phone
		phone = &p
	}
	return dto.SupplierResponse{
		ID:        s.ID,
		Name:      s.Name,
		Phone:     phone,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

// ToSupplierResponses keeps the order and is never nil (JSON []).
func ToSupplierResponses(suppliers []domain.Supplier) []dto.SupplierResponse {
	items := make([]dto.SupplierResponse, len(suppliers))
	for i, s := range suppliers {
		items[i] = ToSupplierResponse(s)
	}
	return items
}
