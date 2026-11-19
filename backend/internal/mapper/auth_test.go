package mapper_test

import (
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/mapper"
)

func TestToAuthUserResponse(t *testing.T) {
	user := domain.User{
		ID: 7, Email: "admin@example.com", PasswordHash: passwordHash, Role: "admin",
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
	before := user

	got := mapper.ToAuthUserResponse(user)

	want := dto.AuthUserResponse{ID: 7, Email: "admin@example.com", Role: "admin", CreatedAt: "2026-03-04T05:06:07Z"}
	if got != want {
		t.Fatalf("ToAuthUserResponse = %+v, want %+v", got, want)
	}
	assertJSON(t, got, `{"id":7,"email":"admin@example.com","role":"admin","created_at":"2026-03-04T05:06:07Z"}`)
	assertNoSecretFields(t, got)

	if user != before {
		t.Errorf("input was modified: %+v, want %+v", user, before)
	}
	if again := mapper.ToAuthUserResponse(user); again != got {
		t.Errorf("second call = %+v, want %+v (mapping must be deterministic)", again, got)
	}
}

// Auth created_at: time.RFC3339 to the second, in the value's own offset;
// the zero time (in any location) becomes "".
func TestToAuthUserResponse_CreatedAt(t *testing.T) {
	check := func(name string, in time.Time, want string) {
		t.Run(name, func(t *testing.T) {
			got := mapper.ToAuthUserResponse(domain.User{ID: 1, Email: "a@example.com", Role: "user", CreatedAt: in})
			if got.CreatedAt != want {
				t.Errorf("created_at = %q, want %q", got.CreatedAt, want)
			}
		})
	}

	check("UTC drops sub-seconds", createdAt, "2026-03-04T05:06:07Z")
	check("positive offset kept", createdAt.In(ict), "2026-03-04T12:06:07+07:00")
	check("negative half-hour offset kept", createdAt.In(nst), "2026-03-03T23:36:07-05:30")
	check("whole second", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), "2026-01-02T03:04:05Z")
	check("Unix epoch is not zero", time.Unix(0, 0).UTC(), "1970-01-01T00:00:00Z")
	check("zero time", time.Time{}, "")
	check("zero time in another location", time.Time{}.In(ict), "")
}

// /auth/me builds the user from JWT claims only (no timestamps), so the
// mapper must yield created_at "" for it.
func TestToAuthUserResponse_ClaimsOnlyUser(t *testing.T) {
	got := mapper.ToAuthUserResponse(domain.User{ID: 1, Email: "test@example.com", Role: "user"})

	assertJSON(t, got, `{"id":1,"email":"test@example.com","role":"user","created_at":""}`)
}

func TestToAuthUserResponse_ZeroUser(t *testing.T) {
	assertJSON(t, mapper.ToAuthUserResponse(domain.User{}), `{"id":0,"email":"","role":"","created_at":""}`)
}
