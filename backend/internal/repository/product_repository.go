package repository

import (
	"context"

	"gostock/internal/domain"
)

type ProductRepository interface {
	GetByID(ctx context.Context, id int64) (*domain.Product, error)
	Create(ctx context.Context, product *domain.Product) error
	Update(ctx context.Context, product *domain.Product) error
	Delete(ctx context.Context, id int64) error

	List(
		ctx context.Context,
		query string,
		page, limit int,
		sortBy, order string,
	) ([]domain.Product, int64, error)

	StockIn(ctx context.Context, id int64, quantity int) (*domain.Product, error)
	StockOut(ctx context.Context, id int64, quantity int) (*domain.Product, error)
}
