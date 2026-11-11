package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const EnvVar = "TEST_DATABASE_URL"

// RequiredName is the only database integration tests may use. An exact name,
// not just a "_test" suffix, keeps them off every other database, including
// gostock_ui_test, which is reserved for manual UI testing.
const RequiredName = "gostock_test"

func Open(t testing.TB) *sql.DB {
	t.Helper()

	dsn := os.Getenv(EnvVar)
	if dsn == "" {
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

func CheckName(name string) error {
	if name != RequiredName {
		return fmt.Errorf("connected database %q is not %q; point %s at the dedicated integration test database",
			name, RequiredName, EnvVar)
	}
	return nil
}
