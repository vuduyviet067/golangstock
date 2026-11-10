package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/repository"
)

var (
	ErrInvalidUserID   = errors.New("invalid user ID")
	ErrInvalidUser     = errors.New("invalid user")
	ErrInvalidUserRole = errors.New("invalid user role")
	ErrEmptyUpdate     = errors.New("at least one field must be provided for update")
	ErrInvalidPage     = errors.New("page must be greater than or equal to 1")
	ErrInvalidLimit    = errors.New("limit must be between 1 and 100")
)

type UserUseCase struct {
	repo repository.UserManagementRepository
}

func NewUserUseCase(
	repo repository.UserManagementRepository,
) *UserUseCase {
	return &UserUseCase{
		repo: repo,
	}
}

func (uc *UserUseCase) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	if id <= 0 {
		return nil, ErrInvalidUserID
	}

	user, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	user.PasswordHash = ""
	return user, nil
}

func (uc *UserUseCase) List(
	ctx context.Context,
	query string,
	page, limit int,
) ([]domain.User, int64, error) {
	if page < 1 {
		return nil, 0, ErrInvalidPage
	}
	if limit < 1 || limit > 100 {
		return nil, 0, ErrInvalidLimit
	}

	query = strings.TrimSpace(query)
	users, total, err := uc.repo.List(ctx, query, page, limit)
	if err != nil {
		return nil, 0, err
	}

	for i := range users {
		users[i].PasswordHash = ""
	}

	return users, total, nil
}

func (uc *UserUseCase) Update(
	ctx context.Context,
	id int64,
	input dto.UpdateUserInput,
) (*domain.User, error) {
	if id <= 0 {
		return nil, ErrInvalidUserID
	}

	if input.Email == nil && input.Role == nil {
		return nil, ErrEmptyUpdate
	}

	existing, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	targetEmail := existing.Email
	targetRole := existing.Role
	roleChanged := false

	if input.Email != nil {
		normalizedEmail := strings.ToLower(strings.TrimSpace(*input.Email))
		if !isValidEmail(normalizedEmail) {
			return nil, ErrInvalidEmail
		}
		targetEmail = normalizedEmail
	}

	if input.Role != nil {
		trimmedRole := strings.TrimSpace(*input.Role)
		if !domain.IsValidRole(trimmedRole) {
			return nil, ErrInvalidUserRole
		}
		if trimmedRole != existing.Role {
			roleChanged = true
		}
		targetRole = trimmedRole
	}

	userToUpdate := &domain.User{
		ID:        existing.ID,
		Email:     targetEmail,
		Role:      targetRole,
		CreatedAt: existing.CreatedAt,
	}

	if roleChanged {
		if err := uc.repo.UpdateAndRevokeTokens(ctx, userToUpdate, time.Now().UTC()); err != nil {
			return nil, err
		}
	} else {
		if err := uc.repo.Update(ctx, userToUpdate); err != nil {
			return nil, err
		}
	}

	userToUpdate.PasswordHash = ""
	return userToUpdate, nil
}
