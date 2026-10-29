package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"gostock/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func mapUserError(err error) error {
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "users_email_unique_idx" {
		return domain.ErrEmailExists
	}

	return err
}

func (r *UserRepository) Create(
	ctx context.Context,
	user *domain.User,
) error {
	user.Email = strings.ToLower(strings.TrimSpace(user.Email))

	err := r.db.QueryRowContext(
		ctx,
		`INSERT INTO users (email, password_hash, role)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at, updated_at`,
		user.Email,
		user.PasswordHash,
		user.Role,
	).Scan(
		&user.ID,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	return mapUserError(err)
}

func (r *UserRepository) GetByEmail(
	ctx context.Context,
	email string,
) (*domain.User, error) {
	user := &domain.User{}
	email = strings.ToLower(strings.TrimSpace(email))

	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, email, password_hash, role, created_at, updated_at
		 FROM users
		 WHERE LOWER(email) = $1`,
		email,
	).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (r *UserRepository) GetByID(
	ctx context.Context,
	id int64,
) (*domain.User, error) {
	user := &domain.User{}

	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, email, password_hash, role, created_at, updated_at
		 FROM users
		 WHERE id = $1`,
		id,
	).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}

var _ domain.UserRepository = (*UserRepository)(nil)
