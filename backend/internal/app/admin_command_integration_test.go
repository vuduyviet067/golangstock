package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gostock/internal/platform/jwt"
	"gostock/internal/repository/postgres"
	"gostock/internal/testdb"
	"gostock/internal/usecase"
)

// TestAdminCommand_PostgreSQL runs `gostock admin` through RunAdminCommand
// against the integration test database, with the preflight on, and checks
// the result by logging in through the auth use case.
func TestAdminCommand_PostgreSQL(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()

	// RunAdminCommand reads its configuration like the binary does.
	t.Setenv("DATABASE_URL", os.Getenv(testdb.EnvVar))
	t.Setenv("EXPECTED_DATABASE_NAME", testdb.RequiredName)

	tag := fmt.Sprintf("admin-cli-%d", time.Now().UnixNano())
	adminEmail := tag + "-admin@example.com"
	memberEmail := tag + "-member@example.com"
	// Registered before anything is written, so a failure cannot leave
	// accounts behind.
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM users WHERE email LIKE $1`, tag+"-%"); err != nil {
			t.Errorf("cleanup users: %v", err)
		}
	})

	jwtService, err := jwt.NewJWTService("a-very-long-and-secure-secret-key-32-bytes-minimum!", "gostock", "gostock-api", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	auth := usecase.NewAuthUseCase(postgres.NewUserRepository(db), postgres.NewRefreshTokenRepository(db), jwtService,
		15*time.Minute, 7*24*time.Hour, 15*time.Second)

	run := func(stdin string, args ...string) (int, string, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := RunAdminCommand(ctx, append([]string(nil), args...), strings.NewReader(stdin), &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}
	login := func(email, password string) error {
		_, err := auth.Login(ctx, email, password)
		return err
	}

	const firstPW, secondPW, memberPW = "first-admin-password", "second-admin-password", "member-password-1"
	secrets := []string{firstPW, secondPW, memberPW, "$2a$", "$2b$"}
	noSecrets := func(step, output string) {
		t.Helper()
		for _, s := range secrets {
			if strings.Contains(output, s) {
				t.Errorf("%s: output contains %q:\n%s", step, s, output)
			}
		}
	}

	// create
	code, stdout, stderr := run(firstPW+"\n", "create", "--email", adminEmail)
	noSecrets("create", stdout+stderr)
	if code != adminExitOK || stdout != "admin account created: "+adminEmail+"\n" {
		t.Fatalf("create: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "database preflight passed: "+testdb.RequiredName) {
		t.Errorf("create: the preflight did not run; stderr %q", stderr)
	}
	result, err := auth.Login(ctx, adminEmail, firstPW)
	if err != nil {
		t.Fatalf("login with the created admin: %v", err)
	}
	if result.User.Role != usecase.RoleAdmin {
		t.Errorf("created account role = %q, want admin", result.User.Role)
	}

	// create again: the email is taken
	code, stdout, stderr = run(firstPW+"\n", "create", "--email", adminEmail)
	noSecrets("create twice", stdout+stderr)
	if code != adminExitError || !strings.Contains(stderr, "an account with this email already exists") {
		t.Errorf("create twice: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	// reset-password: the old password stops working, the new one works
	code, stdout, stderr = run(secondPW+"\r\n", "reset-password", "--email", adminEmail)
	noSecrets("reset", stdout+stderr)
	if code != adminExitOK {
		t.Fatalf("reset: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if err := login(adminEmail, firstPW); !errors.Is(err, usecase.ErrInvalidCredentials) {
		t.Errorf("login with the old password: err = %v, want ErrInvalidCredentials", err)
	}
	if err := login(adminEmail, secondPW); err != nil {
		t.Errorf("login with the new password: %v", err)
	}

	// reset-password on a member account is refused and changes nothing
	if _, err := auth.Register(ctx, memberEmail, memberPW); err != nil {
		t.Fatalf("register member: %v", err)
	}
	code, stdout, stderr = run(secondPW+"\n", "reset-password", "--email", memberEmail)
	noSecrets("reset member", stdout+stderr)
	if code != adminExitError || !strings.Contains(stderr, "this account is not an admin") {
		t.Errorf("reset member: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if err := login(memberEmail, memberPW); err != nil {
		t.Errorf("member login after the refused reset: %v", err)
	}

	// reset-password on an unknown account
	code, _, stderr = run(secondPW+"\n", "reset-password", "--email", tag+"-nobody@example.com")
	if code != adminExitError || !strings.Contains(stderr, "no account with this email") {
		t.Errorf("reset unknown: code %d, stderr %q", code, stderr)
	}
}
