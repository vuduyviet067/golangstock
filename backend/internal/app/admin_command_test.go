package app

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"

	"gostock/internal/domain"
	"gostock/internal/repository"
	"gostock/internal/usecase"
)

// adminTestDSN is the connection string the tests "use"; its password must
// never be printed.
const (
	adminTestDSN        = "postgres://gostock:db-secret-pw@localhost:5432/gostock?sslmode=disable"
	adminTestDBPassword = "db-secret-pw"
)

// adminFakeUsers is an in-memory users table for the admin use case.
type adminFakeUsers struct {
	byEmail map[string]*domain.User
	nextID  int64
	revoked map[int64]bool
}

func newAdminFakeUsers() *adminFakeUsers {
	return &adminFakeUsers{byEmail: map[string]*domain.User{}, revoked: map[int64]bool{}}
}

func (f *adminFakeUsers) add(email, role, password string) {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	f.nextID++
	f.byEmail[email] = &domain.User{ID: f.nextID, Email: email, Role: role, PasswordHash: string(hash)}
}

func (f *adminFakeUsers) Create(_ context.Context, u *domain.User) error {
	if _, ok := f.byEmail[u.Email]; ok {
		return repository.ErrEmailExists
	}
	f.nextID++
	u.ID = f.nextID
	stored := *u
	f.byEmail[u.Email] = &stored
	return nil
}

func (f *adminFakeUsers) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	u, ok := f.byEmail[email]
	if !ok {
		return nil, repository.ErrUserNotFound
	}
	copied := *u
	return &copied, nil
}

func (f *adminFakeUsers) GetByID(_ context.Context, id int64) (*domain.User, error) {
	for _, u := range f.byEmail {
		if u.ID == id {
			copied := *u
			return &copied, nil
		}
	}
	return nil, repository.ErrUserNotFound
}

func (f *adminFakeUsers) UpdatePasswordAndRevokeTokens(_ context.Context, userID int64, hash string, _ time.Time) error {
	for _, u := range f.byEmail {
		if u.ID == userID {
			u.PasswordHash = hash
			f.revoked[userID] = true
			return nil
		}
	}
	return repository.ErrUserNotFound
}

func (f *adminFakeUsers) passwordMatches(email, password string) bool {
	u, ok := f.byEmail[email]
	return ok && bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil
}

// adminRun is the outcome of one admin command.
type adminRun struct {
	code           int
	stdout, stderr string
	opened         bool
}

// runAdminForTest runs the admin command against users, with stdin as the
// password input (a pipe), and fails the test if any secret appears in its
// output: the given passwords, the database password, or a bcrypt hash.
func runAdminForTest(t *testing.T, users *adminFakeUsers, stdin string, args []string, secrets ...string) adminRun {
	t.Helper()
	var run adminRun
	cmd := adminCommand{
		databaseURL:  func() (string, error) { return adminTestDSN, nil },
		expectedName: func() (string, error) { return "", nil },
		open: func(string) (*sql.DB, error) {
			run.opened = true
			db, _, err := sqlmock.New()
			return db, err
		},
		newUseCase: func(*sql.DB) *usecase.AdminUseCase {
			return usecase.NewAdminUseCaseWithPasswordReset(users, users)
		},
		passwords: newPasswordReader,
	}
	var stdout, stderr bytes.Buffer
	run.code = cmd.run(context.Background(), args, strings.NewReader(stdin), &stdout, &stderr)
	run.stdout, run.stderr = stdout.String(), stderr.String()
	assertNoSecrets(t, run.stdout+run.stderr, append(secrets, adminTestDBPassword)...)
	return run
}

func assertNoSecrets(t *testing.T, output string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if s != "" && strings.Contains(output, s) {
			t.Errorf("output contains the secret %q:\n%s", s, output)
		}
	}
	if strings.Contains(output, "$2a$") || strings.Contains(output, "$2b$") {
		t.Errorf("output contains a bcrypt hash:\n%s", output)
	}
}

