package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrRefreshTokenNotFound = errors.New("refresh token not found")
	ErrRefreshTokenRevoked  = errors.New("refresh token has been revoked")
	ErrRefreshTokenExpired  = errors.New("refresh token has expired")
	ErrRefreshTokenReused   = errors.New("refresh token has already been used and replaced")
	ErrConcurrentRefresh    = errors.New("concurrent refresh in progress")
	ErrTokenOwnerMismatch   = errors.New("refresh token user mismatch")
	ErrInvalidRefreshToken  = errors.New("invalid refresh token")
)

type RefreshToken struct {
	ID             int64
	UserID         int64
	TokenHash      string
	ExpiresAt      time.Time
	CreatedAt      time.Time
	RevokedAt      *time.Time
	ReplacedByHash *string
}

// IsRevoked returns true if the refresh token has been revoked.
func (t *RefreshToken) IsRevoked() bool {
	return t.RevokedAt != nil
}

// IsExpired reports whether the refresh token has expired as of now.
func (t *RefreshToken) IsExpired(now time.Time) bool {
	return !t.ExpiresAt.After(now)
}

// IsReplaced reports whether the refresh token has already been rotated.
func (t *RefreshToken) IsReplaced() bool {
	return t.ReplacedByHash != nil && *t.ReplacedByHash != ""
}

// IsWithinGracePeriod reports whether the token was replaced within the specified grace period.
func (t *RefreshToken) IsWithinGracePeriod(gracePeriod time.Duration, now time.Time) bool {
	if !t.IsReplaced() || t.RevokedAt == nil || gracePeriod <= 0 {
		return false
	}
	elapsed := now.Sub(t.RevokedAt.UTC())
	return elapsed >= 0 && elapsed <= gracePeriod
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *RefreshToken) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	Rotate(ctx context.Context, oldTokenHash string, newToken *RefreshToken) error
	Revoke(ctx context.Context, tokenHash string, revokedAt time.Time) error
	RevokeAllForUser(ctx context.Context, userID int64, revokedAt time.Time) error
}
