package postgres

import (
	"context"
	"database/sql"
	"errors"
	"gostock/internal/domain"
	"gostock/internal/repository"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type ProductRepository struct {
	db *sql.DB
}

func mapProductError(err error) error {
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "products_sku_key" {
		return repository.ErrProductSKUExists
	}

	// The actor recorded on a stock movement no longer exists.
	if errors.As(err, &pgErr) &&
		pgErr.Code == "23503" &&
		pgErr.ConstraintName == "stock_movements_user_id_fkey" {
		return repository.ErrInvalidActor
	}

	return err
}

// insertStockMovement records m. It must run in the transaction that changed
// the stock, so the quantity change and its history commit or roll back
// together.
func insertStockMovement(ctx context.Context, tx *sql.Tx, m domain.StockMovement) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO stock_movements
		     (product_id, product_sku, movement_type,
		      quantity_change, quantity_before, quantity_after, user_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		m.ProductID,
		m.ProductSKU,
		m.Type,
		m.QuantityChange,
		m.QuantityBefore,
		m.QuantityAfter,
		m.UserID,
	)
	return err
}

func NewProductRepository(db *sql.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

func (r *ProductRepository) GetByID(
	ctx context.Context,
	id int64,
) (*domain.Product, error) {
	product := &domain.Product{}

	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, sku, name, price, quantity, minimum_stock
         FROM products
         WHERE id = $1`,
		id,
	).Scan(
		&product.ID,
		&product.SKU,
		&product.Name,
		&product.Price,
		&product.Quantity,
		&product.MinimumStock,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrProductNotFound
	}

	if err != nil {
		return nil, err
	}

	return product, nil
}

// Create inserts the product and, when it starts with stock, its 'initial'
// movement in one transaction. product.ID is set only after the commit.
//
// The actor must exist even when no movement is written (quantity 0), so a
// user deleted while their access token is still valid cannot create
// products.
func (r *ProductRepository) Create(
	ctx context.Context,
	product *domain.Product,
	actorID int64,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// FOR KEY SHARE keeps the actor from being deleted until this transaction
	// ends: a concurrent DELETE waits for the commit, and a DELETE already in
	// progress is waited for here, after which a deleted row is not found.
	// It does not block updates to the user's email or role.
	var actorExists int
	err = tx.QueryRowContext(
		ctx,
		`SELECT 1 FROM users WHERE id = $1 FOR KEY SHARE`,
		actorID,
	).Scan(&actorExists)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrInvalidActor
	}
	if err != nil {
		return err
	}

	var id int64
	err = tx.QueryRowContext(
		ctx,
		`INSERT INTO products (sku, name, price, quantity, minimum_stock)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		product.SKU,
		product.Name,
		product.Price,
		product.Quantity,
		product.MinimumStock,
	).Scan(&id)
	if err != nil {
		return mapProductError(err)
	}

	if product.Quantity > 0 {
		err = insertStockMovement(ctx, tx, domain.StockMovement{
			ProductID:      &id,
			ProductSKU:     product.SKU,
			Type:           repository.MovementTypeInitial,
			QuantityChange: product.Quantity,
			QuantityBefore: 0,
			QuantityAfter:  product.Quantity,
			UserID:         &actorID,
		})
		if err != nil {
			return mapProductError(err)
		}
	}

	if err := tx.Commit(); err != nil {
		return mapProductError(err)
	}

	product.ID = id
	return nil
}

func (r *ProductRepository) Update(
	ctx context.Context,
	product *domain.Product,
	minimumStock *int,
) error {
	// quantity is intentionally not written: stock changes only through
	// StockIn/StockOut. A nil minimumStock binds NULL, so COALESCE keeps the
	// stored threshold. RETURNING reports the stored quantity and threshold.
	err := r.db.QueryRowContext(
		ctx,
		`UPDATE products
		 SET sku = $1,
		     name = $2,
		     price = $3,
		     minimum_stock = COALESCE($5, minimum_stock),
		     updated_at = NOW()
		 WHERE id = $4
		 RETURNING quantity, minimum_stock`,
		product.SKU,
		product.Name,
		product.Price,
		product.ID,
		minimumStock,
	).Scan(&product.Quantity, &product.MinimumStock)

	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrProductNotFound
	}
	if err != nil {
		return mapProductError(err)
	}

	return nil
}

func (r *ProductRepository) Delete(
	ctx context.Context,
	id int64,
) error {
	result, err := r.db.ExecContext(
		ctx,
		`DELETE FROM products
		 WHERE id = $1`,
		id,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return repository.ErrProductNotFound
	}

	return nil
}

