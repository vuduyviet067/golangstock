package domain

import (
	"context"
	"errors"
)

var (
	ErrProductNotFound   = errors.New("product not found")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrProductSKUExists  = errors.New("product SKU already exists")
)

type ProductRepository interface {
	GetByID(ctx context.Context, id int64) (*Product, error)
	Create(ctx context.Context, product *Product) error
	Update(ctx context.Context, product *Product) error
	Delete(ctx context.Context, id int64) error

	List(
		ctx context.Context,
		query string,
		page, limit int,
		sortBy, order string,
	) ([]Product, int64, error)

	StockIn(ctx context.Context, id int64, quantity int) (*Product, error)
	StockOut(ctx context.Context, id int64, quantity int) (*Product, error)
}
