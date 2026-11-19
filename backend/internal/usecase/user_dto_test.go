package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/repository"
)

// User use case results as response DTOs (Mapper refactor Step 3.4).
// Expected JSON is written out by hand. User timestamps are RFC3339Nano in the
// value's own offset, and the zero time is "0001-01-01T00:00:00Z"; they are
// never the Auth format (RFC3339 to the second, zero -> "").

var (
	userCreatedAt = time.Date(2026, 3, 4, 5, 6, 7, 891234567, time.UTC)
	userUpdatedAt = time.Date(2026, 3, 5, 6, 7, 8, 120000000, time.UTC)
	userICT       = time.FixedZone("ICT", 7*60*60)
)

// assertNoUserSecrets fails if the DTO type of v has a field that could hold
// a password or a hash, or if its JSON mentions one or contains a secret.
func assertNoUserSecrets(t *testing.T, v any, secrets ...string) {
	t.Helper()
	typ := reflect.TypeOf(v)
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name + " " + typ.Field(i).Tag.Get("json"))
		if strings.Contains(name, "password") || strings.Contains(name, "hash") {
			t.Errorf("%s has sensitive field %s", typ, typ.Field(i).Name)
		}
	}
	body := userJSON(t, v)
	if lower := strings.ToLower(body); strings.Contains(lower, "password") || strings.Contains(lower, "hash") {
		t.Errorf("JSON mentions a password or hash: %s", body)
	}
	for _, s := range secrets {
		if strings.Contains(body, s) {
			t.Errorf("JSON leaks %q: %s", s, body)
		}
	}
}

func userJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	return string(b)
}

func assertUserJSON(t *testing.T, v any, want string) {
	t.Helper()
	if got := userJSON(t, v); got != want {
		t.Errorf("JSON mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestUserGetByID_DTO(t *testing.T) {
	t.Run("all fields, nanoseconds and offset kept", func(t *testing.T) {
		repo := &mockUserRepo{getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
			return &domain.User{ID: id, Email: "user@example.com", PasswordHash: "$2a$10$getSecret", Role: "user",
				CreatedAt: userCreatedAt, UpdatedAt: userUpdatedAt.In(userICT)}, nil
		}}
		got, err := NewUserUseCase(repo).GetByID(context.Background(), 42)
		if err != nil {
			t.Fatal(err)
		}
		assertUserJSON(t, got, `{"id":42,"email":"user@example.com","role":"user","created_at":"2026-03-04T05:06:07.891234567Z","updated_at":"2026-03-05T13:07:08.12+07:00"}`)
		assertNoUserSecrets(t, got, "$2a$10$getSecret")
	})

	t.Run("zero timestamps", func(t *testing.T) {
		repo := &mockUserRepo{getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
			return &domain.User{ID: id, Email: "zero@example.com", Role: "admin"}, nil
		}}
		got, err := NewUserUseCase(repo).GetByID(context.Background(), 43)
		if err != nil {
			t.Fatal(err)
		}
		assertUserJSON(t, got, `{"id":43,"email":"zero@example.com","role":"admin","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}`)
	})

	dbErr := errors.New("connection reset")
	for name, tc := range map[string]struct {
		id      int64
		repoErr error
		wantErr error
	}{
		"invalid ID":       {0, nil, ErrInvalidUserID},
		"not found":        {9, repository.ErrUserNotFound, ErrUserNotFound},
		"repository error": {9, dbErr, dbErr},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &mockUserRepo{getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
				if tc.repoErr != nil {
					return nil, tc.repoErr
				}
				return &domain.User{ID: id, Email: "x@example.com", Role: "user"}, nil
			}}
			got, err := NewUserUseCase(repo).GetByID(context.Background(), tc.id)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got, dto.UserResponse{}) {
				t.Errorf("result = %+v, want the zero DTO", got)
			}
		})
	}
}

