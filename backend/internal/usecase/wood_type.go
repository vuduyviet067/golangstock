package usecase

import (
	"context"
	"errors"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/mapper"
	"gostock/internal/repository"
)

var (
	ErrInvalidWoodTypeID = errors.New("invalid wood type ID")
	ErrInvalidWoodType   = errors.New("invalid wood type")
)

type WoodTypeUseCase struct {
	repo repository.WoodTypeRepository
}

func NewWoodTypeUseCase(repo repository.WoodTypeRepository) *WoodTypeUseCase {
	return &WoodTypeUseCase{repo: repo}
}

// List returns every wood type sorted by name, never nil (JSON []).
func (uc *WoodTypeUseCase) List(ctx context.Context) ([]dto.WoodTypeResponse, error) {
	woodTypes, err := uc.repo.List(ctx)
	if err != nil {
		return nil, fromRepository(err)
	}
	return mapper.ToWoodTypeResponses(woodTypes), nil
}

// Create stores a new wood type. The name is trimmed and must have 1 to 100
// characters; names need not be unique.
func (uc *WoodTypeUseCase) Create(ctx context.Context, in dto.CreateWoodTypeInput) (dto.WoodTypeResponse, error) {
	name, ok := normalizeCatalogName(in.Name)
	if !ok {
		return dto.WoodTypeResponse{}, ErrInvalidWoodType
	}

	woodType := domain.WoodType{Name: name}
	if err := uc.repo.Create(ctx, &woodType); err != nil {
		return dto.WoodTypeResponse{}, fromRepository(err)
	}
	return mapper.ToWoodTypeResponse(woodType), nil
}

// Update renames the wood type. The name is the only field, so an input
// without it is ErrEmptyUpdate.
func (uc *WoodTypeUseCase) Update(ctx context.Context, in dto.UpdateWoodTypeInput) (dto.WoodTypeResponse, error) {
	if in.ID <= 0 {
		return dto.WoodTypeResponse{}, ErrInvalidWoodTypeID
	}
	if in.Name == nil {
		return dto.WoodTypeResponse{}, ErrEmptyUpdate
	}
	name, ok := normalizeCatalogName(*in.Name)
	if !ok {
		return dto.WoodTypeResponse{}, ErrInvalidWoodType
	}

	woodType := domain.WoodType{ID: in.ID, Name: name}
	if err := uc.repo.Update(ctx, &woodType); err != nil {
		return dto.WoodTypeResponse{}, fromRepository(err)
	}
	return mapper.ToWoodTypeResponse(woodType), nil
}

// Delete removes the wood type for good. A wood type still referenced by
// another row is ErrInUse.
func (uc *WoodTypeUseCase) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrInvalidWoodTypeID
	}
	return fromRepository(uc.repo.Delete(ctx, id))
}
