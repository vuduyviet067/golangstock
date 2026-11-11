package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"gostock/internal/testdb"
)

// These tests run against the integration test database: testdb.Open only
// returns a pool connected to exactly gostock_test.

func TestVerifyDatabaseName(t *testing.T) {
	db := testdb.Open(t)
	dsn := os.Getenv(testdb.EnvVar)

	t.Run("matching name passes", func(t *testing.T) {
		if err := VerifyDatabaseName(context.Background(), db, testdb.RequiredName); err != nil {
			t.Fatalf("VerifyDatabaseName(%q) = %v, want nil", testdb.RequiredName, err)
		}
	})

	t.Run("other name fails and names both databases", func(t *testing.T) {
		err := VerifyDatabaseName(context.Background(), db, "gostock_ui_test")
		if err == nil {
			t.Fatal("VerifyDatabaseName(\"gostock_ui_test\") = nil, want an error")
		}
		want := `connected to "gostock_test", expected "gostock_ui_test"`
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
		if strings.Contains(err.Error(), dsn) {
			t.Error("error contains the connection string")
		}
	})

	t.Run("names are compared exactly", func(t *testing.T) {
		for _, want := range []string{"GOSTOCK_TEST", "gostock_test ", "gostock"} {
			if err := VerifyDatabaseName(context.Background(), db, want); err == nil {
				t.Errorf("VerifyDatabaseName(%q) = nil, want an error", want)
			}
		}
	})

	t.Run("cancelled context fails", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := VerifyDatabaseName(ctx, db, testdb.RequiredName)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	})
}

func TestVerifyDatabaseName_ClosedPoolHidesDetails(t *testing.T) {
	testdb.Open(t) // checks the database before a separate pool is opened
	dsn := os.Getenv(testdb.EnvVar)

	db, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	err = VerifyDatabaseName(context.Background(), db, testdb.RequiredName)
	if err == nil {
		t.Fatal("VerifyDatabaseName on a closed pool = nil, want an error")
	}
	if !strings.Contains(err.Error(), "details hidden") {
		t.Errorf("error = %q, want the generic message", err)
	}
	if strings.Contains(err.Error(), dsn) {
		t.Error("error contains the connection string")
	}
}

// TestPreflightQueryError_HidesConnectionDetails builds the errors a failed
// preflight query can return, wrapped the way the driver wraps connection
// failures, and checks that no user, database or server message from
// classes 28 and 08 reaches the returned error.
func TestPreflightQueryError_HidesConnectionDetails(t *testing.T) {
	const secretUser = "secret_ui_user"
	// wrap mimics pgconn.ConnectError, whose text names the user and database.
	wrap := func(err error) error {
		return fmt.Errorf("failed to connect to `user=%s database=gostock_ui_test`: %w", secretUser, err)
	}

	hidden := []*pgconn.PgError{
		{Code: "28P01", Message: `password authentication failed for user "` + secretUser + `"`},
		{Code: "28000", Message: `role "` + secretUser + `" is not permitted to log in`},
		{Code: "08006", Message: `connection to user "` + secretUser + `" failed`},
		{Code: "08004", Message: `server rejected the connection for "` + secretUser + `"`},
	}
	for _, pgErr := range hidden {
		t.Run("class "+pgErr.Code[:2]+" "+pgErr.Code, func(t *testing.T) {
			err := preflightQueryError(context.Background(), wrap(pgErr))
			msg := err.Error()
			if !strings.Contains(msg, pgErr.Code) {
				t.Errorf("error = %q, want it to contain the SQLSTATE %s", msg, pgErr.Code)
			}
			if strings.Contains(msg, pgErr.Message) || strings.Contains(msg, secretUser) {
				t.Errorf("error = %q exposes the server message or the user", msg)
			}
			if strings.Contains(msg, "gostock_ui_test") {
				t.Errorf("error = %q exposes the database from the connection settings", msg)
			}
		})
	}

	t.Run("other classes keep the server message", func(t *testing.T) {
		pgErr := &pgconn.PgError{Code: "57014", Message: "canceling statement due to statement timeout"}
		msg := preflightQueryError(context.Background(), wrap(pgErr)).Error()
		if !strings.Contains(msg, "57014") || !strings.Contains(msg, pgErr.Message) {
			t.Errorf("error = %q, want SQLSTATE and message", msg)
		}
		if strings.Contains(msg, secretUser) {
			t.Errorf("error = %q exposes the user from the wrapping error", msg)
		}
	})

	t.Run("non-server error gets the generic message", func(t *testing.T) {
		msg := preflightQueryError(context.Background(), wrap(errors.New("dial tcp: connection refused"))).Error()
		if !strings.Contains(msg, "details hidden") || strings.Contains(msg, secretUser) || strings.Contains(msg, "dial tcp") {
			t.Errorf("error = %q, want only the generic message", msg)
		}
	})

	t.Run("cancelled context is reported as such", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := preflightQueryError(ctx, wrap(&pgconn.PgError{Code: "28P01", Message: secretUser}))
		if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), secretUser) {
			t.Errorf("error = %v, want context.Canceled without the user", err)
		}
	})
}
