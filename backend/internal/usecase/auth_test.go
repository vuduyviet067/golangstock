package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

type fakeUserRepository struct {
	mu            sync.Mutex
	user          *domain.User
	createErr     error
	created       *domain.User
	createCall    int
	getByIDErr    error
	getByEmailErr error
}

func (f *fakeUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getByEmailErr != nil {
		return nil, f.getByEmailErr
	}
	if f.user == nil {
		return nil, domain.ErrUserNotFound
	}
	copy := *f.user
	return &copy, nil
}

func (f *fakeUserRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	if f.user == nil {
		return nil, domain.ErrUserNotFound
	}
	copy := *f.user
	return &copy, nil
}

func (f *fakeUserRepository) Create(ctx context.Context, user *domain.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCall++
	if f.createErr != nil {
		return f.createErr
	}
	copy := *user
	f.created = &copy
	user.ID = 1
	return nil
}

var _ repository.UserRepository = (*fakeUserRepository)(nil)

type fakeRefreshTokenRepository struct {
	mu         sync.Mutex
	createErr  error
	created    *domain.RefreshToken
	createCall int

	token       *domain.RefreshToken
	getErr      error
	getCall     int
	gotHash     string
	getByHashFn func(ctx context.Context, tokenHash string, callCount int) (*domain.RefreshToken, error)

	rotateErr  error
	rotateCall int
	rotatedOld string
	rotatedNew *domain.RefreshToken

	revokeErr   error
	revokeCall  int
	revokedHash string

	revokeAllErr     error
	revokeAllCall    int
	revokedAllUserID int64
}

func (f *fakeRefreshTokenRepository) Create(ctx context.Context, token *domain.RefreshToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCall++
	if f.createErr != nil {
		return f.createErr
	}
	copy := *token
	f.created = &copy
	token.ID = 1
	return nil
}

func (f *fakeRefreshTokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCall++
	f.gotHash = tokenHash
	if f.getByHashFn != nil {
		return f.getByHashFn(ctx, tokenHash, f.getCall)
	}
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.token == nil {
		return nil, domain.ErrRefreshTokenNotFound
	}
	copy := *f.token
	return &copy, nil
}

func (f *fakeRefreshTokenRepository) Rotate(ctx context.Context, oldHash string, newToken *domain.RefreshToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rotateCall++
	f.rotatedOld = oldHash
	f.rotatedNew = newToken
	if f.rotateErr != nil {
		return f.rotateErr
	}
	newToken.ID = 2
	if f.token != nil {
		newToken.UserID = f.token.UserID
	}
	return nil
}

func (f *fakeRefreshTokenRepository) Revoke(ctx context.Context, tokenHash string, revokedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revokeCall++
	f.revokedHash = tokenHash
	if f.revokeErr != nil {
		return f.revokeErr
	}
	return nil
}

func (f *fakeRefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID int64, revokedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revokeAllCall++
	f.revokedAllUserID = userID
	if f.revokeAllErr != nil {
		return f.revokeAllErr
	}
	return nil
}

type fakeTokenManager struct {
	genToken  string
	genExpiry time.Time
	genErr    error

	validClaims *TokenClaims
	validErr    error
}

func (f *fakeTokenManager) GenerateAccessToken(userID int64, email, role string) (string, time.Time, error) {
	if f.genErr != nil {
		return "", time.Time{}, f.genErr
	}
	expiry := f.genExpiry
	if expiry.IsZero() {
		expiry = time.Now().Add(15 * time.Minute)
	}
	token := f.genToken
	if token == "" {
		token = "mock-jwt-access-token"
	}
	return token, expiry, nil
}

func (f *fakeTokenManager) ValidateAccessToken(tokenStr string) (*TokenClaims, error) {
	if f.validErr != nil {
		return nil, f.validErr
	}
	if f.validClaims != nil {
		copy := *f.validClaims
		return &copy, nil
	}
	return &TokenClaims{
		UserID: 1,
		Email:  "viet@example.com",
		Role:   "user",
	}, nil
}

func newTestAuthUseCase(
	users repository.UserRepository,
	tokens repository.RefreshTokenRepository,
	tm TokenManager,
) *AuthUseCase {
	if tm == nil {
		tm = &fakeTokenManager{}
	}
	return NewAuthUseCase(users, tokens, tm, 15*time.Minute, 7*24*time.Hour, 15*time.Second)
}

func TestRegisterSuccess(t *testing.T) {
	users := &fakeUserRepository{}
	uc := newTestAuthUseCase(users, &fakeRefreshTokenRepository{}, nil)

	user, err := uc.Register(
		context.Background(),
		"  Viet@example.com ",
		"password123",
	)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if user.Email != "viet@example.com" {
		t.Errorf("Email = %q, want %q", user.Email, "viet@example.com")
	}
	if user.Role != "user" {
		t.Errorf("Role = %q, want %q", user.Role, "user")
	}
	if user.PasswordHash != "" {
		t.Error("Register() must not return password hash")
	}
	if users.created == nil {
		t.Fatal("expected repository Create to be called")
	}
	if users.created.PasswordHash == "password123" {
		t.Fatal("password must not be stored in plain text")
	}
	if users.createCall != 1 {
		t.Errorf("Create called %d times, want 1", users.createCall)
	}
}

func TestRegisterInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		password string
		wantErr  error
	}{
		{"invalid email", "invalid", "password123", ErrInvalidEmail},
		{"short password", "viet@example.com", "123", ErrInvalidPassword},
		{"empty password", "viet@example.com", "", ErrInvalidPassword},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := &fakeUserRepository{}
			uc := newTestAuthUseCase(users, &fakeRefreshTokenRepository{}, nil)

			_, err := uc.Register(context.Background(), tt.email, tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Register() error = %v, want %v", err, tt.wantErr)
			}
			if users.createCall != 0 {
				t.Error("repository must not be called for invalid input")
			}
		})
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	users := &fakeUserRepository{
		createErr: domain.ErrEmailExists,
	}
	uc := newTestAuthUseCase(users, &fakeRefreshTokenRepository{}, nil)

	_, err := uc.Register(
		context.Background(),
		"viet@example.com",
		"password123",
	)
	if !errors.Is(err, domain.ErrEmailExists) {
		t.Fatalf("Register() error = %v, want %v", err, domain.ErrEmailExists)
	}
}

func TestLoginSuccess(t *testing.T) {
	password := "StrongPass123"
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	users := &fakeUserRepository{
		user: &domain.User{
			ID:           1,
			Email:        "viet@example.com",
			PasswordHash: string(passwordHash),
			Role:         "user",
		},
	}
	tokens := &fakeRefreshTokenRepository{}
	tm := &fakeTokenManager{genToken: "jwt-test-token"}

	uc := newTestAuthUseCase(users, tokens, tm)

	result, err := uc.Login(context.Background(), " VIET@example.com ", password)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	if result == nil {
		t.Fatal("Login() returned nil result")
	}
	if result.User.ID != 1 {
		t.Errorf("User.ID = %d, want 1", result.User.ID)
	}
	if result.User.PasswordHash != "" {
		t.Error("returned user must not expose password hash")
	}
	if result.AccessToken != "jwt-test-token" {
		t.Errorf("AccessToken = %q, want 'jwt-test-token'", result.AccessToken)
	}
	if result.RefreshToken == "" {
		t.Fatal("expected non-empty RefreshToken")
	}

	// Verify RefreshToken was hashed before storing in DB
	if tokens.createCall != 1 {
		t.Fatalf("tokens.Create called %d times, want 1", tokens.createCall)
	}
	if tokens.created == nil {
		t.Fatal("no refresh token created in DB")
	}
	if tokens.created.TokenHash == result.RefreshToken {
		t.Fatal("refresh token must be hashed, not stored as plaintext")
	}
	if tokens.created.UserID != 1 {
		t.Errorf("UserID = %d, want 1", tokens.created.UserID)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("StrongPass123"), bcrypt.DefaultCost)
	users := &fakeUserRepository{
		user: &domain.User{
			ID:           1,
			Email:        "viet@example.com",
			PasswordHash: string(passwordHash),
			Role:         "user",
		},
	}
	tokens := &fakeRefreshTokenRepository{}
	uc := newTestAuthUseCase(users, tokens, nil)

	result, err := uc.Login(context.Background(), "viet@example.com", "WrongPassword123")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
	if result != nil {
		t.Error("result must be nil on failed login")
	}
	if tokens.createCall != 0 {
		t.Errorf("tokens.Create called %d times, want 0", tokens.createCall)
	}
}

func TestLoginUserNotFound(t *testing.T) {
	users := &fakeUserRepository{}
	tokens := &fakeRefreshTokenRepository{}
	uc := newTestAuthUseCase(users, tokens, nil)

	result, err := uc.Login(context.Background(), "unknown@example.com", "StrongPass123")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
	if result != nil {
		t.Error("result must be nil for unknown user")
	}
}

func TestRefreshSuccess(t *testing.T) {
	users := &fakeUserRepository{
		user: &domain.User{ID: 10, Email: "user@example.com", Role: "user"},
	}
	tokens := &fakeRefreshTokenRepository{
		token: &domain.RefreshToken{ID: 1, UserID: 10, ExpiresAt: time.Now().Add(time.Hour)},
	}
	tm := &fakeTokenManager{genToken: "new-access-jwt"}

	uc := newTestAuthUseCase(users, tokens, tm)

	res, err := uc.Refresh(context.Background(), "valid-raw-refresh-token")
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil LoginResult")
	}
	if res.AccessToken != "new-access-jwt" {
		t.Errorf("AccessToken = %q, want 'new-access-jwt'", res.AccessToken)
	}
	if res.RefreshToken == "" || res.RefreshToken == "valid-raw-refresh-token" {
		t.Errorf("expected rotated new refresh token, got %q", res.RefreshToken)
	}
	if tokens.rotateCall != 1 {
		t.Errorf("tokens.Rotate called %d times, want 1", tokens.rotateCall)
	}
}

