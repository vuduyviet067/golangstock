package usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/dto"
)

// A repository that returns no user and no error, (nil, nil), must be treated
// as "user not found" rather than dereferenced (Step 4.2). The PostgreSQL
// repository never does this; the guard keeps the use case safe for any
// implementation, as AuthUseCase already is.

// noPanic runs f and fails the test, instead of crashing it, if f panics.
func noPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	f()
}

func nilNilUserRepo() *mockUserRepo {
	return &mockUserRepo{
		getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) { return nil, nil },
	}
}

func TestUserGetByID_RepositoryReturnsNilNil(t *testing.T) {
	var (
		got dto.UserResponse
		err error
	)
	noPanic(t, func() {
		got, err = NewUserUseCase(nilNilUserRepo()).GetByID(context.Background(), 7)
	})

	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("error = %v, want ErrUserNotFound", err)
	}
	if !reflect.DeepEqual(got, dto.UserResponse{}) {
		t.Errorf("result = %+v, want the zero DTO", got)
	}
}

func TestUserUpdate_RepositoryReturnsNilNil(t *testing.T) {
	email, sameRole, newRole := "new@example.com", "user", "admin"
	inputs := map[string]dto.UpdateUserInput{
		"email change": {Email: &email},
		"same role":    {Role: &sameRole},
		"role change":  {Role: &newRole},
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			repo := nilNilUserRepo()
			var (
				got dto.UserResponse
				err error
			)
			noPanic(t, func() {
				got, err = NewUserUseCase(repo).Update(context.Background(), 7, input)
			})

			if !errors.Is(err, ErrUserNotFound) {
				t.Fatalf("error = %v, want ErrUserNotFound", err)
			}
			if !reflect.DeepEqual(got, dto.UserResponse{}) {
				t.Errorf("result = %+v, want the zero DTO", got)
			}
			// No write may follow a lookup that found nothing.
			if len(repo.updateCalls) != 0 || len(repo.updateAndRevokeCalls) != 0 {
				t.Errorf("repository writes = %d Update, %d UpdateAndRevokeTokens; want none",
					len(repo.updateCalls), len(repo.updateAndRevokeCalls))
			}
		})
	}
}

// The existing order of checks is kept: invalid input that is rejected before
// the lookup still reports its own error, and a lookup error wins over the
// nil guard.
func TestUserNilGuard_KeepsErrorOrder(t *testing.T) {
	uc := NewUserUseCase(nilNilUserRepo())
	email := "new@example.com"

	if _, err := uc.GetByID(context.Background(), 0); !errors.Is(err, ErrInvalidUserID) {
		t.Errorf("GetByID(0) error = %v, want ErrInvalidUserID", err)
	}
	if _, err := uc.Update(context.Background(), 0, dto.UpdateUserInput{Email: &email}); !errors.Is(err, ErrInvalidUserID) {
		t.Errorf("Update(0) error = %v, want ErrInvalidUserID", err)
	}
	if _, err := uc.Update(context.Background(), 7, dto.UpdateUserInput{}); !errors.Is(err, ErrEmptyUpdate) {
		t.Errorf("Update(empty) error = %v, want ErrEmptyUpdate", err)
	}

	dbErr := errors.New("connection reset")
	withErr := NewUserUseCase(&mockUserRepo{
		getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) { return nil, dbErr },
	})
	if _, err := withErr.GetByID(context.Background(), 7); !errors.Is(err, dbErr) {
		t.Errorf("GetByID lookup error = %v, want the repository error", err)
	}
	if _, err := withErr.Update(context.Background(), 7, dto.UpdateUserInput{Email: &email}); !errors.Is(err, dbErr) {
		t.Errorf("Update lookup error = %v, want the repository error", err)
	}
}
