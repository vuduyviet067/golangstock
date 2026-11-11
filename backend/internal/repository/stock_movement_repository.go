package repository

import (
	"context"

	"gostock/internal/domain"
)

// Stock movement types, as stored in stock_movements.movement_type (see its
// CHECK constraint). ProductRepository writes 'initial' when a product is
// created with stock, 'in' on a stock-in and 'out' on a stock-out.
const (
	MovementTypeInitial = "initial"
	MovementTypeIn      = "in"
	MovementTypeOut     = "out"
)

// StockMovementRepository reads the immutable stock movement history. It has
// no write methods: movements are recorded only by ProductRepository, in the
// same transaction as the stock change.
type StockMovementRepository interface {
	// ListByProduct returns one page of the product's movements, newest first
	// (id DESC), and the total number of movements for the product. It does
	// not check that the product exists.
	ListByProduct(ctx context.Context, productID int64, page, limit int) ([]domain.StockMovement, int64, error)

	// List returns one page of all movements, including those of deleted
	// products, newest first (created_at DESC, id DESC), and the total number
	// of movements.
	List(ctx context.Context, page, limit int) ([]domain.StockMovement, int64, error)
}
