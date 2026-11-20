package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/mapper"
	"gostock/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidEmail       = errors.New("invalid email")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrInvalidSession     = errors.New("invalid session")
	ErrInvalidTokenTTL    = errors.New("token TTLs must be positive")

	// ErrRegistrationDisabled: self-registration is off (ALLOW_REGISTRATION).
	ErrRegistrationDisabled = errors.New("registration is disabled")

	// ChangePassword: the current password given is wrong, or the new
	// password is the current one.
	ErrInvalidCurrentPassword = errors.New("current password is incorrect")
	ErrPasswordUnchanged      = errors.New("new password must differ from the current password")
)

const (
	defaultUserRole = RoleUser
	passwordMinLen  = 8
	passwordMaxLen  = 72
)

// TokenManager abstracts JWT token generation and validation.
type TokenManager interface {
	GenerateAccessToken(userID int64, email, role string) (string, time.Time, error)
	ValidateAccessToken(tokenStr string) (*dto.TokenClaims, error)
}

type AuthUseCase struct {
	users               repository.UserRepository
	refreshTokens       repository.RefreshTokenRepository
	tokenManager        TokenManager
	accessTokenTTL      time.Duration
	refreshTokenTTL     time.Duration
	rotationGracePeriod time.Duration
	registrationAllowed bool
	passwordChange      repository.PasswordChangeRepository
}

// AuthOption configures an AuthUseCase.
type AuthOption func(*AuthUseCase)

// WithRegistration sets whether Register may create accounts. Without it,
// registration is disabled and Register returns ErrRegistrationDisabled.
func WithRegistration(allowed bool) AuthOption {
	return func(uc *AuthUseCase) { uc.registrationAllowed = allowed }
}

// WithPasswordChange sets the repository ChangePassword writes with.
func WithPasswordChange(repo repository.PasswordChangeRepository) AuthOption {
	return func(uc *AuthUseCase) { uc.passwordChange = repo }
}

func NewAuthUseCase(
	users repository.UserRepository,
	refreshTokens repository.RefreshTokenRepository,
	tokenManager TokenManager,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
	rotationGracePeriod time.Duration,
	opts ...AuthOption,
) *AuthUseCase {
	if rotationGracePeriod < 0 {
		rotationGracePeriod = 15 * time.Second
	}
	uc := &AuthUseCase{
		users:               users,
		refreshTokens:       refreshTokens,
		tokenManager:        tokenManager,
		accessTokenTTL:      accessTokenTTL,
		refreshTokenTTL:     refreshTokenTTL,
		rotationGracePeriod: rotationGracePeriod,
	}
	for _, opt := range opts {
		opt(uc)
	}
	return uc
}

// Register creates a user with the default role and returns it as the auth
// response DTO (no password hash; created_at is RFC3339 to the second, or ""
// when the repository sets none).
func (uc *AuthUseCase) Register(
	ctx context.Context,
	email string,
	password string,
) (dto.AuthUserResponse, error) {
	// Checked first: with registration off, the input is not examined.
	if !uc.registrationAllowed {
		return dto.AuthUserResponse{}, ErrRegistrationDisabled
	}

	email = strings.ToLower(strings.TrimSpace(email))

	if !isValidEmail(email) {
		return dto.AuthUserResponse{}, ErrInvalidEmail
	}

	if len(password) < passwordMinLen || len(password) > passwordMaxLen {
		return dto.AuthUserResponse{}, ErrInvalidPassword
	}

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return dto.AuthUserResponse{}, err
	}

	user := &domain.User{
		Email:        email,
		PasswordHash: string(passwordHash),
		Role:         defaultUserRole,
	}

	if err := uc.users.Create(ctx, user); err != nil {
		return dto.AuthUserResponse{}, fromRepository(err)
	}

	user.PasswordHash = ""
	return mapper.ToAuthUserResponse(*user), nil
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
) (*dto.LoginResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	if !isValidEmail(email) || password == "" {
		return nil, ErrInvalidCredentials
	}

	user, err := uc.users.GetByEmail(ctx, email)
	if err = fromRepository(err); err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash),
		[]byte(password),
	); err != nil {
		return nil, ErrInvalidCredentials
	}

	if uc.accessTokenTTL <= 0 || uc.refreshTokenTTL <= 0 {
		return nil, ErrInvalidTokenTTL
	}

	// 1. Generate JWT Access Token
	accessToken, _, err := uc.tokenManager.GenerateAccessToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	// 2. Generate 32 bytes cryptographically secure random Refresh Token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	rawRefreshToken := base64.RawURLEncoding.EncodeToString(tokenBytes)

	// 3. Hash Refresh Token with SHA-256 before storing
	tokenHash := sha256.Sum256([]byte(rawRefreshToken))
	encodedHash := base64.RawURLEncoding.EncodeToString(tokenHash[:])
	refreshExpiry := time.Now().Add(uc.refreshTokenTTL)

	refreshTokenRecord := &domain.RefreshToken{
		UserID:    user.ID,
		TokenHash: encodedHash,
		ExpiresAt: refreshExpiry,
	}

	if err := uc.refreshTokens.Create(ctx, refreshTokenRecord); err != nil {
		return nil, fromRepository(err)
	}

	user.PasswordHash = ""

	return &dto.LoginResult{
		User:         mapper.ToAuthUserResponse(*user),
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
	}, nil
}

