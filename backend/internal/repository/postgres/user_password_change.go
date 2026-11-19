package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gostock/internal/repository"
)

// UpdateOwnPasswordAndRevokeTokens implements
// repository.PasswordChangeRepository: in one transaction it locks the user's
// row, sets the new password hash and revokes the user's refresh tokens that
// are not revoked yet. Unlike UpdatePasswordAndRevokeTokens it accepts any
// role.
func (r *UserRepository) UpdateOwnPasswordAndRevokeTokens(
	ctx context.Context,
	userID int64,
	passwordHash string,
	revokedAt time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Lock the row: a concurrent change of the same user waits for this one.
	var lockedID int64
	err = tx.QueryRowContext(
		ctx,
		`SELECT id
         FROM users
         WHERE id = $1
         FOR UPDATE`,
		userID,
	).Scan(&lockedID)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrUserNotFound
	}
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE users
         SET password_hash = $1,
             updated_at = NOW()
         WHERE id = $2`,
		passwordHash,
		userID,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE refresh_tokens
         SET revoked_at = $1
         WHERE user_id = $2
           AND revoked_at IS NULL`,
		revokedAt,
		userID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

var _ repository.PasswordChangeRepository = (*UserRepository)(nil)