func TestAdminCommand_Usage(t *testing.T) {
	const pw = "typed-on-the-command-line"
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string // in stderr; usage errors also print the usage
	}{
		{"no subcommand", nil, adminExitUsage, "missing subcommand"},
		{"unknown subcommand", []string{"delete", "--email", "a@example.com"}, adminExitUsage, "unknown subcommand"},
		{"missing --email", []string{"create"}, adminExitUsage, "--email is required"},
		{"empty --email", []string{"create", "--email", "  "}, adminExitUsage, "--email is required"},
		{"--email without a value", []string{"reset-password", "--email"}, adminExitUsage, "flag needs an argument"},
		{"password flag", []string{"create", "--email", "a@example.com", "--password", pw}, adminExitUsage, "flag provided but not defined"},
		{"password flag with =", []string{"create", "--email", "a@example.com", "--password=" + pw}, adminExitUsage, "flag provided but not defined"},
		{"password as an argument", []string{"create", "--email", "a@example.com", pw}, adminExitUsage, "unexpected argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := runAdminForTest(t, newAdminFakeUsers(), "", tt.args, pw)
			if run.code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; stderr: %s", run.code, tt.wantCode, run.stderr)
			}
			if !strings.Contains(run.stderr, tt.wantErr) || !strings.Contains(run.stderr, "usage:") {
				t.Errorf("stderr = %q, want %q and the usage", run.stderr, tt.wantErr)
			}
			if run.stdout != "" {
				t.Errorf("stdout = %q, want nothing", run.stdout)
			}
			if run.opened {
				t.Error("the database was opened for an invalid command line")
			}
		})
	}
}

func TestAdminCommand_Help(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}, {"create", "-h"}, {"reset-password", "--help"}} {
		run := runAdminForTest(t, newAdminFakeUsers(), "", args)
		if run.code != adminExitOK || !strings.Contains(run.stdout, "gostock admin create --email <email>") || run.stderr != "" {
			t.Errorf("%v: code %d, stdout %q, stderr %q; want 0 and the usage on stdout", args, run.code, run.stdout, run.stderr)
		}
		if run.opened {
			t.Errorf("%v: the database was opened for help", args)
		}
	}
}

func TestAdminCommand_Create(t *testing.T) {
	const pw = "s3cret-admin-pass"
	users := newAdminFakeUsers()
	run := runAdminForTest(t, users, pw+"\r\n", []string{"create", "--email", "  Boss@Example.com "}, pw)
	if run.code != adminExitOK {
		t.Fatalf("exit code = %d; stderr: %s", run.code, run.stderr)
	}
	if run.stdout != "admin account created: boss@example.com\n" {
		t.Errorf("stdout = %q", run.stdout)
	}
	u := users.byEmail["boss@example.com"]
	if u == nil || u.Role != usecase.RoleAdmin {
		t.Fatalf("stored user = %+v, want an admin", u)
	}
	if !users.passwordMatches("boss@example.com", pw) {
		t.Error("the stored hash does not match the password read from stdin (without its line ending)")
	}
}

func TestAdminCommand_CreateFailures(t *testing.T) {
	existing := newAdminFakeUsers()
	existing.add("taken@example.com", "user", "whatever-pass")
	tests := []struct {
		name    string
		users   *adminFakeUsers
		email   string
		stdin   string
		wantErr string
	}{
		{"invalid email", newAdminFakeUsers(), "not-an-email", "long-enough-pw\n", "invalid email address"},
		{"password of 7 bytes", newAdminFakeUsers(), "a@example.com", "1234567\n", "invalid password: it must be 8 to 72 bytes long"},
		{"password of 73 bytes", newAdminFakeUsers(), "a@example.com", strings.Repeat("p", 73) + "\n", "invalid password"},
		{"email already used", existing, "Taken@example.com", "long-enough-pw\n", "an account with this email already exists"},
		{"empty stdin", newAdminFakeUsers(), "a@example.com", "", "read password: the password is empty"},
		{"empty first line", newAdminFakeUsers(), "a@example.com", "\r\nlong-enough-pw\n", "read password: the password is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(tt.users.byEmail)
			pw := strings.TrimRight(tt.stdin, "\r\n")
			run := runAdminForTest(t, tt.users, tt.stdin, []string{"create", "--email", tt.email}, pw)
			if run.code != adminExitError {
				t.Fatalf("exit code = %d, want 1; stdout %q stderr %q", run.code, run.stdout, run.stderr)
			}
			if !strings.Contains(run.stderr, tt.wantErr) || run.stdout != "" {
				t.Errorf("stdout %q, stderr %q; want nothing and %q", run.stdout, run.stderr, tt.wantErr)
			}
			if len(tt.users.byEmail) != before {
				t.Error("a user was created")
			}
		})
	}
}

// The message of ErrInvalidPassword states the use case's length rules; this
// pins them, so a change of the rules shows up here.
func TestAdminCommand_PasswordLengthRules(t *testing.T) {
	for _, n := range []int{8, 72} {
		pw := strings.Repeat("x", n)
		run := runAdminForTest(t, newAdminFakeUsers(), pw+"\n", []string{"create", "--email", "a@example.com"}, pw)
		if run.code != adminExitOK {
			t.Errorf("%d bytes: exit code %d, stderr %s", n, run.code, run.stderr)
		}
	}
}