func TestRefreshExpiredToken(t *testing.T) {
	tokens := &fakeRefreshTokenRepository{
		token: &domain.RefreshToken{ID: 1, UserID: 10, ExpiresAt: time.Now().Add(-time.Hour)},
	}
	uc := newTestAuthUseCase(&fakeUserRepository{}, tokens, nil)

	_, err := uc.Refresh(context.Background(), "expired-token")
	if !errors.Is(err, ErrRefreshTokenExpired) {
		t.Fatalf("Refresh() error = %v, want ErrRefreshTokenExpired", err)
	}
}

func TestRefreshRevokedToken(t *testing.T) {
	now := time.Now()
	tokens := &fakeRefreshTokenRepository{
		token: &domain.RefreshToken{ID: 1, UserID: 10, ExpiresAt: time.Now().Add(time.Hour), RevokedAt: &now},
	}
	uc := newTestAuthUseCase(&fakeUserRepository{}, tokens, nil)

	_, err := uc.Refresh(context.Background(), "revoked-token")
	if !errors.Is(err, ErrRefreshTokenRevoked) {
		t.Fatalf("Refresh() error = %v, want ErrRefreshTokenRevoked", err)
	}
}

func TestRefreshReusedToken_TriggersFamilyRevocation(t *testing.T) {
	replaced := "earlier-replacement-hash"
	tokens := &fakeRefreshTokenRepository{
		token: &domain.RefreshToken{ID: 5, UserID: 99, ExpiresAt: time.Now().Add(time.Hour), ReplacedByHash: &replaced},
	}
	uc := newTestAuthUseCase(&fakeUserRepository{}, tokens, nil)

	_, err := uc.Refresh(context.Background(), "stolen-reused-token")
	if !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("Refresh() error = %v, want ErrRefreshTokenReused", err)
	}

	// Verify that Token Reuse Detection revoked all tokens for User ID 99
	if tokens.revokeAllCall != 1 {
		t.Fatalf("RevokeAllForUser called %d times, want 1", tokens.revokeAllCall)
	}
	if tokens.revokedAllUserID != 99 {
		t.Errorf("RevokeAllForUser called for UserID %d, want 99", tokens.revokedAllUserID)
	}
}

func TestRefreshEmptyToken(t *testing.T) {
	uc := newTestAuthUseCase(&fakeUserRepository{}, &fakeRefreshTokenRepository{}, nil)

	_, err := uc.Refresh(context.Background(), "")
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("Refresh() error = %v, want ErrInvalidSession", err)
	}
}

func TestLogoutSuccess(t *testing.T) {
	tokens := &fakeRefreshTokenRepository{}
	uc := newTestAuthUseCase(&fakeUserRepository{}, tokens, nil)

	err := uc.Logout(context.Background(), "raw-refresh-token")
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if tokens.revokeCall != 1 {
		t.Errorf("tokens.Revoke called %d times, want 1", tokens.revokeCall)
	}
	if tokens.revokedHash == "" || tokens.revokedHash == "raw-refresh-token" {
		t.Error("Logout must revoke by token hash, not raw token")
	}
}

func TestLogoutInvalidOrEmpty(t *testing.T) {
	tokens := &fakeRefreshTokenRepository{
		revokeErr: domain.ErrRefreshTokenNotFound,
	}
	uc := newTestAuthUseCase(&fakeUserRepository{}, tokens, nil)

	// Empty token
	err := uc.Logout(context.Background(), "")
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("Logout('') error = %v, want ErrInvalidSession", err)
	}

	// Token not found
	err = uc.Logout(context.Background(), "unknown-token")
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("Logout(unknown) error = %v, want ErrInvalidSession", err)
	}
}

func TestAuthenticateStateless(t *testing.T) {
	t.Run("empty access token", func(t *testing.T) {
		uc := newTestAuthUseCase(&fakeUserRepository{}, &fakeRefreshTokenRepository{}, nil)
		user, err := uc.Authenticate(context.Background(), "")
		if !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("error = %v, want ErrInvalidSession", err)
		}
		if user != nil {
			t.Fatal("user must be nil")
		}
	})

	t.Run("invalid or expired JWT", func(t *testing.T) {
		tm := &fakeTokenManager{validErr: errors.New("jwt expired or invalid")}
		uc := newTestAuthUseCase(&fakeUserRepository{}, &fakeRefreshTokenRepository{}, tm)

		user, err := uc.Authenticate(context.Background(), "bad-jwt")
		if !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("error = %v, want ErrInvalidSession", err)
		}
		if user != nil {
			t.Fatal("user must be nil")
		}
	})

	t.Run("valid JWT without database query", func(t *testing.T) {
		users := &fakeUserRepository{
			getByIDErr: errors.New("should NOT be called!"),
		}
		tokens := &fakeRefreshTokenRepository{
			getErr: errors.New("should NOT be called!"),
		}
		tm := &fakeTokenManager{
			validClaims: &TokenClaims{
				UserID: 77,
				Email:  "principal@example.com",
				Role:   "admin",
			},
		}

		uc := newTestAuthUseCase(users, tokens, tm)

		user, err := uc.Authenticate(context.Background(), "valid-jwt")
		if err != nil {
			t.Fatalf("Authenticate() unexpected error: %v", err)
		}
		if user == nil {
			t.Fatal("expected non-nil user")
		}
		if user.ID != 77 {
			t.Errorf("ID = %d, want 77", user.ID)
		}
		if user.Email != "principal@example.com" {
			t.Errorf("Email = %q, want principal@example.com", user.Email)
		}
		if user.Role != "admin" {
			t.Errorf("Role = %q, want admin", user.Role)
		}
		if users.createCall != 0 {
			t.Error("users repo should not be touched")
		}
	})
}