func (r *ProductRepository) List(
	ctx context.Context,
	query string,
	page, limit int,
	sortBy, order string,
) ([]domain.Product, int64, error) {
	allowedSortFields := map[string]string{
		"id":    "id",
		"name":  "name",
		"price": "price",
	}

	column, ok := allowedSortFields[sortBy]
	if !ok {
		return nil, 0, errors.New("invalid sort field")
	}

	direction := strings.ToUpper(order)
	if direction != "ASC" && direction != "DESC" {
		return nil, 0, errors.New("invalid sort order")
	}

	var total int64

	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*)
		 FROM products
		 WHERE ($1 = '' OR name ILIKE '%' || $1 || '%'
		        OR sku ILIKE '%' || $1 || '%')`,
		query,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// column và direction chỉ được lấy từ allowlist ở trên.
	sqlQuery := `
		SELECT id, sku, name, price, quantity, minimum_stock
		FROM products
		WHERE ($1 = '' OR name ILIKE '%' || $1 || '%'
		       OR sku ILIKE '%' || $1 || '%')
		ORDER BY ` + column + ` ` + direction + `, id ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(
		ctx,
		sqlQuery,
		query,
		limit,
		(page-1)*limit,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	products := make([]domain.Product, 0)

	for rows.Next() {
		var product domain.Product

		if err := rows.Scan(
			&product.ID,
			&product.SKU,
			&product.Name,
			&product.Price,
			&product.Quantity,
			&product.MinimumStock,
		); err != nil {
			return nil, 0, err
		}

		products = append(products, product)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

// StockIn adds quantity and records an 'in' movement in one transaction.
//
// The UPDATE is atomic and locks the product row until the commit, so
// concurrent stock changes to the same product are serialized and the
// returned quantity is exactly the stock after this change.
func (r *ProductRepository) StockIn(
	ctx context.Context,
	id int64,
	quantity int,
	actorID int64,
) (*domain.Product, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	product := &domain.Product{}

	err = tx.QueryRowContext(
		ctx,
		`UPDATE products
		 SET quantity = quantity + $1,
		     updated_at = NOW()
		 WHERE id = $2
		 RETURNING id, sku, name, price, quantity, minimum_stock`,
		quantity,
		id,
	).Scan(
		&product.ID,
		&product.SKU,
		&product.Name,
		&product.Price,
		&product.Quantity,
		&product.MinimumStock,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}

	err = insertStockMovement(ctx, tx, domain.StockMovement{
		ProductID:      &product.ID,
		ProductSKU:     product.SKU,
		Type:           repository.MovementTypeIn,
		QuantityChange: quantity,
		QuantityBefore: product.Quantity - quantity,
		QuantityAfter:  product.Quantity,
		UserID:         &actorID,
	})
	if err != nil {
		return nil, mapProductError(err)
	}

	if err := tx.Commit(); err != nil {
		return nil, mapProductError(err)
	}

	return product, nil
}

// StockOut removes quantity and records an 'out' movement in one transaction.
//
// The conditional UPDATE never lets stock go negative: under concurrent
// requests PostgreSQL re-checks "quantity >= $1" against the latest committed
// row after waiting for its lock, and the lock is held until the commit.
func (r *ProductRepository) StockOut(
	ctx context.Context,
	id int64,
	quantity int,
	actorID int64,
) (*domain.Product, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	product := &domain.Product{}

	err = tx.QueryRowContext(
		ctx,
		`UPDATE products
		 SET quantity = quantity - $1,
		     updated_at = NOW()
		 WHERE id = $2
		   AND quantity >= $1
		 RETURNING id, sku, name, price, quantity, minimum_stock`,
		quantity,
		id,
	).Scan(
		&product.ID,
		&product.SKU,
		&product.Name,
		&product.Price,
		&product.Quantity,
		&product.MinimumStock,
	)

	if errors.Is(err, sql.ErrNoRows) {
		var exists bool

		checkErr := tx.QueryRowContext(
			ctx,
			`SELECT EXISTS(
				SELECT 1 FROM products WHERE id = $1
			)`,
			id,
		).Scan(&exists)

		if checkErr != nil {
			return nil, checkErr
		}
		if !exists {
			return nil, repository.ErrProductNotFound
		}

		return nil, repository.ErrInsufficientStock
	}

	if err != nil {
		return nil, err
	}

	err = insertStockMovement(ctx, tx, domain.StockMovement{
		ProductID:      &product.ID,
		ProductSKU:     product.SKU,
		Type:           repository.MovementTypeOut,
		QuantityChange: -quantity,
		QuantityBefore: product.Quantity + quantity,
		QuantityAfter:  product.Quantity,
		UserID:         &actorID,
	})
	if err != nil {
		return nil, mapProductError(err)
	}

	if err := tx.Commit(); err != nil {
		return nil, mapProductError(err)
	}

	return product, nil
}

var _ repository.ProductRepository = (*ProductRepository)(nil)
