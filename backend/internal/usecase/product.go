package usecase

import (
	"context"
	"errors"
	"math"
	"strings"

	"gostock/internal/dto"
	"gostock/internal/mapper"
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

// GetByID returns the product as a response DTO. A nil product from the
// repository stays nil (JSON null).
func (uc *ProductUseCase) GetByID(
	ctx context.Context,
	id int64,
) (*dto.ProductResponse, error) {
	if id <= 0 {
		return nil, ErrInvalidID
	}

	product, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fromRepository(err)
	}
	return mapper.ToProductResponsePtr(product), nil
}

// Create validates and stores a new product. actorID is the authenticated
// user creating it; it is recorded on the 'initial' stock movement.
//
// SKU and name are trimmed and an omitted minimum stock means 0; the caller's
// input is never modified. It returns the product as stored: the repository
// assigns the ID.
func (uc *ProductUseCase) Create(
	ctx context.Context,
	in dto.CreateProductInput,
	actorID int64,
) (dto.ProductResponse, error) {
	// in is a copy; only its SKU and Name are reassigned here, and the
	// MinimumStock pointer is only read.
	in.SKU = strings.TrimSpace(in.SKU)
	in.Name = strings.TrimSpace(in.Name)

	if in.SKU == "" || in.Name == "" {
		return dto.ProductResponse{}, ErrInvalidProduct
	}

	if len(in.SKU) > 50 || len(in.Name) > 200 {
		return dto.ProductResponse{}, ErrInvalidProduct
	}

	if in.Price < 0 || in.Quantity < 0 {
		return dto.ProductResponse{}, ErrInvalidProduct
	}

	minimumStock := 0
	if in.MinimumStock != nil {
		minimumStock = *in.MinimumStock
	}
	if !isValidMinimumStock(minimumStock) {
		return dto.ProductResponse{}, ErrInvalidProduct
	}

	if actorID <= 0 {
		return dto.ProductResponse{}, ErrInvalidActor
	}

	product := mapper.ProductFromCreateInput(in, minimumStock)
	if err := fromRepository(uc.repo.Create(ctx, &product, actorID)); err != nil {
		return dto.ProductResponse{}, err
	}
	return mapper.ToProductResponse(product), nil
}

// isValidMinimumStock reports whether v fits products.minimum_stock: an
// INTEGER column constrained by products_minimum_stock_check (>= 0).
func isValidMinimumStock(v int) bool {
	return v >= 0 && v <= math.MaxInt32
}

// Update edits the product's details. in.MinimumStock is optional: nil keeps
// the stored threshold, any other value (including 0) replaces it.
//
// SKU and name are trimmed; the caller's input is never modified. It returns
// the product as stored: quantity and minimum stock are the values the
// repository reports, never the caller's.
func (uc *ProductUseCase) Update(
	ctx context.Context,
	in dto.UpdateProductInput,
) (dto.ProductResponse, error) {
	if in.ID <= 0 {
		return dto.ProductResponse{}, ErrInvalidID
	}

	// in is a copy; only its SKU and Name are reassigned here, and the
	// MinimumStock pointer is only read.
	in.SKU = strings.TrimSpace(in.SKU)
	in.Name = strings.TrimSpace(in.Name)

	if in.SKU == "" || in.Name == "" {
		return dto.ProductResponse{}, ErrInvalidProduct
	}

	if len(in.SKU) > 50 || len(in.Name) > 200 {
		return dto.ProductResponse{}, ErrInvalidProduct
	}

	// Quantity is not an update field (the input has none): stock changes only
	// through StockIn and StockOut, and the repository does not write it.
	if in.Price < 0 {
		return dto.ProductResponse{}, ErrInvalidProduct
	}

	if in.MinimumStock != nil && !isValidMinimumStock(*in.MinimumStock) {
		return dto.ProductResponse{}, ErrInvalidProduct
	}

	// The repository fills in the stored quantity and minimum stock; it only
	// reads minimumStock.
	product, minimumStock := mapper.ProductFromUpdateInput(in)
	if err := fromRepository(uc.repo.Update(ctx, &product, minimumStock)); err != nil {
		return dto.ProductResponse{}, err
	}
	return mapper.ToProductResponse(product), nil
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

// List returns one page of products as response DTOs in the repository's
// order, and the repository's total. A nil page stays nil (JSON null) and an
// empty page stays empty (JSON []).
func (uc *ProductUseCase) List(
	ctx context.Context,
	query string,
	page, limit int,
	sortBy, order string,
) ([]dto.ProductResponse, int64, error) {
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
	return mapper.ToProductResponses(products), total, nil
}

// StockIn adds stock. actorID is the authenticated user; it is recorded on
// the 'in' stock movement.
func (uc *ProductUseCase) StockIn(
	ctx context.Context,
	id int64,
	quantity int,
	actorID int64,
) (*dto.ProductResponse, error) {
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
	return mapper.ToProductResponsePtr(product), nil
}

// StockOut removes stock. actorID is the authenticated user; it is recorded
// on the 'out' stock movement.
func (uc *ProductUseCase) StockOut(
	ctx context.Context,
	id int64,
	quantity int,
	actorID int64,
) (*dto.ProductResponse, error) {
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
	return mapper.ToProductResponsePtr(product), nil
}
