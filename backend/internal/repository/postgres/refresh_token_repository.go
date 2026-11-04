package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gostock/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

type RefreshTokenRepository struct {
	db *sql.DB
}

func NewRefreshTokenRepository(db *sql.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func mapRefreshTokenError(err error) error {
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23503" && pgErr.ConstraintName == "refresh_tokens_user_id_fkey" {
			return domain.ErrUserNotFound
		}
	}

	return err
}

func (r *RefreshTokenRepository) Create(
	ctx context.Context,
	token *domain.RefreshToken,
) error {
	var err error
	if token.CreatedAt.IsZero() {
		err = r.db.QueryRowContext(
			ctx,
			`INSERT INTO refresh_tokens (user_id, token_hash, expires_at, revoked_at, replaced_by_hash)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id, created_at`,
			token.UserID,
			token.TokenHash,
			token.ExpiresAt,
			token.RevokedAt,
			token.ReplacedByHash,
		).Scan(
			&token.ID,
			&token.CreatedAt,
		)
	} else {
		err = r.db.QueryRowContext(
			ctx,
			`INSERT INTO refresh_tokens (user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 RETURNING id`,
			token.UserID,
			token.TokenHash,
			token.ExpiresAt,
			token.CreatedAt,
			token.RevokedAt,
			token.ReplacedByHash,
		).Scan(
			&token.ID,
		)
	}

	return mapRefreshTokenError(err)
}

