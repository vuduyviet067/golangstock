package dto

import (
	"time"

	"gostock/internal/domain"
)

// LoginRequest represents the POST /api/v1/auth/login body.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterRequest represents the POST /api/v1/auth/register body.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthUserResponse is the user payload returned by the register, login,
// refresh, and me endpoints. CreatedAt is RFC 3339, or empty when unknown.
type AuthUserResponse struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

// LoginResult is the outcome of a successful login or token refresh.
type LoginResult struct {
	User                  *domain.User
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
}
