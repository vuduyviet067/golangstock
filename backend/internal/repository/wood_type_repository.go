package repository

import (
	"context"

	"gostock/internal/domain"
)

type WoodTypeRepository interface {
	// List returns every wood type sorted by name, then ID; never nil.
	List(ctx context.Context) ([]domain.WoodType, error)
	// GetByID returns ErrWoodTypeNotFound when no wood type has the ID.
	GetByID(ctx context.Context, id int64) (*domain.WoodType, error)
	// Create inserts the wood type and sets its ID and timestamps.
	Create(ctx context.Context, woodType *domain.WoodType) error
	// Update writes the name, sets updated_at to now and fills in both stored
	// timestamps. It returns ErrWoodTypeNotFound when no wood type has the ID.
	Update(ctx context.Context, woodType *domain.WoodType) error
	// Delete removes the wood type. It returns ErrWoodTypeNotFound when no wood
	// type has the ID and ErrInUse when another row still references it.
	Delete(ctx context.Context, id int64) error
}
