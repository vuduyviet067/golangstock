package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gostock/internal/domain"
	"gostock/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

type AdminUseCase struct {
	users         repository.UserRepository
	passwordReset repository.AdminPasswordResetRepository
}

func NewAdminUseCase(users repository.UserRepository) *AdminUseCase {
	return &AdminUseCase{users: users}
}

func NewAdminUseCaseWithPasswordReset(
	users repository.UserRepository,
	passwordReset repository.AdminPasswordResetRepository,
) *AdminUseCase {
	return &AdminUseCase{
		users:         users,
		passwordReset: passwordReset,
	}
}

func (uc *AdminUseCase) CreateAdmin(
	ctx context.Context,
	email, password string,
) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !isValidEmail(email) {
		return nil, ErrInvalidEmail
	}
	if len(password) < passwordMinLen || len(password) > passwordMaxLen {
		return nil, ErrInvalidPassword
	}

	existing, err := uc.users.GetByEmail(ctx, email)
	err = fromRepository(err)
	switch {
	case errors.Is(err, ErrUserNotFound):
		// The email is available.
	case err != nil:
		return nil, fmt.Errorf("check existing user: %w", err)
	case existing != nil:
		return nil, ErrEmailExists
	default:
		// Create and the unique constraint still reject duplicates.
	}

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	admin := &domain.User{
		Email:        email,
		PasswordHash: string(passwordHash),
		Role:         RoleAdmin,
	}

	if err := fromRepository(uc.users.Create(ctx, admin)); err != nil {
		if errors.Is(err, ErrEmailExists) {
			return nil, ErrEmailExists
		}
		return nil, fmt.Errorf("create admin: %w", err)
	}

	result := *admin
	result.PasswordHash = ""
	return &result, nil
}

func (uc *AdminUseCase) ResetAdminPassword(
	ctx context.Context,
	email string,
	password string,
) error {
	if uc.passwordReset == nil {
		return errors.New("admin password reset repository is not configured")
	}

	email = strings.ToLower(strings.TrimSpace(email))
	if !isValidEmail(email) {
		return ErrInvalidEmail
	}

	if len(password) < passwordMinLen || len(password) > passwordMaxLen {
		return ErrInvalidPassword
	}

	user, err := uc.passwordReset.GetByEmail(ctx, email)
	err = fromRepository(err)
	if errors.Is(err, ErrUserNotFound) {
		return ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("find admin account: %w", err)
	}
	if user == nil {
		return errors.New("user repository returned nil user without error")
	}
	if user.Role != RoleAdmin {
		return ErrNotAdmin
	}

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := uc.passwordReset.UpdatePasswordAndRevokeTokens(
		ctx,
		user.ID,
		string(passwordHash),
		time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("reset admin password: %w", fromRepository(err))
	}

	return nil
}
