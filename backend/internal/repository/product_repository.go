package repository

import (
	"context"

	"gostock/internal/domain"
)

type ProductRepository interface {
	GetByID(ctx context.Context, id int64) (*domain.Product, error)
	// Create inserts the product and, when its quantity is positive, an
	// 'initial' stock movement by actorID, in one transaction.
	Create(ctx context.Context, product *domain.Product, actorID int64) error
	// Update writes sku, name and price, never quantity. minimumStock nil keeps
	// the stored threshold. product.Quantity and product.MinimumStock are set
	// to the stored values.
	Update(ctx context.Context, product *domain.Product, minimumStock *int) error
	Delete(ctx context.Context, id int64) error

	List(
		ctx context.Context,
		query string,
		page, limit int,
		sortBy, order string,
	) ([]domain.Product, int64, error)

	// StockIn and StockOut change the quantity and record the matching stock
	// movement by actorID in one transaction: both apply or neither does.
	StockIn(ctx context.Context, id int64, quantity int, actorID int64) (*domain.Product, error)
	StockOut(ctx context.Context, id int64, quantity int, actorID int64) (*domain.Product, error)
}
