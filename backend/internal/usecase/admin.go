package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gostock/internal/domain"
	"gostock/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

const adminRole = "admin"

type AdminUseCase struct {
	users repository.UserRepository
}

func NewAdminUseCase(users repository.UserRepository) *AdminUseCase {
	return &AdminUseCase{users: users}
}

func (uc *AdminUseCase) CreateAdmin(ctx context.Context, email, password string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !isValidEmail(email) {
		return nil, ErrInvalidEmail
	}
	if len(password) < passwordMinLen || len(password) > passwordMaxLen {
		return nil, ErrInvalidPassword
	}

	existing, err := uc.users.GetByEmail(ctx, email)
	switch {
	case errors.Is(err, domain.ErrUserNotFound):
		// The email is available.
	case err != nil:
		return nil, fmt.Errorf("check existing user: %w", err)
	case existing != nil:
		return nil, domain.ErrEmailExists
	default:
		// (nil, nil) violates the repository contract. Treat the email as
		// available: Create and the unique constraint still reject duplicates.
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	admin := &domain.User{
		Email:        email,
		PasswordHash: string(passwordHash),
		Role:         adminRole,
	}
	if err := uc.users.Create(ctx, admin); err != nil {
		if errors.Is(err, domain.ErrEmailExists) {
			// Lost a race with a concurrent creation of the same email.
			return nil, domain.ErrEmailExists
		}
		return nil, fmt.Errorf("create admin: %w", err)
	}

	// Return a copy without the hash, leaving the repository's object untouched.
	result := *admin
	result.PasswordHash = ""
	return &result, nil
}
