package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const EnvVar = "TEST_DATABASE_URL"

// RequireEnvVar makes the PostgreSQL integration tests mandatory. CI sets it
// to true: a missing TEST_DATABASE_URL then fails the test instead of
// skipping it, so a run without a database cannot pass. Unset (local
// development), a missing TEST_DATABASE_URL still skips. A value that is not
// a boolean fails, so a typo cannot turn the requirement off.
const RequireEnvVar = "REQUIRE_TEST_DATABASE"

const RequiredName = "gostock_test"

func Open(t testing.TB) *sql.DB {
	t.Helper()

	required, err := Required()
	if err != nil {
		t.Fatalf("%v", err)
	}

	dsn := os.Getenv(EnvVar)
	if dsn == "" {
		if required {
			t.Fatalf("%s is not set, but %s requires the PostgreSQL integration tests to run", EnvVar, RequireEnvVar)
		}
		t.Skipf("%s is not set; skipping PostgreSQL integration test", EnvVar)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open test database from %s: %v", EnvVar, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var name string
	if err := db.QueryRowContext(ctx, "SELECT current_database()").Scan(&name); err != nil {
		t.Fatalf("query current_database() on the database in %s: %v", EnvVar, err)
	}
	if err := CheckName(name); err != nil {
		t.Fatalf("refusing to run integration test: %v", err)
	}
	return db
}

// Required reports whether RequireEnvVar makes the integration tests
// mandatory: false when it is unset or empty, its value when it is a boolean
// (strconv.ParseBool), and an error otherwise.
func Required() (bool, error) {
	value := strings.TrimSpace(os.Getenv(RequireEnvVar))
	if value == "" {
		return false, nil
	}
	required, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s=%q is not a boolean (use true or false)", RequireEnvVar, value)
	}
	return required, nil
}

func CheckName(name string) error {
	if name != RequiredName {
		return fmt.Errorf("connected database %q is not %q; point %s at the dedicated integration test database",
			name, RequiredName, EnvVar)
	}
	return nil
}
