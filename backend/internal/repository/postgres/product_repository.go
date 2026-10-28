package postgres

import (
	"context"
	"database/sql"
	"errors"
	"gostock/internal/domain"
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
		return domain.ErrProductSKUExists
	}

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
		`SELECT id, sku, name, price, quantity
         FROM products
         WHERE id = $1`,
		id,
	).Scan(
		&product.ID,
		&product.SKU,
		&product.Name,
		&product.Price,
		&product.Quantity,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrProductNotFound
	}

	if err != nil {
		return nil, err
	}

	return product, nil
}

func (r *ProductRepository) Create(
	ctx context.Context,
	product *domain.Product,
) error {
	err := r.db.QueryRowContext(
		ctx,
		`INSERT INTO products (sku, name, price, quantity)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		product.SKU,
		product.Name,
		product.Price,
		product.Quantity,
	).Scan(&product.ID)

	return mapProductError(err)
}

func (r *ProductRepository) Update(
	ctx context.Context,
	product *domain.Product,
) error {
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE products
		 SET sku = $1,
		     name = $2,
		     price = $3,
		     quantity = $4,
		     updated_at = NOW()
		 WHERE id = $5`,
		product.SKU,
		product.Name,
		product.Price,
		product.Quantity,
		product.ID,
	)
	if err != nil {
		return mapProductError(err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrProductNotFound
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
		return domain.ErrProductNotFound
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
		SELECT id, sku, name, price, quantity
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

func (r *ProductRepository) StockIn(
	ctx context.Context,
	id int64,
	quantity int,
) (*domain.Product, error) {
	product := &domain.Product{}

	err := r.db.QueryRowContext(
		ctx,
		`UPDATE products
		 SET quantity = quantity + $1,
		     updated_at = NOW()
		 WHERE id = $2
		 RETURNING id, sku, name, price, quantity`,
		quantity,
		id,
	).Scan(
		&product.ID,
		&product.SKU,
		&product.Name,
		&product.Price,
		&product.Quantity,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}

	return product, nil
}

func (r *ProductRepository) StockOut(
	ctx context.Context,
	id int64,
	quantity int,
) (*domain.Product, error) {
	product := &domain.Product{}

	err := r.db.QueryRowContext(
		ctx,
		`UPDATE products
		 SET quantity = quantity - $1,
		     updated_at = NOW()
		 WHERE id = $2
		   AND quantity >= $1
		 RETURNING id, sku, name, price, quantity`,
		quantity,
		id,
	).Scan(
		&product.ID,
		&product.SKU,
		&product.Name,
		&product.Price,
		&product.Quantity,
	)

	if errors.Is(err, sql.ErrNoRows) {
		var exists bool

		checkErr := r.db.QueryRowContext(
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
			return nil, domain.ErrProductNotFound
		}

		return nil, domain.ErrInsufficientStock
	}

	if err != nil {
		return nil, err
	}

	return product, nil
}

var _ domain.ProductRepository = (*ProductRepository)(nil)
