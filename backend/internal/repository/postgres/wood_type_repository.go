package postgres

import (
	"context"
	"database/sql"
	"errors"

	"gostock/internal/domain"
	"gostock/internal/repository"
)

type WoodTypeRepository struct {
	db *sql.DB
}

func NewWoodTypeRepository(db *sql.DB) *WoodTypeRepository {
	return &WoodTypeRepository{db: db}
}

func (r *WoodTypeRepository) List(ctx context.Context) ([]domain.WoodType, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, name, created_at, updated_at
		 FROM wood_types
		 ORDER BY name, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	woodTypes := make([]domain.WoodType, 0)
	for rows.Next() {
		var w domain.WoodType
		if err := rows.Scan(&w.ID, &w.Name, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		woodTypes = append(woodTypes, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return woodTypes, nil
}

func (r *WoodTypeRepository) GetByID(ctx context.Context, id int64) (*domain.WoodType, error) {
	w := &domain.WoodType{}
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, name, created_at, updated_at
		 FROM wood_types
		 WHERE id = $1`,
		id,
	).Scan(&w.ID, &w.Name, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrWoodTypeNotFound
	}
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (r *WoodTypeRepository) Create(ctx context.Context, woodType *domain.WoodType) error {
	return r.db.QueryRowContext(
		ctx,
		`INSERT INTO wood_types (name)
		 VALUES ($1)
		 RETURNING id, created_at, updated_at`,
		woodType.Name,
	).Scan(&woodType.ID, &woodType.CreatedAt, &woodType.UpdatedAt)
}

func (r *WoodTypeRepository) Update(ctx context.Context, woodType *domain.WoodType) error {
	err := r.db.QueryRowContext(
		ctx,
		`UPDATE wood_types
		 SET name = $1,
		     updated_at = NOW()
		 WHERE id = $2
		 RETURNING created_at, updated_at`,
		woodType.Name,
		woodType.ID,
	).Scan(&woodType.CreatedAt, &woodType.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrWoodTypeNotFound
	}
	return err
}

func (r *WoodTypeRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(
		ctx,
		`DELETE FROM wood_types
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
		return repository.ErrWoodTypeNotFound
	}
	return nil
}

var _ repository.WoodTypeRepository = (*WoodTypeRepository)(nil)