// List never returns nil items (nil and empty pages are []), keeps the order
// and the repository total (also for a page past the end), and trims the
// query.
func TestUserList_DTO(t *testing.T) {
	list := func(users []domain.User, total int64, err error) ([]dto.UserResponse, int64, error, *string) {
		var gotQuery string
		repo := &mockUserRepo{listFn: func(ctx context.Context, query string, page, limit int) ([]domain.User, int64, error) {
			gotQuery = query
			return users, total, err
		}}
		items, n, e := NewUserUseCase(repo).List(context.Background(), "  example  ", 3, 10)
		return items, n, e, &gotQuery
	}

	t.Run("nil page past the end keeps the total", func(t *testing.T) {
		got, total, err, query := list(nil, 25, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || len(got) != 0 || total != 25 {
			t.Fatalf("List = %#v, %d; want a non-nil empty slice and total 25", got, total)
		}
		assertUserJSON(t, got, `[]`)
		if *query != "example" {
			t.Errorf("repository query = %q, want the trimmed %q", *query, "example")
		}
	})

	t.Run("empty page", func(t *testing.T) {
		got, total, err, _ := list([]domain.User{}, 0, nil)
		if err != nil || got == nil || len(got) != 0 || total != 0 {
			t.Fatalf("List = %#v, %d, %v; want [], 0, nil", got, total, err)
		}
	})

	t.Run("order, fields and timestamps", func(t *testing.T) {
		got, total, err, _ := list([]domain.User{
			{ID: 9, Email: "z@example.com", PasswordHash: "$2a$10$listSecretA", Role: "user", CreatedAt: userCreatedAt, UpdatedAt: userUpdatedAt},
			{ID: 2, Email: "b@example.com", PasswordHash: "$2a$10$listSecretB", Role: "admin", CreatedAt: userCreatedAt.In(userICT)},
		}, 40, nil)
		if err != nil {
			t.Fatal(err)
		}
		if total != 40 {
			t.Errorf("total = %d, want 40", total)
		}
		assertUserJSON(t, got, `[`+
			`{"id":9,"email":"z@example.com","role":"user","created_at":"2026-03-04T05:06:07.891234567Z","updated_at":"2026-03-05T06:07:08.12Z"},`+
			`{"id":2,"email":"b@example.com","role":"admin","created_at":"2026-03-04T12:06:07.891234567+07:00","updated_at":"0001-01-01T00:00:00Z"}]`)
		assertNoUserSecrets(t, got, "$2a$10$listSecretA", "$2a$10$listSecretB")
	})

	t.Run("repository error", func(t *testing.T) {
		dbErr := errors.New("connection reset")
		got, total, err, _ := list([]domain.User{{ID: 1}}, 1, dbErr)
		if !errors.Is(err, dbErr) || got != nil || total != 0 {
			t.Fatalf("List = %#v, %d, %v; want nil, 0, the repository error", got, total, err)
		}
	})

	t.Run("invalid pagination", func(t *testing.T) {
		uc := NewUserUseCase(&mockUserRepo{})
		if got, total, err := uc.List(context.Background(), "", 0, 10); !errors.Is(err, ErrInvalidPage) || got != nil || total != 0 {
			t.Errorf("page 0: %#v, %d, %v", got, total, err)
		}
		if got, total, err := uc.List(context.Background(), "", 1, 101); !errors.Is(err, ErrInvalidLimit) || got != nil || total != 0 {
			t.Errorf("limit 101: %#v, %d, %v", got, total, err)
		}
	})
}

// Update returns the stored created_at (full precision and offset), the
// updated_at the repository reports, and the normalized email/role, on both
// the plain update path and the role-change path.
func TestUserUpdate_DTO(t *testing.T) {
	existing := func() *domain.User {
		return &domain.User{ID: 10, Email: "existing@example.com", PasswordHash: "$2a$10$updateSecret", Role: "user",
			CreatedAt: userCreatedAt.In(userICT), UpdatedAt: userCreatedAt}
	}
	setUpdatedAt := func(ctx context.Context, u *domain.User) error {
		u.UpdatedAt = userUpdatedAt
		return nil
	}

	t.Run("email change", func(t *testing.T) {
		repo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) { return existing(), nil },
			updateFn:  setUpdatedAt,
		}
		email := "  Renamed@Example.COM "
		got, err := NewUserUseCase(repo).Update(context.Background(), 10, dto.UpdateUserInput{Email: &email})
		if err != nil {
			t.Fatal(err)
		}
		assertUserJSON(t, got, `{"id":10,"email":"renamed@example.com","role":"user","created_at":"2026-03-04T12:06:07.891234567+07:00","updated_at":"2026-03-05T06:07:08.12Z"}`)
		assertNoUserSecrets(t, got, "$2a$10$updateSecret")
	})

	t.Run("role change", func(t *testing.T) {
		repo := &mockUserRepo{
			getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) { return existing(), nil },
			updateAndRevokeTokensFn: func(ctx context.Context, u *domain.User, revokedAt time.Time) error {
				u.UpdatedAt = userUpdatedAt.In(userICT)
				return nil
			},
		}
		role := " admin "
		got, err := NewUserUseCase(repo).Update(context.Background(), 10, dto.UpdateUserInput{Role: &role})
		if err != nil {
			t.Fatal(err)
		}
		assertUserJSON(t, got, `{"id":10,"email":"existing@example.com","role":"admin","created_at":"2026-03-04T12:06:07.891234567+07:00","updated_at":"2026-03-05T13:07:08.12+07:00"}`)
		assertNoUserSecrets(t, got, "$2a$10$updateSecret")
	})

	dbErr := errors.New("connection reset")
	strPtr := func(s string) *string { return &s }
	for name, tc := range map[string]struct {
		id        int64
		input     dto.UpdateUserInput
		getErr    error
		updateErr error
		wantErr   error
	}{
		"invalid ID":       {0, dto.UpdateUserInput{Role: strPtr("admin")}, nil, nil, ErrInvalidUserID},
		"empty update":     {10, dto.UpdateUserInput{}, nil, nil, ErrEmptyUpdate},
		"not found":        {10, dto.UpdateUserInput{Role: strPtr("admin")}, repository.ErrUserNotFound, nil, ErrUserNotFound},
		"invalid email":    {10, dto.UpdateUserInput{Email: strPtr("bad")}, nil, nil, ErrInvalidEmail},
		"invalid role":     {10, dto.UpdateUserInput{Role: strPtr("superadmin")}, nil, nil, ErrInvalidUserRole},
		"duplicate email":  {10, dto.UpdateUserInput{Email: strPtr("taken@example.com")}, nil, repository.ErrEmailExists, ErrEmailExists},
		"repository error": {10, dto.UpdateUserInput{Email: strPtr("ok@example.com")}, nil, dbErr, dbErr},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &mockUserRepo{
				getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
					if tc.getErr != nil {
						return nil, tc.getErr
					}
					return existing(), nil
				},
				updateFn: func(ctx context.Context, u *domain.User) error { return tc.updateErr },
			}
			got, err := NewUserUseCase(repo).Update(context.Background(), tc.id, tc.input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got, dto.UserResponse{}) {
				t.Errorf("result = %+v, want the zero DTO", got)
			}
		})
	}
}
