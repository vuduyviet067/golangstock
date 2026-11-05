package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"gostock/internal/domain"
	"gostock/internal/repository"

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

func (r *UserRepository) List(
	ctx context.Context,
	query string,
	page, limit int,
) ([]domain.User, int64, error) {
	query = strings.TrimSpace(query)

	var total int64
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*)
		 FROM users
		 WHERE ($1 = '' OR email ILIKE '%' || $1 || '%')`,
		query,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, email, password_hash, role, created_at, updated_at
		 FROM users
		 WHERE ($1 = '' OR email ILIKE '%' || $1 || '%')
		 ORDER BY id ASC
		 LIMIT $2 OFFSET $3`,
		query,
		limit,
		offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(
			&u.ID,
			&u.Email,
			&u.PasswordHash,
			&u.Role,
			&u.CreatedAt,
			&u.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (r *UserRepository) updateUserRow(ctx context.Context, qr queryRower, user *domain.User) error {
	user.Email = strings.ToLower(strings.TrimSpace(user.Email))

	err := qr.QueryRowContext(
		ctx,
		`UPDATE users
		 SET email = $1,
		     role = $2,
		     updated_at = NOW()
		 WHERE id = $3
		 RETURNING updated_at`,
		user.Email,
		user.Role,
		user.ID,
	).Scan(&user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrUserNotFound
	}
	if err != nil {
		return mapUserError(err)
	}

	return nil
}

func (r *UserRepository) Update(
	ctx context.Context,
	user *domain.User,
) error {
	return r.updateUserRow(ctx, r.db, user)
}

func (r *UserRepository) UpdateAndRevokeTokens(
	ctx context.Context,
	user *domain.User,
	revokedAt time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := r.updateUserRow(ctx, tx, user); err != nil {
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE refresh_tokens
		 SET revoked_at = COALESCE(revoked_at, $1)
		 WHERE user_id = $2`,
		revokedAt,
		user.ID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

var _ repository.UserRepository = (*UserRepository)(nil)
var _ repository.UserManagementRepository = (*UserRepository)(nil)
