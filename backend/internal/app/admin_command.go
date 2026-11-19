package app

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"golang.org/x/term"

	"gostock/config"
	"gostock/internal/platform/database"
	"gostock/internal/repository/postgres"
	"gostock/internal/usecase"
)

// Admin command (TD-09): `gostock admin create|reset-password --email <email>`
// creates an admin account or resets an admin's password on the server. The
// password is read from standard input only, never from an argument or an
// environment variable. There is no HTTP API for it.

// Exit codes of RunAdminCommand.
const (
	adminExitOK    = 0 // the command succeeded (or help was printed)
	adminExitError = 1 // the command ran and failed
	adminExitUsage = 2 // the command line is invalid
)

const adminUsage = `usage:
  gostock admin create --email <email>
  gostock admin reset-password --email <email>

create          creates an admin account
reset-password  sets a new password for an admin account and revokes its sessions

The password is read from standard input, never from arguments or environment
variables: a terminal asks for it twice without echo; a pipe gives one line.
Requires DATABASE_URL; EXPECTED_DATABASE_NAME is checked as by the server.
`

// maxPasswordLine bounds the line read from a pipe for a password.
const maxPasswordLine = 4096

var (
	errAdminHelp        = errors.New("help requested")
	errEmptyPassword    = errors.New("the password is empty")
	errPasswordMismatch = errors.New("the passwords do not match")
	errPasswordTooLong  = errors.New("the password line is too long")
)

// RunAdminCommand runs `gostock admin <args>` and returns the process exit
// code: 0 on success, 1 when the command fails, 2 for an invalid command
// line. stdout receives the success message, stderr the prompts, usage and
// errors. It never writes the password, its hash or the database password.
func RunAdminCommand(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd := adminCommand{
		databaseURL:  config.LoadDatabaseURL,
		expectedName: config.LoadExpectedDatabaseName,
		open:         database.OpenPostgres,
		newUseCase: func(db *sql.DB) *usecase.AdminUseCase {
			users := postgres.NewUserRepository(db)
			return usecase.NewAdminUseCaseWithPasswordReset(users, users)
		},
		passwords: newPasswordReader,
	}
	return cmd.run(ctx, args, stdin, stdout, stderr)
}

// adminCommand holds the dependencies of the admin command, replaced in tests.
type adminCommand struct {
	databaseURL  func() (string, error)
	expectedName func() (string, error)
	open         func(dsn string) (*sql.DB, error)
	newUseCase   func(db *sql.DB) *usecase.AdminUseCase
	passwords    func(stdin io.Reader, prompt io.Writer) passwordReader
}

func (c adminCommand) run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	sub, email, err := parseAdminArgs(args)
	if errors.Is(err, errAdminHelp) {
		fmt.Fprint(stdout, adminUsage)
		return adminExitOK
	}
	if err != nil {
		fmt.Fprintf(stderr, "gostock admin: %v\n\n%s", err, adminUsage)
		return adminExitUsage
	}

	var secrets []string
	fail := func(err error) int {
		fmt.Fprintf(stderr, "gostock admin %s: %s\n", sub, redact(adminErrorMessage(err), secrets...))
		return adminExitError
	}

	dsn, err := c.databaseURL()
	if err != nil {
		return fail(err)
	}
	secrets = append(secrets, dsnPassword(dsn))
	expected, err := c.expectedName()
	if err != nil {
		return fail(err)
	}

	db, err := c.open(dsn)
	if err != nil {
		return fail(err)
	}
	defer db.Close()

	// The same preflight as the server, before anything is written.
	if expected != "" {
		preflightCtx, cancel := context.WithTimeout(ctx, databasePreflightTimeout)
		err := database.VerifyDatabaseName(preflightCtx, db, expected)
		cancel()
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(stderr, "database preflight passed: %s\n", expected)
	}

	password, err := c.passwords(stdin, stderr).ReadPassword(ctx)
	if err != nil {
		return fail(fmt.Errorf("read password: %w", err))
	}
	secrets = append(secrets, password)

	uc := c.newUseCase(db)
	switch sub {
	case "create":
		admin, err := uc.CreateAdmin(ctx, email, password)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "admin account created: %s\n", admin.Email)
	case "reset-password":
		if err := uc.ResetAdminPassword(ctx, email, password); err != nil {
			return fail(err)
		}
		fmt.Fprintf(stdout, "admin password reset: %s (its sessions were revoked)\n",
			strings.ToLower(strings.TrimSpace(email)))
	}
	return adminExitOK
}

// parseAdminArgs parses `<subcommand> --email <email>`. Errors name flags and
// arguments by position only, never by value: a password typed on the
// command line by mistake must not be echoed.
func parseAdminArgs(args []string) (sub, email string, err error) {
	if len(args) == 0 {
		return "", "", errors.New("missing subcommand")
	}
	sub = args[0]
	switch sub {
	case "help", "-h", "-help", "--help":
		return "", "", errAdminHelp
	case "create", "reset-password":
	default:
		return "", "", errors.New("unknown subcommand (want create or reset-password)")
	}

	fs := flag.NewFlagSet("gostock admin "+sub, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&email, "email", "", "email of the admin account")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return "", "", errAdminHelp
		}
		return "", "", err
	}
	if fs.NArg() > 0 {
		return "", "", errors.New("unexpected argument; the password is read from standard input, never from arguments")
	}
	if strings.TrimSpace(email) == "" {
		return "", "", errors.New("--email is required")
	}
	return sub, email, nil
}

