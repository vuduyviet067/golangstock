package mapper_test

import (
	"reflect"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/mapper"
)

func TestToUserResponse(t *testing.T) {
	user := domain.User{
		ID: 42, Email: "user@example.com", PasswordHash: passwordHash, Role: "user",
		CreatedAt: createdAt, UpdatedAt: updatedAt.In(ict),
	}
	before := user

	got := mapper.ToUserResponse(user)

	if got.ID != 42 || got.Email != "user@example.com" || got.Role != "user" {
		t.Errorf("ToUserResponse = %+v, want id 42, user@example.com, user", got)
	}
	if !got.CreatedAt.Equal(createdAt) || got.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt = %v, want %v in UTC", got.CreatedAt, createdAt)
	}
	if !got.UpdatedAt.Equal(updatedAt) || got.UpdatedAt.Location() != ict {
		t.Errorf("UpdatedAt = %v, want %v in ICT", got.UpdatedAt, updatedAt.In(ict))
	}
	assertJSON(t, got, `{"id":42,"email":"user@example.com","role":"user","created_at":"2026-03-04T05:06:07.891234567Z","updated_at":"2026-03-05T13:07:08.12+07:00"}`)
	assertNoSecretFields(t, got)

	if user != before {
		t.Errorf("input was modified: %+v, want %+v", user, before)
	}
	if again := mapper.ToUserResponse(user); !reflect.DeepEqual(again, got) {
		t.Errorf("second call = %+v, want %+v (mapping must be deterministic)", again, got)
	}
}

// User timestamps: RFC3339Nano (trailing zeros trimmed) in the value's own
// offset; the zero time is "0001-01-01T00:00:00Z", never "" or null.
func TestToUserResponse_Timestamps(t *testing.T) {
	check := func(name string, in time.Time, want string) {
		t.Run(name, func(t *testing.T) {
			got := mapper.ToUserResponse(domain.User{ID: 1, Email: "a@example.com", Role: "user", CreatedAt: in, UpdatedAt: in})
			assertJSON(t, got, `{"id":1,"email":"a@example.com","role":"user","created_at":"`+want+`","updated_at":"`+want+`"}`)
		})
	}

	check("UTC keeps nanoseconds", createdAt, "2026-03-04T05:06:07.891234567Z")
	check("trailing zeros trimmed", updatedAt, "2026-03-05T06:07:08.12Z")
	check("positive offset kept", createdAt.In(ict), "2026-03-04T12:06:07.891234567+07:00")
	check("negative half-hour offset kept", createdAt.In(nst), "2026-03-03T23:36:07.891234567-05:30")
	check("microseconds from PostgreSQL", time.Date(2026, 11, 15, 3, 53, 9, 478442000, ict), "2026-11-15T03:53:09.478442+07:00")
	check("whole second", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), "2026-01-02T03:04:05Z")
	check("zero time", time.Time{}, "0001-01-01T00:00:00Z")
}

// The same user maps to different created_at text for Auth and for Users.
func TestAuthAndUserTimestampRulesDiffer(t *testing.T) {
	user := domain.User{ID: 3, Email: "c@example.com", Role: "admin", CreatedAt: createdAt.In(ict)}

	assertJSON(t, mapper.ToAuthUserResponse(user),
		`{"id":3,"email":"c@example.com","role":"admin","created_at":"2026-03-04T12:06:07+07:00"}`)
	assertJSON(t, mapper.ToUserResponse(user),
		`{"id":3,"email":"c@example.com","role":"admin","created_at":"2026-03-04T12:06:07.891234567+07:00","updated_at":"0001-01-01T00:00:00Z"}`)

	zero := domain.User{ID: 4, Email: "d@example.com", Role: "user"}
	assertJSON(t, mapper.ToAuthUserResponse(zero), `{"id":4,"email":"d@example.com","role":"user","created_at":""}`)
	assertJSON(t, mapper.ToUserResponse(zero),
		`{"id":4,"email":"d@example.com","role":"user","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}`)
}

func TestToUserResponses(t *testing.T) {
	t.Run("nil becomes empty, not nil", func(t *testing.T) {
		got := mapper.ToUserResponses(nil)
		if got == nil || len(got) != 0 {
			t.Fatalf("ToUserResponses(nil) = %#v, want a non-nil empty slice", got)
		}
		assertJSON(t, got, `[]`)
	})

	t.Run("empty stays empty", func(t *testing.T) {
		got := mapper.ToUserResponses([]domain.User{})
		if got == nil || len(got) != 0 {
			t.Fatalf("ToUserResponses(empty) = %#v, want a non-nil empty slice", got)
		}
		assertJSON(t, got, `[]`)
	})

	t.Run("order and fields kept, no password", func(t *testing.T) {
		users := []domain.User{
			{ID: 9, Email: "z@example.com", PasswordHash: passwordHash, Role: "user", CreatedAt: createdAt, UpdatedAt: updatedAt},
			{ID: 2, Email: "b@example.com", PasswordHash: passwordHash, Role: "admin", CreatedAt: createdAt.In(ict), UpdatedAt: updatedAt.In(ict)},
			{ID: 5, Email: "zero@example.com", PasswordHash: passwordHash, Role: "user"},
		}
		before := append([]domain.User(nil), users...)

		got := mapper.ToUserResponses(users)

		assertJSON(t, got, `[`+
			`{"id":9,"email":"z@example.com","role":"user","created_at":"2026-03-04T05:06:07.891234567Z","updated_at":"2026-03-05T06:07:08.12Z"},`+
			`{"id":2,"email":"b@example.com","role":"admin","created_at":"2026-03-04T12:06:07.891234567+07:00","updated_at":"2026-03-05T13:07:08.12+07:00"},`+
			`{"id":5,"email":"zero@example.com","role":"user","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}`+
			`]`)
		assertNoSecretFields(t, got)
		if !reflect.DeepEqual(users, before) {
			t.Errorf("input was modified: %+v, want %+v", users, before)
		}
	})
}
