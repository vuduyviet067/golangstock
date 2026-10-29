package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"gostock/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

type fakeSessionRepository struct {
	createErr  error
	created    *domain.Session
	createCall int

	session *domain.Session
	getErr  error
	getCall int
	gotHash string

	deleteErr   error
	deleteCall  int
	deletedHash string
}
type fakeUserRepository struct {
	user       *domain.User
	createErr  error
	created    *domain.User
	createCall int
}

func (f *fakeUserRepository) Create(
	ctx context.Context,
	user *domain.User,
) error {
	f.createCall++

	if f.createErr != nil {
		return f.createErr
	}

	copy := *user
	f.created = &copy
	user.ID = 1
	return nil
}
func (f *fakeSessionRepository) Create(
	ctx context.Context,
	session *domain.Session,
) error {
	f.createCall++

	if f.createErr != nil {
		return f.createErr
	}

	copy := *session
	f.created = &copy
	return nil
}

func (f *fakeUserRepository) GetByEmail(
	ctx context.Context,
	email string,
) (*domain.User, error) {
	if f.user == nil {
		return nil, domain.ErrUserNotFound
	}
	return f.user, nil
}

func (f *fakeUserRepository) GetByID(
	ctx context.Context,
	id int64,
) (*domain.User, error) {
	if f.user == nil {
		return nil, domain.ErrUserNotFound
	}
	return f.user, nil
}

func (f *fakeSessionRepository) GetByTokenHash(
	ctx context.Context,
	tokenHash string,
) (*domain.Session, error) {
	f.getCall++
	f.gotHash = tokenHash

	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.session == nil {
		return nil, errors.New("session not found")
	}

	copy := *f.session
	return &copy, nil
}

func (f *fakeSessionRepository) DeleteByTokenHash(
	ctx context.Context,
	tokenHash string,
) error {
	f.deleteCall++
	f.deletedHash = tokenHash

	if f.deleteErr != nil {
		return f.deleteErr
	}

	if f.session != nil && f.session.TokenHash == tokenHash {
		f.session = nil
	}

	return nil
}

func (f *fakeSessionRepository) DeleteExpired(
	ctx context.Context,
	now time.Time,
) error {
	return nil
}

func TestRegisterSuccess(t *testing.T) {
	users := &fakeUserRepository{}
	uc := NewAuthUseCase(users, &fakeSessionRepository{}, 24*time.Hour)

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
			uc := NewAuthUseCase(users, &fakeSessionRepository{}, 24*time.Hour)

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
	uc := NewAuthUseCase(users, &fakeSessionRepository{}, 24*time.Hour)

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

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
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

	sessions := &fakeSessionRepository{}

	uc := NewAuthUseCase(
		users,
		sessions,
		24*time.Hour,
	)

	result, err := uc.Login(
		context.Background(),
		"  VIET@example.com ",
		password,
	)
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

	if result.Token == "" {
		t.Fatal("Login() returned empty token")
	}

	if sessions.createCall != 1 {
		t.Errorf(
			"session Create() called %d times, want 1",
			sessions.createCall,
		)
	}

	if sessions.created == nil {
		t.Fatal("Login() did not persist a session")
	}

	if sessions.created.UserID != result.User.ID {
		t.Errorf(
			"session UserID = %d, want %d",
			sessions.created.UserID,
			result.User.ID,
		)
	}

	if sessions.created.TokenHash == result.Token {
		t.Error("session must store token hash, not raw token")
	}

	if sessions.created.ExpiresAt.IsZero() {
		t.Error("session expiry must be set")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte("StrongPass123"),
		bcrypt.DefaultCost,
	)
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

	sessions := &fakeSessionRepository{}
	uc := NewAuthUseCase(users, sessions, 24*time.Hour)

	result, err := uc.Login(
		context.Background(),
		"viet@example.com",
		"WrongPassword123",
	)

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}

	if result != nil {
		t.Error("Login() result must be nil for wrong password")
	}

	if sessions.createCall != 0 {
		t.Errorf(
			"session Create() called %d times, want 0",
			sessions.createCall,
		)
	}
}

