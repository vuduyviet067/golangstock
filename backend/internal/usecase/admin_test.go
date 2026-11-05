package usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

// adminUserRepoFake is a repository.UserRepository for AdminUseCase tests. It
// records lookups and keeps a copy of the user exactly as Create received it,
// including the password hash.
type adminUserRepoFake struct {
	// getByEmail controls GetByEmail; nil means "not found".
	getByEmail  func(email string) (*domain.User, error)
	createErr   error
	lookedUp    []string
	createCalls int
	created     *domain.User
}

func (f *adminUserRepoFake) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	f.lookedUp = append(f.lookedUp, email)
	if f.getByEmail == nil {
		return nil, domain.ErrUserNotFound
	}
	return f.getByEmail(email)
}

func (f *adminUserRepoFake) GetByID(context.Context, int64) (*domain.User, error) {
	return nil, domain.ErrUserNotFound
}

func (f *adminUserRepoFake) Create(_ context.Context, user *domain.User) error {
	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}
	stored := *user
	f.created = &stored
	user.ID = 42
	return nil
}

var _ repository.UserRepository = (*adminUserRepoFake)(nil)

const adminTestPassword = "Adm1n-Passw0rd"

func TestAdminUseCase_CreateAdmin_Success(t *testing.T) {
	repo := &adminUserRepoFake{}
	uc := NewAdminUseCase(repo)

	admin, err := uc.CreateAdmin(context.Background(), "  Admin@Example.COM \n", adminTestPassword)
	if err != nil {
		t.Fatalf("CreateAdmin: unexpected error: %v", err)
	}

	// Email is normalized before the lookup and before storage.
	if !slices.Equal(repo.lookedUp, []string{"admin@example.com"}) {
		t.Errorf("GetByEmail calls = %q, want [admin@example.com]", repo.lookedUp)
	}
	if repo.createCalls != 1 || repo.created == nil {
		t.Fatalf("Create calls = %d, want 1", repo.createCalls)
	}
	if repo.created.Email != "admin@example.com" {
		t.Errorf("stored email = %q, want admin@example.com", repo.created.Email)
	}

	// Role is exactly "admin".
	if repo.created.Role != "admin" {
		t.Errorf("stored role = %q, want exactly %q", repo.created.Role, "admin")
	}

	// The repository receives a real bcrypt hash of the password, never the password itself.
	if repo.created.PasswordHash == "" || repo.created.PasswordHash == adminTestPassword {
		t.Fatalf("stored PasswordHash = %q, want a bcrypt hash", repo.created.PasswordHash)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.created.PasswordHash), []byte(adminTestPassword)); err != nil {
		t.Errorf("stored hash does not match the password: %v", err)
	}

	// The caller gets the created user without the hash.
	if admin.PasswordHash != "" {
		t.Errorf("returned PasswordHash = %q, want empty", admin.PasswordHash)
	}
	if admin.ID != 42 || admin.Email != "admin@example.com" || admin.Role != "admin" {
		t.Errorf("returned user = %+v, want ID 42, admin@example.com, role admin", *admin)
	}
}

func TestAdminUseCase_CreateAdmin_InvalidEmail(t *testing.T) {
	for _, email := range []string{
		"",
		"   ",
		"no-at-sign.example.com",
		"@example.com",
		"admin@",
		"admin@localhost",
		"ad min@example.com",
		strings.Repeat("a", 250) + "@example.com", // over 255 characters
	} {
		t.Run(email, func(t *testing.T) {
			repo := &adminUserRepoFake{}
			_, err := NewAdminUseCase(repo).CreateAdmin(context.Background(), email, adminTestPassword)

			if !errors.Is(err, ErrInvalidEmail) {
				t.Fatalf("error = %v, want ErrInvalidEmail", err)
			}
			if len(repo.lookedUp) != 0 || repo.createCalls != 0 {
				t.Errorf("repository must not be called for invalid input (lookups %d, creates %d)", len(repo.lookedUp), repo.createCalls)
			}
		})
	}
}

