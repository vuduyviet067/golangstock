package dto

import "time"

type SupplierResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Phone is null when the supplier has no phone number.
	Phone     *string   `json:"phone"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateSupplierRequest: phone is optional; omitted, null or "" means none.
type CreateSupplierRequest struct {
	Name  string  `json:"name"`
	Phone *string `json:"phone"`
}

// UpdateSupplierRequest is a PATCH body: an omitted or null field keeps the
// stored value; phone "" removes the stored number.
type UpdateSupplierRequest struct {
	Name  *string `json:"name"`
	Phone *string `json:"phone"`
}

// CreateSupplierInput is the use case input for creating a supplier. Phone
// nil or blank means no phone number.
type CreateSupplierInput struct {
	Name  string
	Phone *string
}

// UpdateSupplierInput is the use case input for editing a supplier: a nil
// field keeps the stored value; a blank Phone removes the stored number.
type UpdateSupplierInput struct {
	ID    int64
	Name  *string
	Phone *string
}