func TestRefresh_ConcurrentRefresh_WithinGracePeriod(t *testing.T) {
	replaced := "new-hash-123"
	rotatedAt := time.Now().Add(-2 * time.Second)
	tokens := &fakeRefreshTokenRepository{
		token: &domain.RefreshToken{
			ID:             1,
			UserID:         10,
			ExpiresAt:      time.Now().Add(24 * time.Hour),
			ReplacedByHash: &replaced,
			RevokedAt:      &rotatedAt,
		},
	}
	uc := NewAuthUseCase(&fakeUserRepository{}, tokens, &fakeTokenManager{}, 15*time.Minute, 7*24*time.Hour, 15*time.Second)

	_, err := uc.Refresh(context.Background(), "concurrent-refresh-token")
	if !errors.Is(err, ErrConcurrentRefresh) {
		t.Fatalf("Refresh() error = %v, want ErrConcurrentRefresh", err)
	}

	// Must NOT call RevokeAllForUser within grace period!
	if tokens.revokeAllCall != 0 {
		t.Errorf("RevokeAllForUser called %d times within grace period, want 0", tokens.revokeAllCall)
	}
}

func TestRefresh_MaliciousReuse_OutsideGracePeriod(t *testing.T) {
	replaced := "new-hash-123"
	rotatedAt := time.Now().Add(-30 * time.Second)
	tokens := &fakeRefreshTokenRepository{
		token: &domain.RefreshToken{
			ID:             1,
			UserID:         10,
			ExpiresAt:      time.Now().Add(24 * time.Hour),
			ReplacedByHash: &replaced,
			RevokedAt:      &rotatedAt,
		},
	}
	uc := NewAuthUseCase(&fakeUserRepository{}, tokens, &fakeTokenManager{}, 15*time.Minute, 7*24*time.Hour, 15*time.Second)

	_, err := uc.Refresh(context.Background(), "stolen-old-token")
	if !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("Refresh() error = %v, want ErrRefreshTokenReused", err)
	}

	// Outside grace period: MUST call RevokeAllForUser!
	if tokens.revokeAllCall != 1 {
		t.Fatalf("RevokeAllForUser called %d times, want 1", tokens.revokeAllCall)
	}
	if tokens.revokedAllUserID != 10 {
		t.Errorf("revokedAllUserID = %d, want 10", tokens.revokedAllUserID)
	}
}

func TestRefresh_MaliciousReuse_RevokeAllFailureNotSwallowed(t *testing.T) {
	replaced := "new-hash-123"
	rotatedAt := time.Now().Add(-30 * time.Second)
	dbErr := errors.New("db connection failure")
	tokens := &fakeRefreshTokenRepository{
		token: &domain.RefreshToken{
			ID:             1,
			UserID:         10,
			ExpiresAt:      time.Now().Add(24 * time.Hour),
			ReplacedByHash: &replaced,
			RevokedAt:      &rotatedAt,
		},
		revokeAllErr: dbErr,
	}
	uc := NewAuthUseCase(&fakeUserRepository{}, tokens, &fakeTokenManager{}, 15*time.Minute, 7*24*time.Hour, 15*time.Second)

	_, err := uc.Refresh(context.Background(), "stolen-old-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, dbErr) && !strings.Contains(err.Error(), "db connection failure") {
		t.Fatalf("expected db connection failure to not be swallowed, got %v", err)
	}
}

func TestRefresh_LogoutRevokedNotClassifiedAsReuse(t *testing.T) {
	now := time.Now()
	tokens := &fakeRefreshTokenRepository{
		token: &domain.RefreshToken{
			ID:             1,
			UserID:         10,
			ExpiresAt:      time.Now().Add(24 * time.Hour),
			RevokedAt:      &now,
			ReplacedByHash: nil, // Only revoked by logout, NOT replaced by rotation
		},
	}
	uc := newTestAuthUseCase(&fakeUserRepository{}, tokens, nil)

	_, err := uc.Refresh(context.Background(), "logged-out-token")
	if !errors.Is(err, ErrRefreshTokenRevoked) {
		t.Fatalf("Refresh() error = %v, want ErrRefreshTokenRevoked", err)
	}
	if errors.Is(err, ErrRefreshTokenReused) {
		t.Error("logged out token must not be classified as reused")
	}
	if tokens.revokeAllCall != 0 {
		t.Errorf("RevokeAllForUser called %d times, want 0", tokens.revokeAllCall)
	}
}

func TestRefresh_NilTokenFromRepositoryDoesNotPanic(t *testing.T) {
	tokens := &fakeRefreshTokenRepository{
		getErr: nil,
		token:  nil, // nil token returned with nil err
	}
	uc := newTestAuthUseCase(&fakeUserRepository{}, tokens, nil)

	_, err := uc.Refresh(context.Background(), "any-token")
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("Refresh() error = %v, want ErrInvalidSession", err)
	}
}

