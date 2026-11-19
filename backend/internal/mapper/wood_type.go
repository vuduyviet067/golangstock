package mapper

import (
	"gostock/internal/domain"
	"gostock/internal/dto"
)

// ToWoodTypeResponse maps a wood type to its response body.
func ToWoodTypeResponse(w domain.WoodType) dto.WoodTypeResponse {
	return dto.WoodTypeResponse{
		ID:        w.ID,
		Name:      w.Name,
		CreatedAt: w.CreatedAt,
		UpdatedAt: w.UpdatedAt,
	}
}

// ToWoodTypeResponses keeps the order and is never nil (JSON []).
func ToWoodTypeResponses(woodTypes []domain.WoodType) []dto.WoodTypeResponse {
	items := make([]dto.WoodTypeResponse, len(woodTypes))
	for i, w := range woodTypes {
		items[i] = ToWoodTypeResponse(w)
	}
	return items
}