func TestAdminUseCase_CreateAdmin_PasswordLength(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"empty", "", true},
		{"7 bytes", strings.Repeat("p", 7), true},
		{"8 bytes (minimum)", strings.Repeat("p", 8), false},
		{"72 bytes (bcrypt maximum)", strings.Repeat("p", 72), false},
		{"73 bytes", strings.Repeat("p", 73), true},
		// Limits are in bytes, matching bcrypt: 37 two-byte characters are 74 bytes.
		{"37 multibyte characters (74 bytes)", strings.Repeat("é", 37), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &adminUserRepoFake{}
			_, err := NewAdminUseCase(repo).CreateAdmin(context.Background(), "admin@example.com", tc.password)

			if !tc.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidPassword) {
				t.Fatalf("error = %v, want ErrInvalidPassword", err)
			}
			if len(repo.lookedUp) != 0 || repo.createCalls != 0 {
				t.Errorf("repository must not be called for invalid input (lookups %d, creates %d)", len(repo.lookedUp), repo.createCalls)
			}
		})
	}
}

func TestAdminUseCase_CreateAdmin_ExistingEmailRejected(t *testing.T) {
	repo := &adminUserRepoFake{
		getByEmail: func(email string) (*domain.User, error) {
			return &domain.User{ID: 7, Email: email, Role: "user"}, nil
		},
	}

	admin, err := NewAdminUseCase(repo).CreateAdmin(context.Background(), "Taken@Example.com", adminTestPassword)

	if !errors.Is(err, domain.ErrEmailExists) {
		t.Fatalf("error = %v, want domain.ErrEmailExists", err)
	}
	if admin != nil {
		t.Errorf("returned user = %+v, want nil", *admin)
	}
	if repo.createCalls != 0 {
		t.Errorf("Create must not be called when the email exists, got %d calls", repo.createCalls)
	}
}

func TestAdminUseCase_CreateAdmin_GetByEmailErrorPropagated(t *testing.T) {
	dbDown := errors.New("connection refused")
	repo := &adminUserRepoFake{
		getByEmail: func(string) (*domain.User, error) { return nil, dbDown },
	}

	admin, err := NewAdminUseCase(repo).CreateAdmin(context.Background(), "admin@example.com", adminTestPassword)

	if !errors.Is(err, dbDown) {
		t.Fatalf("error = %v, want it to wrap the repository error", err)
	}
	if errors.Is(err, domain.ErrEmailExists) {
		t.Error("an infrastructure error must not be reported as a duplicate email")
	}
	if admin != nil {
		t.Errorf("returned user = %+v, want nil", *admin)
	}
	if repo.createCalls != 0 {
		t.Errorf("Create must not be called after a failed lookup, got %d calls", repo.createCalls)
	}
}

func TestAdminUseCase_CreateAdmin_CreateErrorPropagated(t *testing.T) {
	insertFailed := errors.New("insert failed")
	repo := &adminUserRepoFake{createErr: insertFailed}

	admin, err := NewAdminUseCase(repo).CreateAdmin(context.Background(), "admin@example.com", adminTestPassword)

	if !errors.Is(err, insertFailed) {
		t.Fatalf("error = %v, want it to wrap the repository error", err)
	}
	if admin != nil {
		t.Errorf("returned user = %+v, want nil", *admin)
	}
}

// TestAdminUseCase_CreateAdmin_ConcurrentDuplicate covers the race where the
// pre-check finds no user but another process inserts the same email before
// Create; the unique constraint makes Create return ErrEmailExists.
func TestAdminUseCase_CreateAdmin_ConcurrentDuplicate(t *testing.T) {
	repo := &adminUserRepoFake{createErr: domain.ErrEmailExists}

	admin, err := NewAdminUseCase(repo).CreateAdmin(context.Background(), "admin@example.com", adminTestPassword)

	if !errors.Is(err, domain.ErrEmailExists) {
		t.Fatalf("error = %v, want domain.ErrEmailExists", err)
	}
	if admin != nil {
		t.Errorf("returned user = %+v, want nil", *admin)
	}
	if repo.createCalls != 1 {
		t.Errorf("Create calls = %d, want 1", repo.createCalls)
	}
}

// TestAdminUseCase_CreateAdmin_GetByEmailNilNil checks that a repository
// returning (nil, nil), which violates its contract, does not cause a panic.
// The email is treated as available and Create (with the database's unique
// constraint) remains the final duplicate guard.
func TestAdminUseCase_CreateAdmin_GetByEmailNilNil(t *testing.T) {
	repo := &adminUserRepoFake{
		getByEmail: func(string) (*domain.User, error) { return nil, nil },
	}

	admin, err := NewAdminUseCase(repo).CreateAdmin(context.Background(), "admin@example.com", adminTestPassword)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.createCalls != 1 {
		t.Errorf("Create calls = %d, want 1", repo.createCalls)
	}
	if admin == nil || admin.Role != "admin" || admin.PasswordHash != "" {
		t.Errorf("returned user = %+v, want an admin without a password hash", admin)
	}
}
