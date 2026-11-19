package repository

import (
	"context"
	"time"

	"gostock/internal/domain"
)

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *domain.RefreshToken) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	Rotate(ctx context.Context, oldTokenHash string, newToken *domain.RefreshToken) error
	Revoke(ctx context.Context, tokenHash string, revokedAt time.Time) error
	RevokeAllForUser(ctx context.Context, userID int64, revokedAt time.Time) error
}