func TestRefresh_InvalidTTLHandledClearly(t *testing.T) {
	tokens := &fakeRefreshTokenRepository{
		token: &domain.RefreshToken{ID: 1, UserID: 10, ExpiresAt: time.Now().Add(time.Hour)},
	}
	// Invalid accessTokenTTL
	uc := NewAuthUseCase(&fakeUserRepository{}, tokens, &fakeTokenManager{}, 0, 7*24*time.Hour, 15*time.Second)
	_, err := uc.Refresh(context.Background(), "valid-token")
	if !errors.Is(err, ErrInvalidTokenTTL) {
		t.Fatalf("Refresh() with 0 accessTokenTTL error = %v, want ErrInvalidTokenTTL", err)
	}

	// Invalid refreshTokenTTL
	uc2 := NewAuthUseCase(&fakeUserRepository{}, tokens, &fakeTokenManager{}, 15*time.Minute, -time.Hour, 15*time.Second)
	_, err2 := uc2.Refresh(context.Background(), "valid-token")
	if !errors.Is(err2, ErrInvalidTokenTTL) {
		t.Fatalf("Refresh() with negative refreshTokenTTL error = %v, want ErrInvalidTokenTTL", err2)
	}
}

func TestRefresh_RotateReused_StaleFallbackFix(t *testing.T) {
	now := time.Now().UTC()
	activeToken := &domain.RefreshToken{
		ID:        1,
		UserID:    10,
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now.Add(-time.Hour),
	}
	user := &domain.User{
		ID:    10,
		Email: "user@example.com",
		Role:  "user",
	}

	t.Run("GetByTokenHash returns NotFound on second fetch", func(t *testing.T) {
		users := &fakeUserRepository{user: user}
		tokens := &fakeRefreshTokenRepository{
			rotateErr: domain.ErrRefreshTokenReused,
			getByHashFn: func(ctx context.Context, tokenHash string, callCount int) (*domain.RefreshToken, error) {
				if callCount == 1 {
					copy := *activeToken
					return &copy, nil
				}
				return nil, domain.ErrRefreshTokenNotFound
			},
		}
		uc := newTestAuthUseCase(users, tokens, &fakeTokenManager{})

		_, err := uc.Refresh(context.Background(), "raw-token")
		if !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("Refresh() error = %v, want ErrInvalidSession", err)
		}
		if tokens.getCall != 2 {
			t.Errorf("GetByTokenHash called %d times, want 2", tokens.getCall)
		}
		if tokens.rotateCall != 1 {
			t.Errorf("Rotate called %d times, want 1", tokens.rotateCall)
		}
		if tokens.revokeAllCall != 0 {
			t.Errorf("RevokeAllForUser called %d times, want 0", tokens.revokeAllCall)
		}
	})

	t.Run("GetByTokenHash returns DB error on second fetch", func(t *testing.T) {
		dbErr := errors.New("infrastructure DB failure")
		users := &fakeUserRepository{user: user}
		tokens := &fakeRefreshTokenRepository{
			rotateErr: domain.ErrRefreshTokenReused,
			getByHashFn: func(ctx context.Context, tokenHash string, callCount int) (*domain.RefreshToken, error) {
				if callCount == 1 {
					copy := *activeToken
					return &copy, nil
				}
				return nil, dbErr
			},
		}
		uc := newTestAuthUseCase(users, tokens, &fakeTokenManager{})

		_, err := uc.Refresh(context.Background(), "raw-token")
		if !errors.Is(err, dbErr) {
			t.Fatalf("Refresh() error = %v, want %v", err, dbErr)
		}
		if tokens.getCall != 2 {
			t.Errorf("GetByTokenHash called %d times, want 2", tokens.getCall)
		}
		if tokens.rotateCall != 1 {
			t.Errorf("Rotate called %d times, want 1", tokens.rotateCall)
		}
		// Stale record fallback MUST NOT happen! RevokeAllForUser must not be called!
		if tokens.revokeAllCall != 0 {
			t.Errorf("RevokeAllForUser called %d times on DB error, want 0", tokens.revokeAllCall)
		}
	})

	t.Run("GetByTokenHash returns nil token on second fetch", func(t *testing.T) {
		users := &fakeUserRepository{user: user}
		tokens := &fakeRefreshTokenRepository{
			rotateErr: domain.ErrRefreshTokenReused,
			getByHashFn: func(ctx context.Context, tokenHash string, callCount int) (*domain.RefreshToken, error) {
				if callCount == 1 {
					copy := *activeToken
					return &copy, nil
				}
				return nil, nil
			},
		}
		uc := newTestAuthUseCase(users, tokens, &fakeTokenManager{})

		_, err := uc.Refresh(context.Background(), "raw-token")
		if !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("Refresh() error = %v, want ErrInvalidSession", err)
		}
		if tokens.getCall != 2 {
			t.Errorf("GetByTokenHash called %d times, want 2", tokens.getCall)
		}
		if tokens.rotateCall != 1 {
			t.Errorf("Rotate called %d times, want 1", tokens.rotateCall)
		}
		if tokens.revokeAllCall != 0 {
			t.Errorf("RevokeAllForUser called %d times, want 0", tokens.revokeAllCall)
		}
	})

	t.Run("GetByTokenHash returns latest record within grace period", func(t *testing.T) {
		replacedHash := "replacement-hash-123"
		replacedAt := time.Now().UTC().Add(-2 * time.Second)
		latestRecord := &domain.RefreshToken{
			ID:             1,
			UserID:         10,
			ExpiresAt:      now.Add(24 * time.Hour),
			CreatedAt:      now.Add(-time.Hour),
			RevokedAt:      &replacedAt,
			ReplacedByHash: &replacedHash,
		}

		users := &fakeUserRepository{user: user}
		tokens := &fakeRefreshTokenRepository{
			rotateErr: domain.ErrRefreshTokenReused,
			getByHashFn: func(ctx context.Context, tokenHash string, callCount int) (*domain.RefreshToken, error) {
				if callCount == 1 {
					copy := *activeToken
					return &copy, nil
				}
				return latestRecord, nil
			},
		}
		uc := NewAuthUseCase(users, tokens, &fakeTokenManager{}, 15*time.Minute, 7*24*time.Hour, 15*time.Second)

		_, err := uc.Refresh(context.Background(), "raw-token")
		if !errors.Is(err, ErrConcurrentRefresh) {
			t.Fatalf("Refresh() error = %v, want ErrConcurrentRefresh", err)
		}
		if tokens.getCall != 2 {
			t.Errorf("GetByTokenHash called %d times, want 2", tokens.getCall)
		}
		if tokens.rotateCall != 1 {
			t.Errorf("Rotate called %d times, want 1", tokens.rotateCall)
		}
		if tokens.revokeAllCall != 0 {
			t.Errorf("RevokeAllForUser called %d times within grace period, want 0", tokens.revokeAllCall)
		}
	})

	t.Run("GetByTokenHash returns latest record outside grace period", func(t *testing.T) {
		replacedHash := "replacement-hash-123"
		replacedAt := time.Now().UTC().Add(-30 * time.Second)
		latestRecord := &domain.RefreshToken{
			ID:             1,
			UserID:         88,
			ExpiresAt:      now.Add(24 * time.Hour),
			CreatedAt:      now.Add(-time.Hour),
			RevokedAt:      &replacedAt,
			ReplacedByHash: &replacedHash,
		}

		users := &fakeUserRepository{user: user}
		tokens := &fakeRefreshTokenRepository{
			rotateErr: domain.ErrRefreshTokenReused,
			getByHashFn: func(ctx context.Context, tokenHash string, callCount int) (*domain.RefreshToken, error) {
				if callCount == 1 {
					copy := *activeToken
					return &copy, nil
				}
				return latestRecord, nil
			},
		}
		uc := NewAuthUseCase(users, tokens, &fakeTokenManager{}, 15*time.Minute, 7*24*time.Hour, 15*time.Second)

		_, err := uc.Refresh(context.Background(), "raw-token")
		if !errors.Is(err, ErrRefreshTokenReused) {
			t.Fatalf("Refresh() error = %v, want ErrRefreshTokenReused", err)
		}
		if tokens.getCall != 2 {
			t.Errorf("GetByTokenHash called %d times, want 2", tokens.getCall)
		}
		if tokens.rotateCall != 1 {
			t.Errorf("Rotate called %d times, want 1", tokens.rotateCall)
		}
		if tokens.revokeAllCall != 1 {
			t.Fatalf("RevokeAllForUser called %d times, want 1", tokens.revokeAllCall)
		}
		if tokens.revokedAllUserID != 88 {
			t.Errorf("RevokeAllForUser called for UserID %d, want 88", tokens.revokedAllUserID)
		}
	})

	t.Run("GetByTokenHash returns latest record that is unreplaced", func(t *testing.T) {
		latestRecord := &domain.RefreshToken{
			ID:             1,
			UserID:         10,
			ExpiresAt:      now.Add(24 * time.Hour),
			CreatedAt:      now.Add(-time.Hour),
			ReplacedByHash: nil,
		}

		users := &fakeUserRepository{user: user}
		tokens := &fakeRefreshTokenRepository{
			rotateErr: domain.ErrRefreshTokenReused,
			getByHashFn: func(ctx context.Context, tokenHash string, callCount int) (*domain.RefreshToken, error) {
				if callCount == 1 {
					copy := *activeToken
					return &copy, nil
				}
				return latestRecord, nil
			},
		}
		uc := NewAuthUseCase(users, tokens, &fakeTokenManager{}, 15*time.Minute, 7*24*time.Hour, 15*time.Second)

		_, err := uc.Refresh(context.Background(), "raw-token")
		if !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("Refresh() error = %v, want ErrInvalidSession", err)
		}
		if tokens.getCall != 2 {
			t.Errorf("GetByTokenHash called %d times, want 2", tokens.getCall)
		}
		if tokens.rotateCall != 1 {
			t.Errorf("Rotate called %d times, want 1", tokens.rotateCall)
		}
		if tokens.revokeAllCall != 0 {
			t.Errorf("RevokeAllForUser called %d times, want 0", tokens.revokeAllCall)
		}
	})
}