// adminErrorMessage explains a failure of the admin command.
func adminErrorMessage(err error) string {
	switch {
	case errors.Is(err, usecase.ErrInvalidEmail):
		return "invalid email address"
	case errors.Is(err, usecase.ErrInvalidPassword):
		return "invalid password: it must be 8 to 72 bytes long"
	case errors.Is(err, usecase.ErrEmailExists):
		return "an account with this email already exists"
	case errors.Is(err, usecase.ErrUserNotFound):
		return "no account with this email"
	case errors.Is(err, usecase.ErrNotAdmin):
		return "this account is not an admin; reset-password only changes admin passwords"
	}
	return err.Error()
}

// redact replaces every secret of three bytes or more in s.
func redact(s string, secrets ...string) string {
	for _, secret := range secrets {
		if len(secret) >= 3 {
			s = strings.ReplaceAll(s, secret, "***")
		}
	}
	return s
}

// dsnPassword returns the password of a URL or keyword/value connection
// string, or "".
func dsnPassword(dsn string) string {
	if u, err := url.Parse(dsn); err == nil && u.User != nil {
		password, _ := u.User.Password()
		return password
	}
	for _, field := range strings.Fields(dsn) {
		if value, ok := strings.CutPrefix(field, "password="); ok {
			return strings.Trim(value, "'")
		}
	}
	return ""
}

// passwordReader reads the password of the admin command.
type passwordReader interface {
	ReadPassword(ctx context.Context) (string, error)
}

// newPasswordReader asks on the terminal when stdin is one, and reads one
// line otherwise.
func newPasswordReader(stdin io.Reader, prompt io.Writer) passwordReader {
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		return terminalPasswordReader{terminal: newOSTerminal(int(f.Fd())), prompt: prompt}
	}
	return linePasswordReader{r: stdin}
}

// linePasswordReader reads the first line of a pipe, without its line ending
// ("\n" or "\r\n").
type linePasswordReader struct {
	r io.Reader
}

func (p linePasswordReader) ReadPassword(context.Context) (string, error) {
	var line []byte
	b := make([]byte, 1)
	for {
		n, err := p.r.Read(b)
		if n == 1 {
			if b[0] == '\n' {
				break
			}
			if len(line) == maxPasswordLine {
				return "", errPasswordTooLong
			}
			line = append(line, b[0])
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
	}
	password := strings.TrimSuffix(string(line), "\r")
	if password == "" {
		return "", errEmptyPassword
	}
	return password, nil
}

// passwordTerminal reads a line without echo.
type passwordTerminal interface {
	ReadPassword() ([]byte, error)
	// Restore undoes the terminal changes of a ReadPassword that was
	// interrupted, so that echo is back on.
	Restore()
}

// terminalPasswordReader asks for the password twice on a terminal.
type terminalPasswordReader struct {
	terminal passwordTerminal
	prompt   io.Writer
}

func (p terminalPasswordReader) ReadPassword(ctx context.Context) (string, error) {
	first, err := p.ask(ctx, "Password: ")
	if err != nil {
		return "", err
	}
	if first == "" {
		return "", errEmptyPassword
	}
	second, err := p.ask(ctx, "Repeat password: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", errPasswordMismatch
	}
	return first, nil
}

// ask prompts and reads one line. An interrupt (ctx canceled) restores the
// terminal instead of leaving echo off.
func (p terminalPasswordReader) ask(ctx context.Context, label string) (string, error) {
	fmt.Fprint(p.prompt, label)
	var line []byte
	done := make(chan error, 1)
	go func() {
		var err error
		line, err = p.terminal.ReadPassword()
		done <- err
	}()
	select {
	case <-ctx.Done():
		p.terminal.Restore()
		fmt.Fprintln(p.prompt)
		return "", ctx.Err()
	case err := <-done:
		fmt.Fprintln(p.prompt)
		if err != nil {
			return "", err
		}
		return string(line), nil
	}
}

// osTerminal is the process terminal on file descriptor fd. Its state is
// saved before any read, so Restore never races with ReadPassword.
type osTerminal struct {
	fd    int
	state *term.State
}

func newOSTerminal(fd int) osTerminal {
	state, err := term.GetState(fd)
	if err != nil {
		state = nil
	}
	return osTerminal{fd: fd, state: state}
}

func (t osTerminal) ReadPassword() ([]byte, error) {
	return term.ReadPassword(t.fd)
}

func (t osTerminal) Restore() {
	if t.state != nil {
		_ = term.Restore(t.fd, t.state)
	}
}