func (uc *AuthUseCase) handleReplacedToken(
	ctx context.Context,
	record *domain.RefreshToken,
	now time.Time,
) error {
	if record == nil {
		return ErrInvalidSession
	}

	if !IsRefreshTokenReplaced(record) {
		if IsRefreshTokenRevoked(record) {
			return ErrRefreshTokenRevoked
		}
		if IsRefreshTokenExpired(record, now) {
			return ErrRefreshTokenExpired
		}
		return ErrInvalidSession
	}

	// Check if this replaced token is within the configured grace period
	if IsRefreshTokenWithinGracePeriod(record, uc.rotationGracePeriod, now) {
		// Concurrent refresh within grace period: do NOT revoke token family!
		return ErrConcurrentRefresh
	}

	// Malicious reuse detected outside grace period: revoke entire token family for this user
	if record.UserID > 0 {
		if err := uc.refreshTokens.RevokeAllForUser(ctx, record.UserID, now); err != nil {
			return fmt.Errorf("failed to revoke all tokens for user %d upon token reuse: %w", record.UserID, fromRepository(err))
		}
	}

	return ErrRefreshTokenReused
}

func (uc *AuthUseCase) Refresh(
	ctx context.Context,
	rawRefreshToken string,
) (*dto.LoginResult, error) {
	if rawRefreshToken == "" {
		return nil, ErrInvalidSession
	}

	if uc.accessTokenTTL <= 0 || uc.refreshTokenTTL <= 0 {
		return nil, ErrInvalidTokenTTL
	}

	oldHash := sha256.Sum256([]byte(rawRefreshToken))
	encodedOldHash := base64.RawURLEncoding.EncodeToString(oldHash[:])

	// 1. Retrieve old token record to inspect status before rotation
	oldRecord, err := uc.refreshTokens.GetByTokenHash(ctx, encodedOldHash)
	if err = fromRepository(err); err != nil {
		if errors.Is(err, ErrRefreshTokenNotFound) {
			return nil, ErrInvalidSession
		}
		return nil, err
	}
	if oldRecord == nil {
		return nil, ErrInvalidSession
	}

	now := time.Now().UTC()

	// 2. Detect token reuse early
	if IsRefreshTokenReplaced(oldRecord) {
		return nil, uc.handleReplacedToken(ctx, oldRecord, now)
	}

	if IsRefreshTokenRevoked(oldRecord) {
		return nil, ErrRefreshTokenRevoked
	}

	if IsRefreshTokenExpired(oldRecord, now) {
		return nil, ErrRefreshTokenExpired
	}

	// 3. Verify user exists and fetch current role/email
	user, err := uc.users.GetByID(ctx, oldRecord.UserID)
	if err = fromRepository(err); err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidSession
		}
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidSession
	}
	user.PasswordHash = ""

	// 4. Pre-generate new Access Token
	newAccessToken, _, err := uc.tokenManager.GenerateAccessToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	// 5. Generate new random Refresh Token
	newTokenBytes := make([]byte, 32)
	if _, err := rand.Read(newTokenBytes); err != nil {
		return nil, err
	}
	newRawRefreshToken := base64.RawURLEncoding.EncodeToString(newTokenBytes)
	newHash := sha256.Sum256([]byte(newRawRefreshToken))
	encodedNewHash := base64.RawURLEncoding.EncodeToString(newHash[:])
	newRefreshExpiry := now.Add(uc.refreshTokenTTL)

	newRecord := &domain.RefreshToken{
		UserID:    user.ID,
		TokenHash: encodedNewHash,
		ExpiresAt: newRefreshExpiry,
		CreatedAt: now,
	}

	// 6. Execute atomic rotation in DB
	err = fromRepository(uc.refreshTokens.Rotate(ctx, encodedOldHash, newRecord))
	if err != nil {
		switch {
		case errors.Is(err, ErrRefreshTokenReused):
			// Fetch the latest token state from DB to evaluate grace period against when it was actually replaced
			latestRecord, getErr := uc.refreshTokens.GetByTokenHash(ctx, encodedOldHash)
			if getErr = fromRepository(getErr); getErr != nil {
				if errors.Is(getErr, ErrRefreshTokenNotFound) {
					return nil, ErrInvalidSession
				}
				return nil, getErr
			}
			if latestRecord == nil {
				return nil, ErrInvalidSession
			}
			return nil, uc.handleReplacedToken(ctx, latestRecord, time.Now().UTC())

		case errors.Is(err, ErrRefreshTokenRevoked):
			return nil, ErrRefreshTokenRevoked
		case errors.Is(err, ErrRefreshTokenExpired):
			return nil, ErrRefreshTokenExpired
		case errors.Is(err, ErrRefreshTokenNotFound):
			return nil, ErrInvalidSession
		default:
			return nil, err
		}
	}

	return &dto.LoginResult{
		User:         mapper.ToAuthUserResponse(*user),
		AccessToken:  newAccessToken,
		RefreshToken: newRawRefreshToken,
	}, nil
}

