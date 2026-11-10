package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/repository"
)

type mockUserRepo struct {
	createFn                func(ctx context.Context, user *domain.User) error
	getByEmailFn            func(ctx context.Context, email string) (*domain.User, error)
	getByIDFn               func(ctx context.Context, id int64) (*domain.User, error)
	listFn                  func(ctx context.Context, query string, page, limit int) ([]domain.User, int64, error)
	updateFn                func(ctx context.Context, user *domain.User) error
	updateAndRevokeTokensFn func(ctx context.Context, user *domain.User, revokedAt time.Time) error

	updateCalls          []*domain.User
	updateAndRevokeCalls []*domain.User
}

func (m *mockUserRepo) Create(ctx context.Context, user *domain.User) error {
	if m.createFn != nil {
		return m.createFn(ctx, user)
	}
	return nil
}

func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if m.getByEmailFn != nil {
		return m.getByEmailFn(ctx, email)
	}
	return nil, domain.ErrUserNotFound
}

func (m *mockUserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, domain.ErrUserNotFound
}

func (m *mockUserRepo) List(ctx context.Context, query string, page, limit int) ([]domain.User, int64, error) {
	if m.listFn != nil {
		return m.listFn(ctx, query, page, limit)
	}
	return []domain.User{}, 0, nil
}

func (m *mockUserRepo) Update(ctx context.Context, user *domain.User) error {
	m.updateCalls = append(m.updateCalls, user)
	if m.updateFn != nil {
		return m.updateFn(ctx, user)
	}
	return nil
}

func (m *mockUserRepo) UpdateAndRevokeTokens(ctx context.Context, user *domain.User, revokedAt time.Time) error {
	m.updateAndRevokeCalls = append(m.updateAndRevokeCalls, user)
	if m.updateAndRevokeTokensFn != nil {
		return m.updateAndRevokeTokensFn(ctx, user, revokedAt)
	}
	return nil
}

var _ repository.UserManagementRepository = (*mockUserRepo)(nil)

func TestUserUseCase_GetByID(t *testing.T) {
	t.Run("success clears PasswordHash", func(t *testing.T) {
		repo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				return &domain.User{
					ID:           id,
					Email:        "user@example.com",
					PasswordHash: "$2a$10$secretHashThatMustNotLeak",
					Role:         "user",
					CreatedAt:    time.Now().Add(-24 * time.Hour),
					UpdatedAt:    time.Now(),
				}, nil
			},
		}
		uc := NewUserUseCase(repo)

		user, err := uc.GetByID(context.Background(), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.ID != 1 {
			t.Errorf("expected ID 1, got %d", user.ID)
		}
		if user.PasswordHash != "" {
			t.Errorf("expected PasswordHash to be empty, got %q", user.PasswordHash)
		}
	})

	t.Run("invalid user ID", func(t *testing.T) {
		uc := NewUserUseCase(&mockUserRepo{})

		for _, invalidID := range []int64{0, -1, -99} {
			_, err := uc.GetByID(context.Background(), invalidID)
			if !errors.Is(err, ErrInvalidUserID) {
				t.Errorf("for id %d, expected ErrInvalidUserID, got %v", invalidID, err)
			}
		}
	})

	t.Run("user not found", func(t *testing.T) {
		repo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				return nil, domain.ErrUserNotFound
			},
		}
		uc := NewUserUseCase(repo)

		_, err := uc.GetByID(context.Background(), 999)
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("expected ErrUserNotFound, got %v", err)
		}
	})
}

