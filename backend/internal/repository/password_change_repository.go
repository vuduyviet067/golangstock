package repository

import (
	"context"
	"time"
)

// PasswordChangeRepository changes the password of any user, whatever its
// role: a user changing their own password (POST /api/v1/auth/change-password).
// AdminPasswordResetRepository, which only accepts admins, is separate.
type PasswordChangeRepository interface {
	// UpdateOwnPasswordAndRevokeTokens sets the password hash of the user and
	// revokes every refresh token of that user not revoked yet, at revokedAt,
	// in one transaction. A missing user is ErrUserNotFound.
	UpdateOwnPasswordAndRevokeTokens(ctx context.Context, userID int64, passwordHash string, revokedAt time.Time) error
}
