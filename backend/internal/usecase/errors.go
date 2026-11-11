package usecase

import (
	"errors"
	"fmt"

	"gostock/internal/repository"
)

// Application errors returned by the use cases and mapped to HTTP responses by
// the handlers. Several have a persistence counterpart in internal/repository
// with the same message; fromRepository converts one into the other.
var (
	ErrProductNotFound   = errors.New("product not found")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrProductSKUExists  = errors.New("product SKU already exists")

	ErrInvalidActor = errors.New("invalid actor")
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
	ErrInvalidRefreshToken  = errors.New("invalid refresh token")
)

// fromRepository converts a repository error to the application error with
// the same meaning. A repository error wrapped with details keeps them in the
// message. Other errors, including repository.ErrTokenOwnerMismatch (an
// internal inconsistency), are returned unchanged.
func fromRepository(err error) error {
	var app error
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrProductNotFound):
		app = ErrProductNotFound
	case errors.Is(err, repository.ErrInsufficientStock):
		app = ErrInsufficientStock
	case errors.Is(err, repository.ErrProductSKUExists):
		app = ErrProductSKUExists
	case errors.Is(err, repository.ErrInvalidActor):
		app = ErrInvalidActor
	case errors.Is(err, repository.ErrUserNotFound):
		app = ErrUserNotFound
	case errors.Is(err, repository.ErrEmailExists):
		app = ErrEmailExists
	case errors.Is(err, repository.ErrNotAdmin):
		app = ErrNotAdmin
	case errors.Is(err, repository.ErrRefreshTokenNotFound):
		app = ErrRefreshTokenNotFound
	case errors.Is(err, repository.ErrRefreshTokenRevoked):
		app = ErrRefreshTokenRevoked
	case errors.Is(err, repository.ErrRefreshTokenExpired):
		app = ErrRefreshTokenExpired
	case errors.Is(err, repository.ErrRefreshTokenReused):
		app = ErrRefreshTokenReused
	case errors.Is(err, repository.ErrInvalidRefreshToken):
		app = ErrInvalidRefreshToken
	default:
		return err
	}

	// A sentinel does not wrap anything; a wrapped one carries details.
	if errors.Unwrap(err) == nil {
		return app
	}
	return fmt.Errorf("%w: %v", app, err)
}
