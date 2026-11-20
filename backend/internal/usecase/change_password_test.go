package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gostock/internal/domain"
	"gostock/internal/repository"
)

// fakePasswordChangeRepo records UpdateOwnPasswordAndRevokeTokens calls.
type fakePasswordChangeRepo struct {
	err       error
	calls     int
	userID    int64
	hash      string
	revokedAt time.Time
}

func (f *fakePasswordChangeRepo) UpdateOwnPasswordAndRevokeTokens(_ context.Context, userID int64, hash string, revokedAt time.Time) error {
	f.calls++
	f.userID, f.hash, f.revokedAt = userID, hash, revokedAt
	return f.err
}

const (
	changeUserID    int64 = 7
	changeCurrentPW       = "current-password"
	changeNewPW           = "brand-new-password"
)

func newChangePasswordUseCase(t *testing.T, users *fakeUserRepository, repo repository.PasswordChangeRepository) *AuthUseCase {
	t.Helper()
	opts := []AuthOption{}
	if repo != nil {
		opts = append(opts, WithPasswordChange(repo))
	}
	return NewAuthUseCase(users, &fakeRefreshTokenRepository{}, &fakeTokenManager{},
		15*time.Minute, 7*24*time.Hour, 15*time.Second, opts...)
}

func userWithPassword(t *testing.T, password string) *fakeUserRepository {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeUserRepository{user: &domain.User{ID: changeUserID, Email: "member@example.com", Role: RoleUser, PasswordHash: string(hash)}}
}

func TestChangePassword_Success(t *testing.T) {
	repo := &fakePasswordChangeRepo{}
	uc := newChangePasswordUseCase(t, userWithPassword(t, changeCurrentPW), repo)
	before := time.Now().UTC()

	if err := uc.ChangePassword(context.Background(), changeUserID, changeCurrentPW, changeNewPW); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if repo.calls != 1 || repo.userID != changeUserID {
		t.Fatalf("repository calls %d for user %d, want 1 for %d", repo.calls, repo.userID, changeUserID)
	}
	if bcrypt.CompareHashAndPassword([]byte(repo.hash), []byte(changeNewPW)) != nil {
		t.Error("the stored hash is not a hash of the new password")
	}
	if repo.revokedAt.Before(before) || repo.revokedAt.Location() != time.UTC {
		t.Errorf("revokedAt = %v, want now in UTC", repo.revokedAt)
	}
}

func TestChangePassword_Rejected(t *testing.T) {
	tests := []struct {
		name             string
		userID           int64
		current, newPass string
		users            func(t *testing.T) *fakeUserRepository
		wantErr          error
	}{
		{"wrong current password", changeUserID, "not-the-password", changeNewPW, nil, ErrInvalidCurrentPassword},
		{"empty current password", changeUserID, "", changeNewPW, nil, ErrInvalidCurrentPassword},
		{"wrong current password is reported before a short new one", changeUserID, "not-the-password", "short", nil, ErrInvalidCurrentPassword},
		{"new password of 7 bytes", changeUserID, changeCurrentPW, "1234567", nil, ErrInvalidPassword},
		{"new password of 73 bytes", changeUserID, changeCurrentPW, strings.Repeat("p", 73), nil, ErrInvalidPassword},
		{"new password equal to the current one", changeUserID, changeCurrentPW, changeCurrentPW, nil, ErrPasswordUnchanged},
		{"user no longer exists", changeUserID, changeCurrentPW, changeNewPW,
			func(*testing.T) *fakeUserRepository { return &fakeUserRepository{} }, ErrInvalidActor},
		{"no user ID", 0, changeCurrentPW, changeNewPW, nil, ErrInvalidActor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := userWithPassword(t, changeCurrentPW)
			if tt.users != nil {
				users = tt.users(t)
			}
			repo := &fakePasswordChangeRepo{}
			err := newChangePasswordUseCase(t, users, repo).ChangePassword(context.Background(), tt.userID, tt.current, tt.newPass)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if repo.calls != 0 {
				t.Errorf("repository called %d time(s) for a rejected change", repo.calls)
			}
		})
	}
}

func TestChangePassword_LengthBoundaries(t *testing.T) {
	for _, n := range []int{8, 72} {
		repo := &fakePasswordChangeRepo{}
		err := newChangePasswordUseCase(t, userWithPassword(t, changeCurrentPW), repo).
			ChangePassword(context.Background(), changeUserID, changeCurrentPW, strings.Repeat("n", n))
		if err != nil || repo.calls != 1 {
			t.Errorf("%d bytes: err %v, calls %d; want success", n, err, repo.calls)
		}
	}
}

func TestChangePassword_RepositoryErrors(t *testing.T) {
	dbErr := errors.New("connection reset")

	t.Run("GetByID fails", func(t *testing.T) {
		users := userWithPassword(t, changeCurrentPW)
		users.getByIDErr = dbErr
		repo := &fakePasswordChangeRepo{}
		err := newChangePasswordUseCase(t, users, repo).ChangePassword(context.Background(), changeUserID, changeCurrentPW, changeNewPW)
		if !errors.Is(err, dbErr) || repo.calls != 0 {
			t.Errorf("err %v, calls %d; want the database error and no update", err, repo.calls)
		}
	})

	t.Run("update fails", func(t *testing.T) {
		repo := &fakePasswordChangeRepo{err: dbErr}
		err := newChangePasswordUseCase(t, userWithPassword(t, changeCurrentPW), repo).
			ChangePassword(context.Background(), changeUserID, changeCurrentPW, changeNewPW)
		if !errors.Is(err, dbErr) {
			t.Errorf("err = %v, want the database error", err)
		}
		for _, notWant := range []error{ErrInvalidCurrentPassword, ErrPasswordUnchanged, ErrInvalidPassword, ErrInvalidActor} {
			if errors.Is(err, notWant) {
				t.Errorf("err = %v must not be %v", err, notWant)
			}
		}
	})

	t.Run("user deleted before the update", func(t *testing.T) {
		repo := &fakePasswordChangeRepo{err: repository.ErrUserNotFound}
		err := newChangePasswordUseCase(t, userWithPassword(t, changeCurrentPW), repo).
			ChangePassword(context.Background(), changeUserID, changeCurrentPW, changeNewPW)
		if !errors.Is(err, ErrInvalidActor) {
			t.Errorf("err = %v, want ErrInvalidActor", err)
		}
	})

	t.Run("repository not configured", func(t *testing.T) {
		err := newChangePasswordUseCase(t, userWithPassword(t, changeCurrentPW), nil).
			ChangePassword(context.Background(), changeUserID, changeCurrentPW, changeNewPW)
		if err == nil || !strings.Contains(err.Error(), "not configured") {
			t.Errorf("err = %v, want a configuration error", err)
		}
	})
}
