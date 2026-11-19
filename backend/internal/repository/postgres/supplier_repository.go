package postgres

import (
	"context"
	"database/sql"
	"errors"

	"gostock/internal/domain"
	"gostock/internal/repository"
)

type SupplierRepository struct {
	db *sql.DB
}

func NewSupplierRepository(db *sql.DB) *SupplierRepository {
	return &SupplierRepository{db: db}
}

// scanSupplier reads one row of (id, name, phone, created_at, updated_at). A
// NULL phone becomes a nil Phone.
func scanSupplier(row interface{ Scan(dest ...any) error }, s *domain.Supplier) error {
	var phone sql.NullString
	if err := row.Scan(&s.ID, &s.Name, &phone, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return err
	}
	s.Phone = nil
	if phone.Valid {
		s.Phone = &phone.String
	}
	return nil
}

func (r *SupplierRepository) List(ctx context.Context) ([]domain.Supplier, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, name, phone, created_at, updated_at
		 FROM suppliers
		 ORDER BY name, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	suppliers := make([]domain.Supplier, 0)
	for rows.Next() {
		var s domain.Supplier
		if err := scanSupplier(rows, &s); err != nil {
			return nil, err
		}
		suppliers = append(suppliers, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return suppliers, nil
}

func (r *SupplierRepository) GetByID(ctx context.Context, id int64) (*domain.Supplier, error) {
	s := &domain.Supplier{}
	err := scanSupplier(r.db.QueryRowContext(
		ctx,
		`SELECT id, name, phone, created_at, updated_at
		 FROM suppliers
		 WHERE id = $1`,
		id,
	), s)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrSupplierNotFound
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *SupplierRepository) Create(ctx context.Context, supplier *domain.Supplier) error {
	return r.db.QueryRowContext(
		ctx,
		`INSERT INTO suppliers (name, phone)
		 VALUES ($1, $2)
		 RETURNING id, created_at, updated_at`,
		supplier.Name,
		supplier.Phone,
	).Scan(&supplier.ID, &supplier.CreatedAt, &supplier.UpdatedAt)
}

func (r *SupplierRepository) Update(ctx context.Context, supplier *domain.Supplier) error {
	err := r.db.QueryRowContext(
		ctx,
		`UPDATE suppliers
		 SET name = $1,
		     phone = $2,
		     updated_at = NOW()
		 WHERE id = $3
		 RETURNING created_at, updated_at`,
		supplier.Name,
		supplier.Phone,
		supplier.ID,
	).Scan(&supplier.CreatedAt, &supplier.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrSupplierNotFound
	}
	return err
}

func (r *SupplierRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(
		ctx,
		`DELETE FROM suppliers
		 WHERE id = $1`,
		id,
	)
	if err != nil {
		return mapDeleteInUse(err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return repository.ErrSupplierNotFound
	}
	return nil
}

var _ repository.SupplierRepository = (*SupplierRepository)(nil)