func TestAdminCommand_ResetPassword(t *testing.T) {
	const oldPW, newPW = "old-admin-password", "new-admin-password"
	users := newAdminFakeUsers()
	users.add("boss@example.com", usecase.RoleAdmin, oldPW)
	users.add("member@example.com", usecase.RoleUser, oldPW)

	run := runAdminForTest(t, users, newPW+"\n", []string{"reset-password", "--email", "BOSS@example.com"}, oldPW, newPW)
	if run.code != adminExitOK || run.stdout != "admin password reset: boss@example.com (its sessions were revoked)\n" {
		t.Fatalf("code %d, stdout %q, stderr %q", run.code, run.stdout, run.stderr)
	}
	if !users.passwordMatches("boss@example.com", newPW) || users.passwordMatches("boss@example.com", oldPW) {
		t.Error("the admin password was not replaced")
	}
	if !users.revoked[users.byEmail["boss@example.com"].ID] {
		t.Error("the admin's sessions were not revoked")
	}

	for _, tt := range []struct{ name, email, wantErr string }{
		{"not an admin", "member@example.com", "this account is not an admin"},
		{"unknown account", "nobody@example.com", "no account with this email"},
		{"invalid email", "nobody", "invalid email address"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			run := runAdminForTest(t, users, newPW+"\n", []string{"reset-password", "--email", tt.email}, oldPW, newPW)
			if run.code != adminExitError || !strings.Contains(run.stderr, tt.wantErr) || run.stdout != "" {
				t.Errorf("code %d, stdout %q, stderr %q; want 1 and %q", run.code, run.stdout, run.stderr, tt.wantErr)
			}
		})
	}
	if !users.passwordMatches("member@example.com", oldPW) {
		t.Error("the member's password changed")
	}
}

func TestAdminCommand_DatabaseFailures(t *testing.T) {
	const pw = "long-enough-pw"
	base := func() adminCommand {
		return adminCommand{
			databaseURL:  func() (string, error) { return adminTestDSN, nil },
			expectedName: func() (string, error) { return "", nil },
			newUseCase: func(*sql.DB) *usecase.AdminUseCase {
				t.Fatal("the use case must not be built")
				return nil
			},
			passwords: newPasswordReader,
		}
	}
	runCmd := func(cmd adminCommand, stdin string) (int, string) {
		var stdout, stderr bytes.Buffer
		code := cmd.run(context.Background(), []string{"create", "--email", "a@example.com"}, strings.NewReader(stdin), &stdout, &stderr)
		assertNoSecrets(t, stdout.String()+stderr.String(), pw, adminTestDBPassword)
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing", stdout.String())
		}
		return code, stderr.String()
	}

	t.Run("DATABASE_URL missing", func(t *testing.T) {
		cmd := base()
		cmd.databaseURL = func() (string, error) { return "", errors.New("DATABASE_URL environment variable is required") }
		cmd.open = func(string) (*sql.DB, error) { t.Fatal("must not connect"); return nil, nil }
		if code, stderr := runCmd(cmd, pw+"\n"); code != adminExitError || !strings.Contains(stderr, "DATABASE_URL") {
			t.Errorf("code %d, stderr %q", code, stderr)
		}
	})

	t.Run("connection error naming the connection string is redacted", func(t *testing.T) {
		cmd := base()
		cmd.open = func(dsn string) (*sql.DB, error) {
			return nil, errors.New("failed to connect to PostgreSQL: cannot parse `" + dsn + "`")
		}
		code, stderr := runCmd(cmd, pw+"\n")
		if code != adminExitError || !strings.Contains(stderr, "failed to connect to PostgreSQL") || !strings.Contains(stderr, "***") {
			t.Errorf("code %d, stderr %q", code, stderr)
		}
	})

	t.Run("preflight on another database stops before reading the password", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectQuery(`SELECT current_database\(\)`).WillReturnRows(sqlmock.NewRows([]string{"current_database"}).AddRow("gostock_dev"))
		cmd := base()
		cmd.expectedName = func() (string, error) { return "gostock_prod", nil }
		cmd.open = func(string) (*sql.DB, error) { return db, nil }
		cmd.passwords = func(io.Reader, io.Writer) passwordReader {
			t.Fatal("the password must not be read after a failed preflight")
			return nil
		}
		code, stderr := runCmd(cmd, pw+"\n")
		if code != adminExitError || !strings.Contains(stderr, `database preflight: connected to "gostock_dev", expected "gostock_prod"`) {
			t.Errorf("code %d, stderr %q", code, stderr)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})

	t.Run("preflight on the expected database", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectQuery(`SELECT current_database\(\)`).WillReturnRows(sqlmock.NewRows([]string{"current_database"}).AddRow("gostock_prod"))
		users := newAdminFakeUsers()
		cmd := base()
		cmd.expectedName = func() (string, error) { return "gostock_prod", nil }
		cmd.open = func(string) (*sql.DB, error) { return db, nil }
		cmd.newUseCase = func(*sql.DB) *usecase.AdminUseCase { return usecase.NewAdminUseCaseWithPasswordReset(users, users) }
		var stdout, stderr bytes.Buffer
		code := cmd.run(context.Background(), []string{"create", "--email", "a@example.com"}, strings.NewReader(pw+"\n"), &stdout, &stderr)
		assertNoSecrets(t, stdout.String()+stderr.String(), pw, adminTestDBPassword)
		if code != adminExitOK || !strings.Contains(stderr.String(), "database preflight passed: gostock_prod") {
			t.Errorf("code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
		}
	})
}

