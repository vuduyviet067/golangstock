package usecase

import (
	"context"
	"math"

	"gostock/internal/domain"
	"gostock/internal/repository"
)

// StockMovementUseCase reads the stock movement history. It never writes:
// movements are recorded by ProductUseCase together with each stock change.
type StockMovementUseCase struct {
	products  repository.ProductRepository
	movements repository.StockMovementRepository
}

func NewStockMovementUseCase(
	products repository.ProductRepository,
	movements repository.StockMovementRepository,
) *StockMovementUseCase {
	return &StockMovementUseCase{
		products:  products,
		movements: movements,
	}
}

// validatePagination applies the project's pagination rules (page >= 1,
// 1 <= limit <= 100). page is also capped so the row offset cannot overflow.
func validatePagination(page, limit int) error {
	if page < 1 || page > math.MaxInt32 {
		return ErrInvalidPage
	}
	if limit < 1 || limit > 100 {
		return ErrInvalidLimit
	}
	return nil
}

// ListByProduct returns one page of an existing product's movements, newest
// first, and their total. A missing product, including a deleted one, is
// ErrProductNotFound; its history stays available through List.
func (uc *StockMovementUseCase) ListByProduct(
	ctx context.Context,
	productID int64,
	page, limit int,
) ([]domain.StockMovement, int64, error) {
	if productID <= 0 {
		return nil, 0, ErrInvalidID
	}
	if err := validatePagination(page, limit); err != nil {
		return nil, 0, err
	}

	if _, err := uc.products.GetByID(ctx, productID); err != nil {
		return nil, 0, fromRepository(err)
	}

	movements, total, err := uc.movements.ListByProduct(ctx, productID, page, limit)
	if err != nil {
		return nil, 0, fromRepository(err)
	}
	return movements, total, nil
}

// List returns one page of all movements, including those of deleted
// products, newest first, and their total. Access is restricted to admins by
// the HTTP router.
func (uc *StockMovementUseCase) List(
	ctx context.Context,
	page, limit int,
) ([]domain.StockMovement, int64, error) {
	if err := validatePagination(page, limit); err != nil {
		return nil, 0, err
	}

	movements, total, err := uc.movements.List(ctx, page, limit)
	if err != nil {
		return nil, 0, fromRepository(err)
	}
	return movements, total, nil
}
