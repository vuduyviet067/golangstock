package usecase

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

// Auth use case results as response DTOs (Mapper refactor Step 3.5).
// Auth timestamps are RFC3339 to the second in the value's own offset, and
// the zero time is ""; they are never the User format (RFC3339Nano). Expected
// values are written out by hand.

var (
	authCreatedAt = time.Date(2026, 3, 4, 5, 6, 7, 891234567, time.UTC)
	authICT       = time.FixedZone("ICT", 7*60*60)
	refreshShape  = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
)

// timestampedUserRepository behaves like PostgreSQL on Create: it assigns the
// ID and the creation time.
type timestampedUserRepository struct {
	fakeUserRepository
	createdAt time.Time
}

func (r *timestampedUserRepository) Create(ctx context.Context, u *domain.User) error {
	if err := r.fakeUserRepository.Create(ctx, u); err != nil {
		return err
	}
	u.ID = 42
	u.CreatedAt = r.createdAt
	return nil
}

func TestAuthRegister_DTO(t *testing.T) {
	cases := map[string]struct {
		createdAt time.Time
		want      dto.AuthUserResponse
	}{
		"UTC drops sub-seconds": {authCreatedAt,
			dto.AuthUserResponse{ID: 42, Email: "new@example.com", Role: "user", CreatedAt: "2026-03-04T05:06:07Z"}},
		"offset kept": {authCreatedAt.In(authICT),
			dto.AuthUserResponse{ID: 42, Email: "new@example.com", Role: "user", CreatedAt: "2026-03-04T12:06:07+07:00"}},
		"zero time is empty": {time.Time{},
			dto.AuthUserResponse{ID: 42, Email: "new@example.com", Role: "user", CreatedAt: ""}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &timestampedUserRepository{createdAt: tc.createdAt}
			got, err := newTestAuthUseCase(repo, &fakeRefreshTokenRepository{}, nil).Register(context.Background(), " NEW@example.com ", "Password123")
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("Register = %+v, want %+v", got, tc.want)
			}
			assertNoUserSecrets(t, got, "Password123", repo.created.PasswordHash)
		})
	}

	dbErr := errors.New("connection reset")
	for name, tc := range map[string]struct {
		email, password string
		createErr       error
		wantErr         error
	}{
		"malformed email":  {"not-an-email", "Password123", nil, ErrInvalidEmail},
		"short password":   {"a@example.com", "short", nil, ErrInvalidPassword},
		"duplicate email":  {"a@example.com", "Password123", repository.ErrEmailExists, ErrEmailExists},
		"repository error": {"a@example.com", "Password123", dbErr, dbErr},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeUserRepository{createErr: tc.createErr}
			got, err := newTestAuthUseCase(repo, &fakeRefreshTokenRepository{}, nil).Register(context.Background(), tc.email, tc.password)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if got != (dto.AuthUserResponse{}) {
				t.Errorf("Register on error = %+v, want the zero DTO", got)
			}
		})
	}
}