func TestUserUseCase_List(t *testing.T) {
	t.Run("success clears PasswordHash on all items", func(t *testing.T) {
		repo := &mockUserRepo{
			listFn: func(ctx context.Context, query string, page, limit int) ([]domain.User, int64, error) {
				return []domain.User{
					{
						ID:           1,
						Email:        "admin@example.com",
						PasswordHash: "$2a$10$secretAdminHash",
						Role:         "admin",
					},
					{
						ID:           2,
						Email:        "user@example.com",
						PasswordHash: "$2a$10$secretUserHash",
						Role:         "user",
					},
				}, 2, nil
			},
		}
		uc := NewUserUseCase(repo)

		users, total, err := uc.List(context.Background(), "", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 2 {
			t.Errorf("expected total 2, got %d", total)
		}
		if len(users) != 2 {
			t.Fatalf("expected 2 users, got %d", len(users))
		}
		for i, u := range users {
			if u.PasswordHash != "" {
				t.Errorf("user %d: expected empty PasswordHash, got %q", i, u.PasswordHash)
			}
		}
	})

	t.Run("invalid page", func(t *testing.T) {
		uc := NewUserUseCase(&mockUserRepo{})
		_, _, err := uc.List(context.Background(), "", 0, 10)
		if !errors.Is(err, ErrInvalidPage) {
			t.Errorf("expected ErrInvalidPage, got %v", err)
		}
		_, _, err = uc.List(context.Background(), "", -5, 10)
		if !errors.Is(err, ErrInvalidPage) {
			t.Errorf("expected ErrInvalidPage, got %v", err)
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		uc := NewUserUseCase(&mockUserRepo{})
		for _, invalidLimit := range []int{0, -1, 101, 500} {
			_, _, err := uc.List(context.Background(), "", 1, invalidLimit)
			if !errors.Is(err, ErrInvalidLimit) {
				t.Errorf("for limit %d: expected ErrInvalidLimit, got %v", invalidLimit, err)
			}
		}
	})
}

func TestUserUseCase_Update(t *testing.T) {
	existingUser := &domain.User{
		ID:           10,
		Email:        "existing@example.com",
		PasswordHash: "$2a$10$someSecretHash",
		Role:         "user",
		CreatedAt:    time.Now().Add(-48 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}

	t.Run("invalid user ID", func(t *testing.T) {
		uc := NewUserUseCase(&mockUserRepo{})
		newEmail := "new@example.com"
		_, err := uc.Update(context.Background(), 0, dto.UpdateUserInput{Email: &newEmail})
		if !errors.Is(err, ErrInvalidUserID) {
			t.Errorf("expected ErrInvalidUserID, got %v", err)
		}
	})

	t.Run("empty update", func(t *testing.T) {
		uc := NewUserUseCase(&mockUserRepo{})
		_, err := uc.Update(context.Background(), 10, dto.UpdateUserInput{})
		if !errors.Is(err, ErrEmptyUpdate) {
			t.Errorf("expected ErrEmptyUpdate, got %v", err)
		}
	})

	t.Run("user not found", func(t *testing.T) {
		repo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				return nil, domain.ErrUserNotFound
			},
		}
		uc := NewUserUseCase(repo)
		role := "admin"
		_, err := uc.Update(context.Background(), 99, dto.UpdateUserInput{Role: &role})
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("expected ErrUserNotFound, got %v", err)
		}
	})

	t.Run("invalid email", func(t *testing.T) {
		repo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				return existingUser, nil
			},
		}
		uc := NewUserUseCase(repo)

		for _, badEmail := range []string{"not-an-email", "@no-user.com", "user@", "user @example.com", "user@nodot"} {
			email := badEmail
			_, err := uc.Update(context.Background(), 10, dto.UpdateUserInput{Email: &email})
			if !errors.Is(err, ErrInvalidEmail) {
				t.Errorf("for email %q: expected ErrInvalidEmail, got %v", badEmail, err)
			}
		}
	})

	t.Run("invalid role", func(t *testing.T) {
		repo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				return existingUser, nil
			},
		}
		uc := NewUserUseCase(repo)

		for _, badRole := range []string{"superadmin", "moderator", "", "USER", "ADMIN "} {
			role := badRole
			_, err := uc.Update(context.Background(), 10, dto.UpdateUserInput{Role: &role})
			if !errors.Is(err, ErrInvalidUserRole) {
				t.Errorf("for role %q: expected ErrInvalidUserRole, got %v", badRole, err)
			}
		}
	})

	t.Run("email normalization: trims and lowercases email; does not invoke token revocation", func(t *testing.T) {
		repo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				u := *existingUser
				return &u, nil
			},
		}
		uc := NewUserUseCase(repo)

		emailInput := "   NEW.EMAIL@EXAMPLE.COM   "
		res, err := uc.Update(context.Background(), 10, dto.UpdateUserInput{Email: &emailInput})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Email != "new.email@example.com" {
			t.Errorf("expected normalized email 'new.email@example.com', got %q", res.Email)
		}
		if len(repo.updateCalls) != 1 {
			t.Fatalf("expected 1 call to Update, got %d", len(repo.updateCalls))
		}
		if repo.updateCalls[0].Email != "new.email@example.com" {
			t.Errorf("expected repo to receive normalized email, got %s", repo.updateCalls[0].Email)
		}
		if len(repo.updateAndRevokeCalls) != 0 {
			t.Errorf("expected 0 calls to UpdateAndRevokeTokens on email-only update, got %d", len(repo.updateAndRevokeCalls))
		}
		if res.PasswordHash != "" {
			t.Errorf("expected PasswordHash to be cleared, got %q", res.PasswordHash)
		}
	})

	t.Run("role unchanged does not invoke token revocation", func(t *testing.T) {
		repo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				u := *existingUser // role is "user"
				return &u, nil
			},
		}
		uc := NewUserUseCase(repo)

		roleInput := "user" // same as existing
		_, err := uc.Update(context.Background(), 10, dto.UpdateUserInput{Role: &roleInput})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(repo.updateCalls) != 1 {
			t.Fatalf("expected 1 call to Update, got %d", len(repo.updateCalls))
		}
		if len(repo.updateAndRevokeCalls) != 0 {
			t.Errorf("expected 0 calls to UpdateAndRevokeTokens when role unchanged, got %d", len(repo.updateAndRevokeCalls))
		}
	})

	t.Run("role changed invokes the atomic repository operation UpdateAndRevokeTokens", func(t *testing.T) {
		repo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				u := *existingUser // role is "user"
				return &u, nil
			},
		}
		uc := NewUserUseCase(repo)

		newRole := "admin" // changing from user to admin
		res, err := uc.Update(context.Background(), 10, dto.UpdateUserInput{Role: &newRole})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Role != "admin" {
			t.Errorf("expected updated role admin, got %s", res.Role)
		}
		if len(repo.updateAndRevokeCalls) != 1 {
			t.Fatalf("expected 1 call to UpdateAndRevokeTokens, got %d", len(repo.updateAndRevokeCalls))
		}
		if repo.updateAndRevokeCalls[0].Role != "admin" {
			t.Errorf("expected updated role 'admin', got %s", repo.updateAndRevokeCalls[0].Role)
		}
		if len(repo.updateCalls) != 0 {
			t.Errorf("expected 0 calls to standard Update when role changed, got %d", len(repo.updateCalls))
		}
		if res.PasswordHash != "" {
			t.Errorf("expected PasswordHash to be cleared, got %q", res.PasswordHash)
		}
	})

	t.Run("role surrounded by whitespace is trimmed and stored as admin", func(t *testing.T) {
		repo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				u := *existingUser // role is "user"
				return &u, nil
			},
		}
		uc := NewUserUseCase(repo)

		paddedRole := " admin "
		res, err := uc.Update(context.Background(), 10, dto.UpdateUserInput{Role: &paddedRole})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Role != "admin" {
			t.Errorf("expected returned role %q, got %q", "admin", res.Role)
		}
		if len(repo.updateAndRevokeCalls) != 1 {
			t.Fatalf("expected 1 call to UpdateAndRevokeTokens, got %d", len(repo.updateAndRevokeCalls))
		}
		if repo.updateAndRevokeCalls[0].Role != "admin" {
			t.Errorf("expected stored role %q, got %q", "admin", repo.updateAndRevokeCalls[0].Role)
		}
	})

	t.Run("atomic repository failure is propagated", func(t *testing.T) {
		repoErr := errors.New("database transaction failure")
		repo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				u := *existingUser
				return &u, nil
			},
			updateAndRevokeTokensFn: func(ctx context.Context, user *domain.User, revokedAt time.Time) error {
				return repoErr
			},
		}
		uc := NewUserUseCase(repo)

		newRole := "admin"
		_, err := uc.Update(context.Background(), 10, dto.UpdateUserInput{Role: &newRole})
		if err == nil {
			t.Fatal("expected error when atomic operation fails, got nil")
		}
		if !errors.Is(err, repoErr) {
			t.Errorf("expected repoErr, got %v", err)
		}
	})
}
