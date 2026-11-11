package usecase

import (
	"context"
	"errors"
	"math"
	"strings"

	"gostock/internal/domain"
	"gostock/internal/repository"
)

var (
	ErrInvalidProduct  = errors.New("invalid product")
	ErrInvalidID       = errors.New("invalid product id")
	ErrInvalidQuantity = errors.New("quantity must be positive")
)

type ProductUseCase struct {
	repo repository.ProductRepository
}

func NewProductUseCase(repo repository.ProductRepository) *ProductUseCase {
	return &ProductUseCase{
		repo: repo,
	}
}

func (uc *ProductUseCase) GetByID(
	ctx context.Context,
	id int64,
) (*domain.Product, error) {
	if id <= 0 {
		return nil, ErrInvalidID
	}

	product, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fromRepository(err)
	}
	return product, nil
}

// Create validates and stores a new product. actorID is the authenticated
// user creating it; it is recorded on the 'initial' stock movement.
func (uc *ProductUseCase) Create(
	ctx context.Context,
	product *domain.Product,
	actorID int64,
) error {
	if product == nil {
		return ErrInvalidProduct
	}

	product.SKU = strings.TrimSpace(product.SKU)
	product.Name = strings.TrimSpace(product.Name)

	if product.SKU == "" || product.Name == "" {
		return ErrInvalidProduct
	}

	if len(product.SKU) > 50 || len(product.Name) > 200 {
		return ErrInvalidProduct
	}

	if product.Price < 0 || product.Quantity < 0 {
		return ErrInvalidProduct
	}

	if !isValidMinimumStock(product.MinimumStock) {
		return ErrInvalidProduct
	}

	if actorID <= 0 {
		return ErrInvalidActor
	}

	return fromRepository(uc.repo.Create(ctx, product, actorID))
}

// isValidMinimumStock reports whether v fits products.minimum_stock: an
// INTEGER column constrained by products_minimum_stock_check (>= 0).
func isValidMinimumStock(v int) bool {
	return v >= 0 && v <= math.MaxInt32
}

// Update edits the product's details. minimumStock is optional: nil keeps the
// stored threshold, any other value replaces it.
func (uc *ProductUseCase) Update(
	ctx context.Context,
	product *domain.Product,
	minimumStock *int,
) error {
	if product == nil {
		return ErrInvalidProduct
	}

	if product.ID <= 0 {
		return ErrInvalidID
	}

	product.SKU = strings.TrimSpace(product.SKU)
	product.Name = strings.TrimSpace(product.Name)

	if product.SKU == "" || product.Name == "" {
		return ErrInvalidProduct
	}

	if len(product.SKU) > 50 || len(product.Name) > 200 {
		return ErrInvalidProduct
	}

	// Quantity is not an update field: stock changes only through StockIn and
	// StockOut, and the repository does not write it on Update.
	if product.Price < 0 {
		return ErrInvalidProduct
	}

	if minimumStock != nil && !isValidMinimumStock(*minimumStock) {
		return ErrInvalidProduct
	}

	return fromRepository(uc.repo.Update(ctx, product, minimumStock))
}

func (uc *ProductUseCase) Delete(
	ctx context.Context,
	id int64,
) error {
	if id <= 0 {
		return ErrInvalidID
	}

	return fromRepository(uc.repo.Delete(ctx, id))
}

func (uc *ProductUseCase) List(
	ctx context.Context,
	query string,
	page, limit int,
	sortBy, order string,
) ([]domain.Product, int64, error) {
	if page < 1 || limit < 1 || limit > 100 {
		return nil, 0, ErrInvalidProduct
	}

	query = strings.TrimSpace(query)
	sortBy = strings.ToLower(sortBy)
	order = strings.ToLower(order)

	if sortBy == "" {
		sortBy = "id"
	}
	if order == "" {
		order = "asc"
	}

	if sortBy != "id" && sortBy != "name" && sortBy != "price" {
		return nil, 0, ErrInvalidProduct
	}

	if order != "asc" && order != "desc" {
		return nil, 0, ErrInvalidProduct
	}

	products, total, err := uc.repo.List(ctx, query, page, limit, sortBy, order)
	if err != nil {
		return nil, 0, fromRepository(err)
	}
	return products, total, nil
}

// StockIn adds stock. actorID is the authenticated user; it is recorded on
// the 'in' stock movement.
func (uc *ProductUseCase) StockIn(
	ctx context.Context,
	id int64,
	quantity int,
	actorID int64,
) (*domain.Product, error) {
	if id <= 0 {
		return nil, ErrInvalidID
	}
	if quantity <= 0 {
		return nil, ErrInvalidQuantity
	}
	if actorID <= 0 {
		return nil, ErrInvalidActor
	}

	product, err := uc.repo.StockIn(ctx, id, quantity, actorID)
	if err != nil {
		return nil, fromRepository(err)
	}
	return product, nil
}

// StockOut removes stock. actorID is the authenticated user; it is recorded
// on the 'out' stock movement.
func (uc *ProductUseCase) StockOut(
	ctx context.Context,
	id int64,
	quantity int,
	actorID int64,
) (*domain.Product, error) {
	if id <= 0 {
		return nil, ErrInvalidID
	}
	if quantity <= 0 {
		return nil, ErrInvalidQuantity
	}
	if actorID <= 0 {
		return nil, ErrInvalidActor
	}

	product, err := uc.repo.StockOut(ctx, id, quantity, actorID)
	if err != nil {
		return nil, fromRepository(err)
	}
	return product, nil
}
