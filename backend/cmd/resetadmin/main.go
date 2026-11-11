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

// run is the CLI composition root. It collects credentials, wires
// dependencies, and delegates reset rules to AdminUseCase.
func run() error {
	fmt.Println("==================================================")
	fmt.Println(" GoStock - Admin Password Reset CLI")
	fmt.Println("==================================================")
	fmt.Println("This command resets the password of an existing admin account.")
	fmt.Println("It does not create accounts or change user roles.")
	fmt.Println()

	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	expectedDatabaseName, err := config.LoadExpectedDatabaseName()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	// Open the database and, when EXPECTED_DATABASE_NAME is set, confirm on
	// this pool that it is the expected database, before asking for any
	// credential.
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

	reader := bufio.NewReader(os.Stdin)

	email, err := readEmail(reader)
	if err != nil {
		return fmt.Errorf("read email: %w", err)
	}

	password, err := readPassword(reader, "Enter new admin password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}

	confirmPassword, err := readPassword(reader, "Confirm new admin password: ")
	if err != nil {
		return fmt.Errorf("read password confirmation: %w", err)
	}

	if password != confirmPassword {
		return errors.New("passwords do not match")
	}

	repo := postgres.NewUserRepository(db)
	adminUseCase := usecase.NewAdminUseCaseWithPasswordReset(repo, repo)

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	if err := adminUseCase.ResetAdminPassword(ctx, email, password); err != nil {
		return fmt.Errorf("reset admin password: %w", err)
	}

	fmt.Printf(
		"\nSuccess: Password for admin account %q has been reset.\n",
		email,
	)
	fmt.Println("All existing refresh tokens for this account have been revoked.")

	return nil
}

func readEmail(reader *bufio.Reader) (string, error) {
	fmt.Print("Enter admin email: ")

	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	return strings.ToLower(strings.TrimSpace(line)), nil
}

func readPassword(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)

	var bytePassword []byte
	var err error

	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		bytePassword, err = term.ReadPassword(fd)
		fmt.Println()
	} else {
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