func (uc *AuthUseCase) Logout(
	ctx context.Context,
	rawRefreshToken string,
) error {
	if rawRefreshToken == "" {
		return ErrInvalidSession
	}

	tokenHash := sha256.Sum256([]byte(rawRefreshToken))
	encodedHash := base64.RawURLEncoding.EncodeToString(tokenHash[:])

	err := fromRepository(uc.refreshTokens.Revoke(ctx, encodedHash, time.Now()))
	if errors.Is(err, ErrRefreshTokenNotFound) {
		return ErrInvalidSession
	}
	return err
}

func (uc *AuthUseCase) Authenticate(
	ctx context.Context,
	accessToken string,
) (*domain.User, error) {
	if accessToken == "" {
		return nil, ErrInvalidSession
	}

	claims, err := uc.tokenManager.ValidateAccessToken(accessToken)
	if err != nil {
		return nil, ErrInvalidSession
	}

	return &domain.User{
		ID:    claims.UserID,
		Email: claims.Email,
		Role:  claims.Role,
	}, nil
}

// Me returns the /auth/me response for the authenticated principal set by
// AuthMiddleware. It does not read the database: the principal comes from the
// access token's claims and has no creation time, so created_at is "".
func (uc *AuthUseCase) Me(principal domain.User) dto.AuthUserResponse {
	return mapper.ToAuthUserResponse(principal)
}

// LogoutResponse is the data of every successful logout response,
// {"message":"logged out"}. Logout succeeds even when Logout is not called
// (no refresh cookie) or reports an already invalid token, so the payload
// does not depend on its result.
func (uc *AuthUseCase) LogoutResponse() dto.MessageResponse {
	return mapper.ToLogoutResponse()
}

// ChangePassword lets the authenticated user userID replace their password.
// The current password must match (ErrInvalidCurrentPassword); the new one
// follows the registration length rules (ErrInvalidPassword) and must differ
// from the current one (ErrPasswordUnchanged). On success the new hash is
// stored and every refresh token of the user is revoked in one repository
// operation. A user that no longer exists is ErrInvalidActor.
func (uc *AuthUseCase) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	if userID <= 0 {
		return ErrInvalidActor
	}

	user, err := uc.users.GetByID(ctx, userID)
	err = fromRepository(err)
	if errors.Is(err, ErrUserNotFound) {
		return ErrInvalidActor
	}
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return errors.New("user repository returned nil user without error")
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)) != nil {
		return ErrInvalidCurrentPassword
	}
	if len(newPassword) < passwordMinLen || len(newPassword) > passwordMaxLen {
		return ErrInvalidPassword
	}
	// The current password is verified, so comparing the strings is enough.
	if newPassword == currentPassword {
		return ErrPasswordUnchanged
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if uc.passwordChange == nil {
		return errors.New("password change repository is not configured")
	}
	err = fromRepository(uc.passwordChange.UpdateOwnPasswordAndRevokeTokens(ctx, userID, string(passwordHash), time.Now().UTC()))
	if errors.Is(err, ErrUserNotFound) {
		return ErrInvalidActor
	}
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	return nil
}
