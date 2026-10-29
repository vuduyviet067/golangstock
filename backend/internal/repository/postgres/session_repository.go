package postgres

import (
	"context"
	"database/sql"
	"gostock/internal/domain"
	"time"
)

type SessionRepository struct {
	db *sql.DB
}

func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(
	ctx context.Context,
	session *domain.Session,
) error {
	return r.db.QueryRowContext(
		ctx,
		`INSERT INTO sessions (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at`,
		session.UserID,
		session.TokenHash,
		session.ExpiresAt,
	).Scan(
		&session.ID,
		&session.CreatedAt,
	)
}

func (r *SessionRepository) GetByTokenHash(
	ctx context.Context,
	tokenHash string,
) (*domain.Session, error) {
	session := &domain.Session{}

	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, user_id, token_hash, expires_at, created_at
		 FROM sessions
		 WHERE token_hash = $1`,
		tokenHash,
	).Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&session.ExpiresAt,
		&session.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	return session, nil
}

func (r *SessionRepository) DeleteByTokenHash(
	ctx context.Context,
	tokenHash string,
) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM sessions WHERE token_hash = $1`,
		tokenHash,
	)
	return err
}

func (r *SessionRepository) DeleteExpired(
	ctx context.Context,
	now time.Time,
) error {
	_, err := r.db.ExecContext(
		ctx,
		`DELETE FROM sessions WHERE expires_at <= $1`,
		now,
	)
	return err
}

var _ domain.SessionRepository = (*SessionRepository)(nil)