// CR-T01: Concurrent refresh in grace period
func TestRefresh_CR_T01_ConcurrentRefresh_WithinGracePeriod(t *testing.T) {
	now := time.Now().UTC()
	activeToken := &domain.RefreshToken{
		ID:        1,
		UserID:    42,
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now.Add(-10 * time.Minute),
	}
	user := &domain.User{
		ID:    42,
		Email: "concurrent@example.com",
		Role:  "user",
	}

	replacedHash := "replaced-by-request-b"
	replacedAt := now.Add(-2 * time.Second) // 2s elapsed <= 15s grace period
	tokenAfterRequestB := &domain.RefreshToken{
		ID:             1,
		UserID:         42,
		ExpiresAt:      now.Add(24 * time.Hour),
		CreatedAt:      now.Add(-10 * time.Minute),
		RevokedAt:      &replacedAt,
		ReplacedByHash: &replacedHash,
	}

	// Request A execution:
	// 1. Initial read gets valid activeToken
	// 2. Concurrently Request B rotates it
	// 3. Request A Rotate receives ErrRefreshTokenReused
	// 4. Request A re-reads token via GetByTokenHash, getting tokenAfterRequestB
	users := &fakeUserRepository{user: user}
	tokens := &fakeRefreshTokenRepository{
		rotateErr: domain.ErrRefreshTokenReused,
		getByHashFn: func(ctx context.Context, tokenHash string, callCount int) (*domain.RefreshToken, error) {
			if callCount == 1 {
				copy := *activeToken
				return &copy, nil
			}
			return tokenAfterRequestB, nil
		},
	}
	uc := NewAuthUseCase(users, tokens, &fakeTokenManager{}, 15*time.Minute, 7*24*time.Hour, 15*time.Second)

	res, err := uc.Refresh(context.Background(), "raw-refresh-token")
	if res != nil {
		t.Fatal("expected nil LoginResult for concurrent refresh rejection")
	}
	if !errors.Is(err, ErrConcurrentRefresh) {
		t.Fatalf("Refresh() error = %v, want ErrConcurrentRefresh", err)
	}

	// Repository verification:
	if tokens.getCall != 2 {
		t.Errorf("GetByTokenHash called %d times, want 2 (initial read + fallback read)", tokens.getCall)
	}
	if tokens.rotateCall != 1 {
		t.Errorf("Rotate called %d times, want 1", tokens.rotateCall)
	}
	// Crucial: family tokens MUST NOT be revoked!
	if tokens.revokeAllCall != 0 {
		t.Errorf("RevokeAllForUser called %d times within grace period, want 0", tokens.revokeAllCall)
	}
}

