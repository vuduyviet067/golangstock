package domain

import "errors"

var (
	ErrProductNotFound   = errors.New("product not found")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrProductSKUExists  = errors.New("product SKU already exists")
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrEmailExists  = errors.New("email already exists")
	ErrNotAdmin     = errors.New("user is not an admin")
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
