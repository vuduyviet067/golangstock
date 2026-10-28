package usecase

import (
	"context"
	"errors"
	"strings"

	"gostock/internal/domain"
)

var (
	ErrInvalidProduct  = errors.New("invalid product")
	ErrInvalidID       = errors.New("invalid product id")
	ErrInvalidQuantity = errors.New("quantity must be positive")
)

type ProductUseCase struct {
	repo domain.ProductRepository
}

func NewProductUseCase(repo domain.ProductRepository) *ProductUseCase {
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

	return uc.repo.GetByID(ctx, id)
}

func (uc *ProductUseCase) Create(
	ctx context.Context,
	product *domain.Product,
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

	return uc.repo.Create(ctx, product)
}

func (uc *ProductUseCase) Update(
	ctx context.Context,
	product *domain.Product,
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

	if product.Price < 0 || product.Quantity < 0 {
		return ErrInvalidProduct
	}

	return uc.repo.Update(ctx, product)
}

func (uc *ProductUseCase) Delete(
	ctx context.Context,
	id int64,
) error {
	if id <= 0 {
		return ErrInvalidID
	}

	return uc.repo.Delete(ctx, id)
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

	return uc.repo.List(ctx, query, page, limit, sortBy, order)
}

func (uc *ProductUseCase) StockIn(
	ctx context.Context,
	id int64,
	quantity int,
) (*domain.Product, error) {
	if id <= 0 {
		return nil, ErrInvalidID
	}
	if quantity <= 0 {
		return nil, ErrInvalidQuantity
	}

	return uc.repo.StockIn(ctx, id, quantity)
}

func (uc *ProductUseCase) StockOut(
	ctx context.Context,
	id int64,
	quantity int,
) (*domain.Product, error) {
	if id <= 0 {
		return nil, ErrInvalidID
	}
	if quantity <= 0 {
		return nil, ErrInvalidQuantity
	}

	return uc.repo.StockOut(ctx, id, quantity)
}
