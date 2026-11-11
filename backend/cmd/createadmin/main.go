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
	"gostock/internal/platform/database"
	"gostock/internal/repository/postgres"
	"gostock/internal/usecase"

	"golang.org/x/term"
)

const dbTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run is the CLI's composition root: it collects input, wires dependencies,
// and delegates all business rules (validation, duplicate check, hashing,
// role assignment) to usecase.AdminUseCase.
func run() error {
	fmt.Println("==================================================")
	fmt.Println(" GoStock - First Admin Account Initialization CLI")
	fmt.Println("==================================================")

	// 1. Load database configuration. Only DATABASE_URL (and the optional
	// EXPECTED_DATABASE_NAME) is needed here, so server-only settings
	// (JWT_SECRET, CORS_ALLOWED_ORIGINS) are not required.
	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	expectedDatabaseName, err := config.LoadExpectedDatabaseName()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	// 2. Open PostgreSQL connection and, when EXPECTED_DATABASE_NAME is set,
	// confirm on this pool that it is the expected database. Both happen
	// before any credential is asked for, so a wrong database fails first.
	db, err := database.OpenPostgres(databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	if expectedDatabaseName != "" {
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		err := database.VerifyDatabaseName(ctx, db, expectedDatabaseName)
		cancel()
		if err != nil {
			return err
		}
		fmt.Printf("Database preflight passed: %s\n", expectedDatabaseName)
	}

	// 3. Read admin credentials interactively. Confirmation is an interaction
	// concern, so it stays here; validation belongs to the use case.
	email, err := readEmail()
	if err != nil {
		return fmt.Errorf("read email: %w", err)
	}

	password, err := readPassword("Enter admin password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}

	confirmPassword, err := readPassword("Confirm admin password: ")
	if err != nil {
		return fmt.Errorf("read password confirmation: %w", err)
	}

	if password != confirmPassword {
		return errors.New("passwords do not match")
	}

	// 4. Wire dependencies
	adminUseCase := usecase.NewAdminUseCase(postgres.NewUserRepository(db))

	// 5. Create the admin account
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	admin, err := adminUseCase.CreateAdmin(ctx, email, password)
	if err != nil {
		return fmt.Errorf("create admin account: %w", err)
	}

	fmt.Printf("\nSuccess: Admin account %q (ID: %d, Role: %s) created successfully.\n",
		admin.Email, admin.ID, admin.Role)
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
