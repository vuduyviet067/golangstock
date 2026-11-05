package domain

import "errors"

// Domain errors, grouped by the entity they describe. Callers match them with
// errors.Is; implementations may wrap them with additional context.

// Product errors.
var (
	ErrProductNotFound   = errors.New("product not found")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrProductSKUExists  = errors.New("product SKU already exists")
)

// User errors.
var (
	ErrUserNotFound = errors.New("user not found")
	ErrEmailExists  = errors.New("email already exists")
)

// Refresh token errors.
var (
	ErrRefreshTokenNotFound = errors.New("refresh token not found")
	ErrRefreshTokenRevoked  = errors.New("refresh token has been revoked")
	ErrRefreshTokenExpired  = errors.New("refresh token has expired")
	ErrRefreshTokenReused   = errors.New("refresh token has already been used and replaced")
	ErrConcurrentRefresh    = errors.New("concurrent refresh in progress")
	ErrTokenOwnerMismatch   = errors.New("refresh token user mismatch")
	ErrInvalidRefreshToken  = errors.New("invalid refresh token")
)