// CR-T02: Stale read fallback outside grace period where RevokeAllForUser fails
func TestRefresh_CR_T02_ReusedToken_OutsideGracePeriod_RevokeAllFailureNotSwallowed(t *testing.T) {
	now := time.Now().UTC()
	activeToken := &domain.RefreshToken{
		ID:        1,
		UserID:    55,
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now.Add(-time.Hour),
	}
	user := &domain.User{
		ID:    55,
		Email: "victim@example.com",
		Role:  "user",
	}

	replacedHash := "replaced-long-ago"
	replacedAt := now.Add(-30 * time.Second) // 30s elapsed > 15s grace period
	tokenReplacedOutsideGrace := &domain.RefreshToken{
		ID:             1,
		UserID:         55,
		ExpiresAt:      now.Add(24 * time.Hour),
		CreatedAt:      now.Add(-time.Hour),
		RevokedAt:      &replacedAt,
		ReplacedByHash: &replacedHash,
	}

	dbErr := errors.New("database connection refused during token revocation")
	users := &fakeUserRepository{user: user}
	tokens := &fakeRefreshTokenRepository{
		rotateErr: domain.ErrRefreshTokenReused,
		getByHashFn: func(ctx context.Context, tokenHash string, callCount int) (*domain.RefreshToken, error) {
			if callCount == 1 {
				copy := *activeToken
				return &copy, nil
			}
			return tokenReplacedOutsideGrace, nil
		},
		revokeAllErr: dbErr,
	}
	uc := NewAuthUseCase(users, tokens, &fakeTokenManager{}, 15*time.Minute, 7*24*time.Hour, 15*time.Second)

	_, err := uc.Refresh(context.Background(), "stolen-raw-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, dbErr) && !strings.Contains(err.Error(), "database connection refused") {
		t.Fatalf("expected dbErr to be propagated, got %v", err)
	}
	if tokens.getCall != 2 {
		t.Errorf("GetByTokenHash called %d times, want 2", tokens.getCall)
	}
	if tokens.rotateCall != 1 {
		t.Errorf("Rotate called %d times, want 1", tokens.rotateCall)
	}
	if tokens.revokeAllCall != 1 {
		t.Fatalf("RevokeAllForUser called %d times, want 1", tokens.revokeAllCall)
	}
	if tokens.revokedAllUserID != 55 {
		t.Errorf("revokedAllUserID = %d, want 55", tokens.revokedAllUserID)
	}
}