func TestAuthLogin_DTO(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("Password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	stored := func() *domain.User {
		return &domain.User{ID: 7, Email: "admin@example.com", PasswordHash: string(hash), Role: "admin",
			CreatedAt: authCreatedAt.In(authICT), UpdatedAt: authCreatedAt}
	}

	t.Run("body user and cookie tokens are separate", func(t *testing.T) {
		tokens := &fakeRefreshTokenRepository{}
		uc := newTestAuthUseCase(&fakeUserRepository{user: stored()}, tokens, &fakeTokenManager{genToken: "jwt-access"})

		got, err := uc.Login(context.Background(), " ADMIN@example.com ", "Password123")
		if err != nil {
			t.Fatal(err)
		}
		want := dto.AuthUserResponse{ID: 7, Email: "admin@example.com", Role: "admin", CreatedAt: "2026-03-04T12:06:07+07:00"}
		if got.User != want {
			t.Errorf("User = %+v, want %+v", got.User, want)
		}
		if got.AccessToken != "jwt-access" || !refreshShape.MatchString(got.RefreshToken) {
			t.Errorf("tokens = %q / %q, want the access JWT and a 43-char refresh token", got.AccessToken, got.RefreshToken)
		}
		if tokens.created == nil || tokens.created.TokenHash == got.RefreshToken || tokens.created.UserID != 7 {
			t.Errorf("stored refresh token = %+v, want the hash of the raw token for user 7", tokens.created)
		}
		assertNoUserSecrets(t, got.User, string(hash), "Password123", got.AccessToken, got.RefreshToken)
	})

	dbErr := errors.New("connection reset")
	for name, tc := range map[string]struct {
		users           *fakeUserRepository
		email, password string
		wantErr         error
	}{
		"wrong password":   {&fakeUserRepository{user: stored()}, "admin@example.com", "WrongPassword", ErrInvalidCredentials},
		"unknown email":    {&fakeUserRepository{}, "nobody@example.com", "Password123", ErrInvalidCredentials},
		"malformed email":  {&fakeUserRepository{user: stored()}, "admin", "Password123", ErrInvalidCredentials},
		"empty password":   {&fakeUserRepository{user: stored()}, "admin@example.com", "", ErrInvalidCredentials},
		"repository error": {&fakeUserRepository{getByEmailErr: dbErr}, "admin@example.com", "Password123", dbErr},
	} {
		t.Run(name, func(t *testing.T) {
			tokens := &fakeRefreshTokenRepository{}
			got, err := newTestAuthUseCase(tc.users, tokens, nil).Login(context.Background(), tc.email, tc.password)
			if !errors.Is(err, tc.wantErr) || got != nil {
				t.Fatalf("Login = %+v, %v; want nil, %v", got, err, tc.wantErr)
			}
			if tokens.createCall != 0 {
				t.Error("no refresh token may be stored on a failed login")
			}
		})
	}
}

func TestAuthRefresh_DTO(t *testing.T) {
	now := time.Now().UTC()
	valid := func() *domain.RefreshToken {
		return &domain.RefreshToken{ID: 1, UserID: 9, ExpiresAt: now.Add(time.Hour)}
	}
	user := &domain.User{ID: 9, Email: "refresh@example.com", PasswordHash: "$2a$10$refreshSecret", Role: "user", CreatedAt: authCreatedAt}

	t.Run("rotated tokens and body user", func(t *testing.T) {
		tokens := &fakeRefreshTokenRepository{token: valid()}
		uc := newTestAuthUseCase(&fakeUserRepository{user: user}, tokens, &fakeTokenManager{genToken: "jwt-rotated"})

		got, err := uc.Refresh(context.Background(), "old-raw-token")
		if err != nil {
			t.Fatal(err)
		}
		want := dto.AuthUserResponse{ID: 9, Email: "refresh@example.com", Role: "user", CreatedAt: "2026-03-04T05:06:07Z"}
		if got.User != want {
			t.Errorf("User = %+v, want %+v", got.User, want)
		}
		if got.AccessToken != "jwt-rotated" || !refreshShape.MatchString(got.RefreshToken) || got.RefreshToken == "old-raw-token" {
			t.Errorf("tokens = %q / %q, want a new access JWT and a new refresh token", got.AccessToken, got.RefreshToken)
		}
		if tokens.rotateCall != 1 || tokens.rotatedNew == nil || tokens.rotatedNew.TokenHash == got.RefreshToken {
			t.Errorf("rotation = %d call(s), new record %+v; want one rotation storing the hash", tokens.rotateCall, tokens.rotatedNew)
		}
		assertNoUserSecrets(t, got.User, "$2a$10$refreshSecret", got.AccessToken, got.RefreshToken, "old-raw-token")
	})

	replaced := "replaced-hash"
	justRotated := now.Add(-2 * time.Second)
	longAgo := now.Add(-time.Hour)
	dbErr := errors.New("connection reset")
	for name, tc := range map[string]struct {
		token   *domain.RefreshToken
		getErr  error
		users   *fakeUserRepository
		wantErr error
	}{
		"unknown token":         {nil, nil, &fakeUserRepository{user: user}, ErrInvalidSession},
		"expired token":         {&domain.RefreshToken{ID: 1, UserID: 9, ExpiresAt: now.Add(-time.Hour)}, nil, &fakeUserRepository{user: user}, ErrRefreshTokenExpired},
		"revoked token":         {&domain.RefreshToken{ID: 1, UserID: 9, ExpiresAt: now.Add(time.Hour), RevokedAt: &longAgo}, nil, &fakeUserRepository{user: user}, ErrRefreshTokenRevoked},
		"concurrent refresh":    {&domain.RefreshToken{ID: 1, UserID: 9, ExpiresAt: now.Add(time.Hour), RevokedAt: &justRotated, ReplacedByHash: &replaced}, nil, &fakeUserRepository{user: user}, ErrConcurrentRefresh},
		"reuse after grace":     {&domain.RefreshToken{ID: 1, UserID: 9, ExpiresAt: now.Add(time.Hour), RevokedAt: &longAgo, ReplacedByHash: &replaced}, nil, &fakeUserRepository{user: user}, ErrRefreshTokenReused},
		"deleted user":          {valid(), nil, &fakeUserRepository{}, ErrInvalidSession},
		"token repository down": {nil, dbErr, &fakeUserRepository{user: user}, dbErr},
	} {
		t.Run(name, func(t *testing.T) {
			tokens := &fakeRefreshTokenRepository{token: tc.token, getErr: tc.getErr}
			got, err := newTestAuthUseCase(tc.users, tokens, nil).Refresh(context.Background(), "raw-token")
			if !errors.Is(err, tc.wantErr) || got != nil {
				t.Fatalf("Refresh = %+v, %v; want nil, %v", got, err, tc.wantErr)
			}
		})
	}
}

