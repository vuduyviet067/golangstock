package dto

import "time"

type WoodTypeResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateWoodTypeRequest struct {
	Name string `json:"name"`
}

// UpdateWoodTypeRequest is a PATCH body: an omitted or null field keeps the
// stored value.
type UpdateWoodTypeRequest struct {
	Name *string `json:"name"`
}

// CreateWoodTypeInput is the use case input for creating a wood type.
type CreateWoodTypeInput struct {
	Name string
}

// UpdateWoodTypeInput is the use case input for editing a wood type: a nil
// field keeps the stored value.
type UpdateWoodTypeInput struct {
	ID   int64
	Name *string
}