func TestLoginUserNotFound(t *testing.T) {
	users := &fakeUserRepository{}
	sessions := &fakeSessionRepository{}

	uc := NewAuthUseCase(
		users,
		sessions,
		24*time.Hour,
	)

	result, err := uc.Login(
		context.Background(),
		"unknown@example.com",
		"StrongPass123",
	)

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf(
			"Login() error = %v, want ErrInvalidCredentials",
			err,
		)
	}

	if result != nil {
		t.Error("Login() result must be nil for unknown email")
	}

	if sessions.createCall != 0 {
		t.Errorf(
			"session Create() called %d times, want 0",
			sessions.createCall,
		)
	}
}

func TestLoginSessionCreateError(t *testing.T) {
	password := "StrongPass123"

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
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

	dbErr := errors.New("database unavailable")
	sessions := &fakeSessionRepository{
		createErr: dbErr,
	}

	uc := NewAuthUseCase(users, sessions, 24*time.Hour)

	result, err := uc.Login(
		context.Background(),
		"viet@example.com",
		password,
	)

	if !errors.Is(err, dbErr) {
		t.Fatalf("Login() error = %v, want %v", err, dbErr)
	}

	if result != nil {
		t.Error("Login() result must be nil when session creation fails")
	}

	if sessions.createCall != 1 {
		t.Errorf(
			"session Create() called %d times, want 1",
			sessions.createCall,
		)
	}
}
func TestAuthenticateEmptyToken(t *testing.T) {
	uc := NewAuthUseCase(
		&fakeUserRepository{},
		&fakeSessionRepository{},
		24*time.Hour,
	)

	user, err := uc.Authenticate(context.Background(), "")

	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("error = %v, want ErrInvalidSession", err)
	}
	if user != nil {
		t.Fatal("user must be nil")
	}
}

func TestAuthenticateExpiredSession(t *testing.T) {
	users := &fakeUserRepository{
		user: &domain.User{ID: 1, Email: "viet@example.com"},
	}
	sessions := &fakeSessionRepository{
		session: &domain.Session{
			UserID:    1,
			ExpiresAt: time.Now().Add(-time.Minute),
		},
	}
	uc := NewAuthUseCase(users, sessions, 24*time.Hour)

	user, err := uc.Authenticate(context.Background(), "test-token")

	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("error = %v, want ErrInvalidSession", err)
	}
	if user != nil {
		t.Fatal("user must be nil")
	}
}

func TestAuthenticateValidSession(t *testing.T) {
	users := &fakeUserRepository{
		user: &domain.User{
			ID:           1,
			Email:        "viet@example.com",
			PasswordHash: "secret-hash",
			Role:         "user",
		},
	}
	sessions := &fakeSessionRepository{
		session: &domain.Session{
			UserID:    1,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	uc := NewAuthUseCase(users, sessions, 24*time.Hour)

	user, err := uc.Authenticate(context.Background(), "test-token")

	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if user == nil {
		t.Fatal("expected user")
	}
	if user.PasswordHash != "" {
		t.Error("password hash must not be returned")
	}
}
func TestLogoutRevokesSession(t *testing.T) {
	sessions := &fakeSessionRepository{}
	uc := NewAuthUseCase(
		&fakeUserRepository{},
		sessions,
		24*time.Hour,
	)

	// Login để tạo token và session thật
	password := "StrongPass123"
	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
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
	uc = NewAuthUseCase(users, sessions, 24*time.Hour)

	result, err := uc.Login(
		context.Background(),
		"viet@example.com",
		password,
	)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	sessions.session = sessions.created

	// Token phải xác thực được trước khi logout
	if _, err := uc.Authenticate(context.Background(), result.Token); err != nil {
		t.Fatalf("Authenticate() before logout error = %v", err)
	}

	if err := uc.Logout(context.Background(), result.Token); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if sessions.deleteCall != 1 {
		t.Fatalf("DeleteByTokenHash() called %d times, want 1", sessions.deleteCall)
	}

	if sessions.deletedHash == "" || sessions.deletedHash == result.Token {
		t.Fatal("Logout() must delete by token hash, not raw token")
	}

	if sessions.session != nil {
		t.Fatal("session still exists after logout")
	}

	// Token cũ phải bị từ chối sau logout
	if _, err := uc.Authenticate(context.Background(), result.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("Authenticate() after logout error = %v, want %v", err, ErrInvalidSession)
	}
}