// CR-T04: Refresh when user does not exist or was deleted
func TestRefresh_CR_T04_UserNotFoundOrDeleted(t *testing.T) {
	now := time.Now().UTC()
	activeToken := &domain.RefreshToken{
		ID:        1,
		UserID:    999, // User that does not exist in users table
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now.Add(-time.Hour),
	}

	t.Run("user deleted/not found returns ErrInvalidSession without rotation", func(t *testing.T) {
		users := &fakeUserRepository{
			getByIDErr: domain.ErrUserNotFound,
		}
		tokens := &fakeRefreshTokenRepository{
			token: activeToken,
		}
		uc := newTestAuthUseCase(users, tokens, &fakeTokenManager{})

		res, err := uc.Refresh(context.Background(), "valid-token-for-deleted-user")
		if res != nil {
			t.Fatal("expected nil result")
		}
		if !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("Refresh() error = %v, want ErrInvalidSession", err)
		}
		if tokens.rotateCall != 0 {
			t.Errorf("tokens.Rotate called %d times, want 0", tokens.rotateCall)
		}
		if tokens.createCall != 0 {
			t.Errorf("tokens.Create called %d times, want 0", tokens.createCall)
		}
	})

	t.Run("user repo returns nil user without error returns ErrInvalidSession", func(t *testing.T) {
		users := &fakeUserRepository{
			user: nil, // nil user
		}
		tokens := &fakeRefreshTokenRepository{
			token: activeToken,
		}
		uc := newTestAuthUseCase(users, tokens, &fakeTokenManager{})

		res, err := uc.Refresh(context.Background(), "valid-token-for-deleted-user")
		if res != nil {
			t.Fatal("expected nil result")
		}
		if !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("Refresh() error = %v, want ErrInvalidSession", err)
		}
		if tokens.rotateCall != 0 {
			t.Errorf("tokens.Rotate called %d times, want 0", tokens.rotateCall)
		}
	})

	t.Run("user repo returns database error propagates error without rotation", func(t *testing.T) {
		userDBErr := errors.New("users table disk I/O error")
		users := &fakeUserRepository{
			getByIDErr: userDBErr,
		}
		tokens := &fakeRefreshTokenRepository{
			token: activeToken,
		}
		uc := newTestAuthUseCase(users, tokens, &fakeTokenManager{})

		res, err := uc.Refresh(context.Background(), "valid-token-for-deleted-user")
		if res != nil {
			t.Fatal("expected nil result")
		}
		if !errors.Is(err, userDBErr) {
			t.Fatalf("Refresh() error = %v, want %v", err, userDBErr)
		}
		if tokens.rotateCall != 0 {
			t.Errorf("tokens.Rotate called %d times, want 0", tokens.rotateCall)
		}
	})
}

// CR-T05: UseCase mappings for repository Rotate errors
func TestRefresh_CR_T05_RotateErrorsMapped(t *testing.T) {
	now := time.Now().UTC()
	activeToken := &domain.RefreshToken{
		ID:        1,
		UserID:    10,
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now.Add(-time.Hour),
	}
	user := &domain.User{
		ID:    10,
		Email: "user@example.com",
		Role:  "user",
	}

	t.Run("Rotate returns ErrRefreshTokenRevoked", func(t *testing.T) {
		users := &fakeUserRepository{user: user}
		tokens := &fakeRefreshTokenRepository{
			token:     activeToken,
			rotateErr: domain.ErrRefreshTokenRevoked,
		}
		uc := newTestAuthUseCase(users, tokens, &fakeTokenManager{})

		_, err := uc.Refresh(context.Background(), "raw-token")
		if !errors.Is(err, ErrRefreshTokenRevoked) {
			t.Fatalf("Refresh() error = %v, want ErrRefreshTokenRevoked", err)
		}
	})

	t.Run("Rotate returns ErrRefreshTokenExpired", func(t *testing.T) {
		users := &fakeUserRepository{user: user}
		tokens := &fakeRefreshTokenRepository{
			token:     activeToken,
			rotateErr: domain.ErrRefreshTokenExpired,
		}
		uc := newTestAuthUseCase(users, tokens, &fakeTokenManager{})

		_, err := uc.Refresh(context.Background(), "raw-token")
		if !errors.Is(err, ErrRefreshTokenExpired) {
			t.Fatalf("Refresh() error = %v, want ErrRefreshTokenExpired", err)
		}
	})

	t.Run("Rotate returns ErrRefreshTokenNotFound", func(t *testing.T) {
		users := &fakeUserRepository{user: user}
		tokens := &fakeRefreshTokenRepository{
			token:     activeToken,
			rotateErr: domain.ErrRefreshTokenNotFound,
		}
		uc := newTestAuthUseCase(users, tokens, &fakeTokenManager{})

		_, err := uc.Refresh(context.Background(), "raw-token")
		if !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("Refresh() error = %v, want ErrInvalidSession", err)
		}
	})

	t.Run("Rotate returns generic database error", func(t *testing.T) {
		genericDBErr := errors.New("deadlock detected or postgres connection broken")
		users := &fakeUserRepository{user: user}
		tokens := &fakeRefreshTokenRepository{
			token:     activeToken,
			rotateErr: genericDBErr,
		}
		uc := newTestAuthUseCase(users, tokens, &fakeTokenManager{})

		_, err := uc.Refresh(context.Background(), "raw-token")
		if !errors.Is(err, genericDBErr) {
			t.Fatalf("Refresh() error = %v, want genericDBErr", err)
		}
	})
}
