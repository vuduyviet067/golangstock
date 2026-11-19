package usecase

import (
	"errors"
	"fmt"
	"testing"

	"gostock/internal/repository"
)

func TestFromRepository(t *testing.T) {
	pairs := []struct {
		repo, app error
	}{
		{repository.ErrProductNotFound, ErrProductNotFound},
		{repository.ErrInsufficientStock, ErrInsufficientStock},
		{repository.ErrProductSKUExists, ErrProductSKUExists},
		{repository.ErrInvalidActor, ErrInvalidActor},
		{repository.ErrUserNotFound, ErrUserNotFound},
		{repository.ErrEmailExists, ErrEmailExists},
		{repository.ErrNotAdmin, ErrNotAdmin},
		{repository.ErrRefreshTokenNotFound, ErrRefreshTokenNotFound},
		{repository.ErrRefreshTokenRevoked, ErrRefreshTokenRevoked},
		{repository.ErrRefreshTokenExpired, ErrRefreshTokenExpired},
		{repository.ErrRefreshTokenReused, ErrRefreshTokenReused},
		{repository.ErrInvalidRefreshToken, ErrInvalidRefreshToken},
	}

	for _, p := range pairs {
		t.Run(p.repo.Error(), func(t *testing.T) {
			got := fromRepository(p.repo)
			if got != p.app {
				t.Fatalf("fromRepository(%v) = %#v, want the application error", p.repo, got)
			}
			if got.Error() != p.repo.Error() {
				t.Errorf("message = %q, want %q", got.Error(), p.repo.Error())
			}

			wrapped := fromRepository(fmt.Errorf("%w: detail", p.repo))
			if !errors.Is(wrapped, p.app) {
				t.Errorf("wrapped: errors.Is(%v, application error) = false", wrapped)
			}
			if errors.Is(wrapped, p.repo) {
				t.Errorf("wrapped: %v still matches the repository error", wrapped)
			}
		})
	}

	t.Run("nil", func(t *testing.T) {
		if got := fromRepository(nil); got != nil {
			t.Fatalf("fromRepository(nil) = %v, want nil", got)
		}
	})

	t.Run("wrapped keeps details", func(t *testing.T) {
		err := fmt.Errorf("%w: new token hash cannot be empty", repository.ErrInvalidRefreshToken)
		got := fromRepository(err)
		want := "invalid refresh token: invalid refresh token: new token hash cannot be empty"
		if got.Error() != want {
			t.Errorf("message = %q, want %q", got.Error(), want)
		}
	})

	t.Run("other errors unchanged", func(t *testing.T) {
		for _, err := range []error{
			errors.New("connection reset"),
			repository.ErrTokenOwnerMismatch,
			ErrInvalidSession,
		} {
			if got := fromRepository(err); got != err {
				t.Errorf("fromRepository(%v) = %v, want it unchanged", err, got)
			}
		}
	})
}
