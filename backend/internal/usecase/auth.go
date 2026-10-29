package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"gostock/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidEmail       = errors.New("invalid email")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrInvalidSession     = errors.New("invalid session")
)

const (
	defaultUserRole = "user"
	passwordMinLen  = 8
	passwordMaxLen  = 72
)

type AuthUseCase struct {
	users      domain.UserRepository
	sessions   domain.SessionRepository
	sessionTTL time.Duration
}
type LoginResult struct {
	User      *domain.User
	Token     string
	ExpiresAt time.Time
}

func NewAuthUseCase(
	users domain.UserRepository,
	sessions domain.SessionRepository,
	sessionTTL time.Duration,
) *AuthUseCase {
	return &AuthUseCase{
		users:      users,
		sessions:   sessions,
		sessionTTL: sessionTTL,
	}
}

func (uc *AuthUseCase) Register(
	ctx context.Context,
	email string,
	password string,
) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	if !isValidEmail(email) {
		return nil, ErrInvalidEmail
	}

	if len(password) < passwordMinLen || len(password) > passwordMaxLen {
		return nil, ErrInvalidPassword
	}

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		Email:        email,
		PasswordHash: string(passwordHash),
		Role:         defaultUserRole,
	}

	if err := uc.users.Create(ctx, user); err != nil {
		return nil, err
	}

	user.PasswordHash = ""

	return user, nil
}

func isValidEmail(email string) bool {
	if email == "" || len(email) > 255 {
		return false
	}

	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return false
	}

	if strings.ContainsAny(email, " \t\r\n") {
		return false
	}

	return strings.Contains(email[at+1:], ".")
}
func (uc *AuthUseCase) Login(
	ctx context.Context,
	email string,
	password string,
) (*LoginResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	if !isValidEmail(email) || password == "" {
		return nil, ErrInvalidCredentials
	}

	user, err := uc.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash),
		[]byte(password),
	); err != nil {
		return nil, ErrInvalidCredentials
	}

	if uc.sessionTTL <= 0 {
		return nil, errors.New("session TTL must be positive")
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}

	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	tokenHash := sha256.Sum256([]byte(token))

	session := &domain.Session{
		UserID:    user.ID,
		TokenHash: base64.RawURLEncoding.EncodeToString(tokenHash[:]),
		ExpiresAt: time.Now().Add(uc.sessionTTL),
	}

	if err := uc.sessions.Create(ctx, session); err != nil {
		return nil, err
	}

	user.PasswordHash = ""

	return &LoginResult{
		User:      user,
		Token:     token,
		ExpiresAt: session.ExpiresAt,
	}, nil
}
func (uc *AuthUseCase) Authenticate(
	ctx context.Context,
	token string,
) (*domain.User, error) {
	if token == "" {
		return nil, ErrInvalidSession
	}

	tokenHash := sha256.Sum256([]byte(token))
	encodedHash := base64.RawURLEncoding.EncodeToString(tokenHash[:])

	session, err := uc.sessions.GetByTokenHash(ctx, encodedHash)
	if err != nil || session == nil {
		return nil, ErrInvalidSession
	}

	if !session.ExpiresAt.After(time.Now()) {
		return nil, ErrInvalidSession
	}

	user, err := uc.users.GetByID(ctx, session.UserID)
	if err != nil || user == nil {
		return nil, ErrInvalidSession
	}

	user.PasswordHash = ""
	return user, nil
}
func (uc *AuthUseCase) Logout(
	ctx context.Context,
	token string,
) error {
	if token == "" {
		return ErrInvalidSession
	}

	tokenHash := sha256.Sum256([]byte(token))
	encodedHash := base64.RawURLEncoding.EncodeToString(tokenHash[:])

	return uc.sessions.DeleteByTokenHash(ctx, encodedHash)
}
