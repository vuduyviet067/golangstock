package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/mapper"
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

// GetByID returns the user as the user management response DTO, which has no
// password field. Its timestamps keep their full precision and offset (JSON
// RFC3339Nano).
func (uc *UserUseCase) GetByID(ctx context.Context, id int64) (dto.UserResponse, error) {
	if id <= 0 {
		return dto.UserResponse{}, ErrInvalidUserID
	}

	user, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return dto.UserResponse{}, fromRepository(err)
	}
	if user == nil {
		return dto.UserResponse{}, ErrUserNotFound
	}

	user.PasswordHash = ""
	return mapper.ToUserResponse(*user), nil
}

// List returns one page of users as response DTOs in the repository's order,
// never nil (an empty page is []), and the repository's total.
func (uc *UserUseCase) List(
	ctx context.Context,
	query string,
	page, limit int,
) ([]dto.UserResponse, int64, error) {
	if page < 1 {
		return nil, 0, ErrInvalidPage
	}
	if limit < 1 || limit > 100 {
		return nil, 0, ErrInvalidLimit
	}

	query = strings.TrimSpace(query)
	users, total, err := uc.repo.List(ctx, query, page, limit)
	if err != nil {
		return nil, 0, fromRepository(err)
	}

	for i := range users {
		users[i].PasswordHash = ""
	}

	return mapper.ToUserResponses(users), total, nil
}

// Update changes the user's email and/or role and returns the updated user as
// a response DTO: created_at is the stored value and updated_at is the one
// the repository reports.
func (uc *UserUseCase) Update(
	ctx context.Context,
	id int64,
	input dto.UpdateUserInput,
) (dto.UserResponse, error) {
	if id <= 0 {
		return dto.UserResponse{}, ErrInvalidUserID
	}

	if input.Email == nil && input.Role == nil {
		return dto.UserResponse{}, ErrEmptyUpdate
	}

	existing, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return dto.UserResponse{}, fromRepository(err)
	}
	if existing == nil {
		return dto.UserResponse{}, ErrUserNotFound
	}

	targetEmail := existing.Email
	targetRole := existing.Role
	roleChanged := false

	if input.Email != nil {
		normalizedEmail := strings.ToLower(strings.TrimSpace(*input.Email))
		if !isValidEmail(normalizedEmail) {
			return dto.UserResponse{}, ErrInvalidEmail
		}
		targetEmail = normalizedEmail
	}

	if input.Role != nil {
		trimmedRole := strings.TrimSpace(*input.Role)
		if !IsValidRole(trimmedRole) {
			return dto.UserResponse{}, ErrInvalidUserRole
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
			return dto.UserResponse{}, fromRepository(err)
		}
	} else {
		if err := uc.repo.Update(ctx, userToUpdate); err != nil {
			return dto.UserResponse{}, fromRepository(err)
		}
	}

	userToUpdate.PasswordHash = ""
	return mapper.ToUserResponse(*userToUpdate), nil
}
