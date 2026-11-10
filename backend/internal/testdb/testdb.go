package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const EnvVar = "TEST_DATABASE_URL"

const RequiredSuffix = "_test"

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
	if len(name) <= len(RequiredSuffix) || !strings.HasSuffix(name, RequiredSuffix) {
		return fmt.Errorf("connected database %q does not end with %q; point %s at a dedicated test database",
			name, RequiredSuffix, EnvVar)
	}
	return nil
}
