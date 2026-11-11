package repository

import "errors"

// Errors returned by the repository implementations for persistence outcomes:
// a missing row, a violated unique or foreign key constraint, a conditional
// update that matched no row, or a record the store refuses to write. The use
// cases convert them to their own application errors; callers outside the use
// cases should not depend on them.
var (
	// ErrProductNotFound means no product has the given ID.
	ErrProductNotFound = errors.New("product not found")
	// ErrInsufficientStock means a stock-out would make the quantity negative.
	ErrInsufficientStock = errors.New("insufficient stock")
	// ErrProductSKUExists means another product already has the SKU.
	ErrProductSKUExists = errors.New("product SKU already exists")
	// ErrInvalidActor means the user recorded on a stock movement does not
	// exist.
	ErrInvalidActor = errors.New("invalid actor")
)

var (
	// ErrUserNotFound means no user has the given ID or email, including the
	// owner of a refresh token being stored.
	ErrUserNotFound = errors.New("user not found")
	// ErrEmailExists means another user already has the email.
	ErrEmailExists = errors.New("email already exists")
	// ErrNotAdmin means the user whose admin password is being reset is not,
	// at the time of the update, an admin.
	ErrNotAdmin = errors.New("user is not an admin")
)

var (
	// ErrRefreshTokenNotFound means no refresh token has the given hash.
	ErrRefreshTokenNotFound = errors.New("refresh token not found")
	// ErrRefreshTokenRevoked, ErrRefreshTokenExpired and ErrRefreshTokenReused
	// mean the token being rotated was revoked, has expired or was already
	// replaced when the rotation locked it.
	ErrRefreshTokenRevoked = errors.New("refresh token has been revoked")
	ErrRefreshTokenExpired = errors.New("refresh token has expired")
	ErrRefreshTokenReused  = errors.New("refresh token has already been used and replaced")
	// ErrTokenOwnerMismatch means a rotation would hand the session to another
	// user.
	ErrTokenOwnerMismatch = errors.New("refresh token user mismatch")
	// ErrInvalidRefreshToken means a token hash or record passed to the
	// repository is malformed.
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
)