func (r *RefreshTokenRepository) GetByTokenHash(
	ctx context.Context,
	tokenHash string,
) (*domain.RefreshToken, error) {
	token := &domain.RefreshToken{}
	var revokedAt sql.NullTime
	var replacedByHash sql.NullString

	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash
		 FROM refresh_tokens
		 WHERE token_hash = $1`,
		tokenHash,
	).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.CreatedAt,
		&revokedAt,
		&replacedByHash,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrRefreshTokenNotFound
	}
	if err != nil {
		return nil, mapRefreshTokenError(err)
	}

	if revokedAt.Valid {
		token.RevokedAt = &revokedAt.Time
	}
	if replacedByHash.Valid {
		token.ReplacedByHash = &replacedByHash.String
	}

	return token, nil
}

func (r *RefreshTokenRepository) Rotate(
	ctx context.Context,
	oldTokenHash string,
	newToken *domain.RefreshToken,
) error {
	if oldTokenHash == "" || oldTokenHash != strings.TrimSpace(oldTokenHash) {
		return fmt.Errorf(
			"%w: invalid old token hash",
			domain.ErrInvalidRefreshToken,
		)
	}

	if newToken == nil {
		return fmt.Errorf(
			"%w: new token cannot be nil",
			domain.ErrInvalidRefreshToken,
		)
	}

	if strings.TrimSpace(newToken.TokenHash) == "" {
		return fmt.Errorf(
			"%w: new token hash cannot be empty",
			domain.ErrInvalidRefreshToken,
		)
	}

	if newToken.TokenHash == oldTokenHash {
		return fmt.Errorf(
			"%w: new token hash cannot match old token hash",
			domain.ErrInvalidRefreshToken,
		)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return mapRefreshTokenError(err)
	}
	defer tx.Rollback()

	oldToken := &domain.RefreshToken{}
	var revokedAt sql.NullTime
	var replacedByHash sql.NullString

	// Step 1 & 2: Lock and inspect old token row inside transaction
	err = tx.QueryRowContext(
		ctx,
		`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash
		 FROM refresh_tokens
		 WHERE token_hash = $1
		 FOR UPDATE`,
		oldTokenHash,
	).Scan(
		&oldToken.ID,
		&oldToken.UserID,
		&oldToken.TokenHash,
		&oldToken.ExpiresAt,
		&oldToken.CreatedAt,
		&revokedAt,
		&replacedByHash,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrRefreshTokenNotFound
	}
	if err != nil {
		return mapRefreshTokenError(err)
	}

	// Step 3: Check replacement, revocation, and expiration
	if replacedByHash.Valid && replacedByHash.String != "" {
		return domain.ErrRefreshTokenReused
	}

	if revokedAt.Valid {
		return domain.ErrRefreshTokenRevoked
	}

	now := time.Now().UTC()

	if !oldToken.ExpiresAt.UTC().After(now) {
		return domain.ErrRefreshTokenExpired
	}

	// Protect user ownership
	if newToken.UserID == 0 {
		newToken.UserID = oldToken.UserID
	} else if newToken.UserID != oldToken.UserID {
		return fmt.Errorf("%w: new token user ID %d does not match old token user ID %d", domain.ErrTokenOwnerMismatch, newToken.UserID, oldToken.UserID)
	}

	// Validate new token hash and expiry before insert
	if strings.TrimSpace(newToken.TokenHash) == "" {
		return fmt.Errorf("%w: new token hash cannot be empty", domain.ErrInvalidRefreshToken)
	}

	if newToken.ExpiresAt.IsZero() || !newToken.ExpiresAt.UTC().After(now) {
		return fmt.Errorf("%w: new token expiry must be in the future", domain.ErrInvalidRefreshToken)
	}

	// Step 4: Mark old token as revoked and replaced by new token hash
	res, err := tx.ExecContext(
		ctx,
		`UPDATE refresh_tokens
		 SET revoked_at = $1, replaced_by_hash = $2
		 WHERE id = $3`,
		now,
		newToken.TokenHash,
		oldToken.ID,
	)
	if err != nil {
		return mapRefreshTokenError(err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return mapRefreshTokenError(err)
	}
	if rows != 1 {
		return errors.New("abnormal rows affected during token rotation")
	}

	// Step 5: Insert new token
	if newToken.CreatedAt.IsZero() {
		err = tx.QueryRowContext(
			ctx,
			`INSERT INTO refresh_tokens (user_id, token_hash, expires_at, revoked_at, replaced_by_hash)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id, created_at`,
			newToken.UserID,
			newToken.TokenHash,
			newToken.ExpiresAt,
			newToken.RevokedAt,
			newToken.ReplacedByHash,
		).Scan(
			&newToken.ID,
			&newToken.CreatedAt,
		)
	} else {
		err = tx.QueryRowContext(
			ctx,
			`INSERT INTO refresh_tokens (user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 RETURNING id`,
			newToken.UserID,
			newToken.TokenHash,
			newToken.ExpiresAt,
			newToken.CreatedAt,
			newToken.RevokedAt,
			newToken.ReplacedByHash,
		).Scan(
			&newToken.ID,
		)
	}
	if err != nil {
		return mapRefreshTokenError(err)
	}

	// Step 6: Commit transaction
	return tx.Commit()
}

func (r *RefreshTokenRepository) Revoke(
	ctx context.Context,
	tokenHash string,
	revokedAt time.Time,
) error {
	var id int64
	err := r.db.QueryRowContext(
		ctx,
		`UPDATE refresh_tokens
		 SET revoked_at = COALESCE(revoked_at, $1)
		 WHERE token_hash = $2
		 RETURNING id`,
		revokedAt,
		tokenHash,
	).Scan(&id)

	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrRefreshTokenNotFound
	}
	if err != nil {
		return mapRefreshTokenError(err)
	}

	return nil
}

func (r *RefreshTokenRepository) RevokeAllForUser(
	ctx context.Context,
	userID int64,
	revokedAt time.Time,
) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE refresh_tokens
		 SET revoked_at = COALESCE(revoked_at, $1)
		 WHERE user_id = $2`,
		revokedAt,
		userID,
	)
	if err != nil {
		return mapRefreshTokenError(err)
	}

	return nil
}

var _ domain.RefreshTokenRepository = (*RefreshTokenRepository)(nil)
