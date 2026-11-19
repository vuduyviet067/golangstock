package postgres

import (
	"context"
	"database/sql"

	"gostock/internal/domain"
	"gostock/internal/repository"
)

type StockMovementRepository struct {
	db *sql.DB
}

func NewStockMovementRepository(db *sql.DB) *StockMovementRepository {
	return &StockMovementRepository{db: db}
}

const stockMovementColumns = `id, product_id, product_sku, movement_type,
	quantity_change, quantity_before, quantity_after, user_id, created_at`

// ListByProduct uses stock_movements_product_id_idx (product_id, id DESC).
func (r *StockMovementRepository) ListByProduct(
	ctx context.Context,
	productID int64,
	page, limit int,
) ([]domain.StockMovement, int64, error) {
	var total int64
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM stock_movements WHERE product_id = $1`,
		productID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	movements, err := r.query(
		ctx,
		`SELECT `+stockMovementColumns+`
		 FROM stock_movements
		 WHERE product_id = $1
		 ORDER BY id DESC
		 LIMIT $2 OFFSET $3`,
		productID,
		limit,
		pageOffset(page, limit),
	)
	if err != nil {
		return nil, 0, err
	}

	return movements, total, nil
}

// List returns every movement; rows of deleted products (product_id NULL)
// are included.
func (r *StockMovementRepository) List(
	ctx context.Context,
	page, limit int,
) ([]domain.StockMovement, int64, error) {
	var total int64
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM stock_movements`,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	movements, err := r.query(
		ctx,
		`SELECT `+stockMovementColumns+`
		 FROM stock_movements
		 ORDER BY created_at DESC, id DESC
		 LIMIT $1 OFFSET $2`,
		limit,
		pageOffset(page, limit),
	)
	if err != nil {
		return nil, 0, err
	}

	return movements, total, nil
}

// pageOffset converts a 1-based page into a row offset. The use case bounds page
// and limit, so the product cannot overflow.
func pageOffset(page, limit int) int64 {
	return int64(page-1) * int64(limit)
}

func (r *StockMovementRepository) query(
	ctx context.Context,
	query string,
	args ...any,
) ([]domain.StockMovement, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	movements := make([]domain.StockMovement, 0)
	for rows.Next() {
		var m domain.StockMovement
		if err := rows.Scan(
			&m.ID,
			&m.ProductID,
			&m.ProductSKU,
			&m.Type,
			&m.QuantityChange,
			&m.QuantityBefore,
			&m.QuantityAfter,
			&m.UserID,
			&m.CreatedAt,
		); err != nil {
			return nil, err
		}
		movements = append(movements, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return movements, nil
}

var _ repository.StockMovementRepository = (*StockMovementRepository)(nil)