// Me maps the principal built from the access token's claims. It reads no
// repository (the use case here has none), so created_at is "".
func TestAuthMe_DTO(t *testing.T) {
	uc := NewAuthUseCase(nil, nil, &fakeTokenManager{
		validClaims: &dto.TokenClaims{UserID: 5, Email: "me@example.com", Role: "admin"},
	}, 15*time.Minute, 7*24*time.Hour, 15*time.Second)

	principal, err := uc.Authenticate(context.Background(), "access-token")
	if err != nil {
		t.Fatal(err)
	}
	got := uc.Me(*principal)

	want := dto.AuthUserResponse{ID: 5, Email: "me@example.com", Role: "admin", CreatedAt: ""}
	if got != want {
		t.Errorf("Me = %+v, want %+v", got, want)
	}
	assertUserJSON(t, got, `{"id":5,"email":"me@example.com","role":"admin","created_at":""}`)

	// The Auth timestamp rule applies if a principal ever carries a time.
	withTime := uc.Me(domain.User{ID: 6, Email: "t@example.com", Role: "user", PasswordHash: "$2a$10$meSecret", CreatedAt: authCreatedAt.In(authICT)})
	assertUserJSON(t, withTime, `{"id":6,"email":"t@example.com","role":"user","created_at":"2026-03-04T12:06:07+07:00"}`)
	assertNoUserSecrets(t, withTime, "$2a$10$meSecret")
}

func TestAuthLogoutResponse(t *testing.T) {
	uc := NewAuthUseCase(nil, nil, &fakeTokenManager{}, 15*time.Minute, 7*24*time.Hour, 15*time.Second)

	got := uc.LogoutResponse()

	if got != (dto.MessageResponse{Message: "logged out"}) {
		t.Fatalf("LogoutResponse = %+v, want {Message:logged out}", got)
	}
	assertUserJSON(t, got, `{"message":"logged out"}`)
	assertUserJSON(t, dto.APIResponse{Success: true, Message: "logged out", Data: got},
		`{"success":true,"message":"logged out","data":{"message":"logged out"}}`)
}
