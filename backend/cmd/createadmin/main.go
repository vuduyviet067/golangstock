package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gostock/config"
	"gostock/internal/domain"
	"gostock/internal/platform/database"
	"gostock/internal/repository/postgres"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

const (
	passwordMinLen = 8
	passwordMaxLen = 72
	dbTimeout      = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("==================================================")
	fmt.Println(" GoStock - First Admin Account Initialization CLI")
	fmt.Println("==================================================")

	// 1. Read admin email interactively
	email, err := readEmail()
	if err != nil {
		return fmt.Errorf("read email: %w", err)
	}

	if err := validateEmail(email); err != nil {
		return fmt.Errorf("invalid email: %w", err)
	}

	// 2. Read admin password and confirmation securely (no echo)
	password, err := readPassword("Enter admin password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}

	if err := validatePassword(password); err != nil {
		return fmt.Errorf("invalid password: %w", err)
	}

	confirmPassword, err := readPassword("Confirm admin password: ")
	if err != nil {
		return fmt.Errorf("read password confirmation: %w", err)
	}

	if password != confirmPassword {
		return errors.New("passwords do not match")
	}

	// 3. Load database configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	// 4. Open PostgreSQL connection
	db, err := database.OpenPostgres(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	userRepo := postgres.NewUserRepository(db)

	// 5. Pre-check: Ensure account does not already exist
	existingUser, err := userRepo.GetByEmail(ctx, email)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		return fmt.Errorf("check existing user: %w", err)
	}
	if existingUser != nil {
		return fmt.Errorf("an account with email %q already exists; operation aborted without changes", email)
	}

	// 6. Hash password using bcrypt
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// 7. Create domain.User with Role: "admin"
	adminUser := &domain.User{
		Email:        email,
		PasswordHash: string(passwordHash),
		Role:         "admin",
	}

	if err := userRepo.Create(ctx, adminUser); err != nil {
		if errors.Is(err, domain.ErrEmailExists) {
			return fmt.Errorf("an account with email %q already exists; operation aborted", email)
		}
		return fmt.Errorf("create admin account: %w", err)
	}

	fmt.Printf("\nSuccess: Admin account %q (ID: %d, Role: %s) created successfully.\n",
		adminUser.Email, adminUser.ID, adminUser.Role)
	return nil
}

func readEmail() (string, error) {
	fmt.Print("Enter admin email: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(line)), nil
}

func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	var bytePassword []byte
	var err error

	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		bytePassword, err = term.ReadPassword(fd)
		fmt.Println()
	} else {
		// Non-terminal fallback (e.g. piped input in scripts)
		reader := bufio.NewReader(os.Stdin)
		line, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return "", readErr
		}
		bytePassword = []byte(strings.TrimRight(line, "\r\n"))
	}

	if err != nil {
		return "", err
	}

	return string(bytePassword), nil
}

func validateEmail(email string) error {
	if email == "" {
		return errors.New("email cannot be empty")
	}
	if len(email) > 255 {
		return errors.New("email length cannot exceed 255 characters")
	}

	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return errors.New("email must contain a valid '@' symbol with local and domain parts")
	}

	if strings.ContainsAny(email, " \t\r\n") {
		return errors.New("email cannot contain whitespace")
	}

	if !strings.Contains(email[at+1:], ".") {
		return errors.New("email domain must contain at least one '.'")
	}

	return nil
}

func validatePassword(password string) error {
	if len(password) < passwordMinLen {
		return fmt.Errorf("password must be at least %d characters", passwordMinLen)
	}
	if len(password) > passwordMaxLen {
		return fmt.Errorf("password cannot exceed %d characters", passwordMaxLen)
	}
	return nil
}
