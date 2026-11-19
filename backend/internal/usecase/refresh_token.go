package usecase

import (
	"time"

	"gostock/internal/domain"
)

// Refresh token state checks used by the refresh flow. They were methods on
// domain.RefreshToken; internal/domain now declares only the struct.

func IsRefreshTokenRevoked(t *domain.RefreshToken) bool {
	return t.RevokedAt != nil
}

func IsRefreshTokenExpired(t *domain.RefreshToken, now time.Time) bool {
	return !t.ExpiresAt.After(now)
}

func IsRefreshTokenReplaced(t *domain.RefreshToken) bool {
	return t.ReplacedByHash != nil && *t.ReplacedByHash != ""
}

func IsRefreshTokenWithinGracePeriod(t *domain.RefreshToken, gracePeriod time.Duration, now time.Time) bool {
	if !IsRefreshTokenReplaced(t) || t.RevokedAt == nil || gracePeriod <= 0 {
		return false
	}
	elapsed := now.Sub(t.RevokedAt.UTC())
	return elapsed >= 0 && elapsed <= gracePeriod
}
