package usecase

import (
	"context"
	"errors"
	"testing"
	"time"
)

// XG-03b: self-registration is off unless the use case is built with
// WithRegistration(true). With it off, Register refuses before looking at
// the input and never reaches the repository.
func TestRegister_Disabled(t *testing.T) {
	for name, opts := range map[string][]AuthOption{
		"no option (default)":     nil,
		"WithRegistration(false)": {WithRegistration(false)},
		"last option wins":        {WithRegistration(true), WithRegistration(false)},
	} {
		t.Run(name, func(t *testing.T) {
			users := &fakeUserRepository{}
			uc := NewAuthUseCase(users, &fakeRefreshTokenRepository{}, &fakeTokenManager{},
				15*time.Minute, 7*24*time.Hour, 15*time.Second, opts...)

			for _, in := range []struct{ email, password string }{
				{"new@example.com", "valid-password"},
				{"not-an-email", "x"}, // invalid input: still refused as disabled
			} {
				_, err := uc.Register(context.Background(), in.email, in.password)
				if !errors.Is(err, ErrRegistrationDisabled) {
					t.Errorf("Register(%q) err = %v, want ErrRegistrationDisabled", in.email, err)
				}
			}
			if users.createCall != 0 {
				t.Errorf("repository Create called %d time(s)", users.createCall)
			}
		})
	}
}

func TestRegister_Enabled(t *testing.T) {
	users := &fakeUserRepository{}
	uc := NewAuthUseCase(users, &fakeRefreshTokenRepository{}, &fakeTokenManager{},
		15*time.Minute, 7*24*time.Hour, 15*time.Second, WithRegistration(true))
	if _, err := uc.Register(context.Background(), "new@example.com", "valid-password"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if users.createCall != 1 || users.created == nil || users.created.Role != RoleUser {
		t.Errorf("created %+v after %d call(s), want one user with role %q", users.created, users.createCall, RoleUser)
	}
}