func TestLinePasswordReader(t *testing.T) {
	tests := []struct {
		name, input, want string
		wantErr           error
	}{
		{"LF", "pass word\n", "pass word", nil},
		{"CRLF", "pass word\r\n", "pass word", nil},
		{"no line ending", "pass word", "pass word", nil},
		{"only the first line", "first\nsecond\n", "first", nil},
		{"inner spaces and tabs are kept", " \tpw \n", " \tpw ", nil},
		{"empty input", "", "", errEmptyPassword},
		{"empty line", "\n", "", errEmptyPassword},
		{"CRLF only", "\r\n", "", errEmptyPassword},
		{"too long", strings.Repeat("p", maxPasswordLine+1) + "\n", "", errPasswordTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := linePasswordReader{r: strings.NewReader(tt.input)}.ReadPassword(context.Background())
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Errorf("got %q, %v; want %q, %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

// fakeTerminal returns its lines in order, recording Restore.
type fakeTerminal struct {
	lines    []string
	block    chan struct{} // when set, ReadPassword waits on it
	restored bool
}

func (f *fakeTerminal) ReadPassword() ([]byte, error) {
	if f.block != nil {
		<-f.block
	}
	if len(f.lines) == 0 {
		return nil, io.EOF
	}
	line := f.lines[0]
	f.lines = f.lines[1:]
	return []byte(line), nil
}

func (f *fakeTerminal) Restore() { f.restored = true }

func TestTerminalPasswordReader(t *testing.T) {
	const pw = "terminal-pass"
	tests := []struct {
		name    string
		lines   []string
		want    string
		wantErr error
	}{
		{"matching entries", []string{pw, pw}, pw, nil},
		{"different entries", []string{pw, pw + "x"}, "", errPasswordMismatch},
		{"empty entry", []string{"", ""}, "", errEmptyPassword},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var prompt bytes.Buffer
			got, err := terminalPasswordReader{terminal: &fakeTerminal{lines: tt.lines}, prompt: &prompt}.ReadPassword(context.Background())
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Errorf("got %q, %v; want %q, %v", got, err, tt.want, tt.wantErr)
			}
			if !strings.Contains(prompt.String(), "Password: ") {
				t.Errorf("prompt = %q", prompt.String())
			}
			assertNoSecrets(t, prompt.String(), pw)
		})
	}

	t.Run("interrupt restores the terminal", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		term := &fakeTerminal{lines: []string{pw}, block: make(chan struct{})}
		defer close(term.block)
		_, err := terminalPasswordReader{terminal: term, prompt: io.Discard}.ReadPassword(ctx)
		if !errors.Is(err, context.Canceled) || !term.restored {
			t.Errorf("err %v, restored %v; want context.Canceled and a restored terminal", err, term.restored)
		}
	})
}

func TestDSNPassword(t *testing.T) {
	for dsn, want := range map[string]string{
		"postgres://u:p%40ss@h/db":                  "p@ss",
		"postgres://u@h/db":                         "",
		"host=h user=u password=kv-secret dbname=d": "kv-secret",
		"host=h user=u password='quoted' dbname=d":  "quoted",
		"": "",
	} {
		if got := dsnPassword(dsn); got != want {
			t.Errorf("dsnPassword(%q) = %q, want %q", dsn, got, want)
		}
	}
}
