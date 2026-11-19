package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// VerifyDatabaseName checks, on the pool the caller is about to use, that
// current_database() is exactly want. A mismatch names both databases. A
// failed query is reported by preflightQueryError, which never exposes the
// connection string, user or host.
func VerifyDatabaseName(ctx context.Context, db *sql.DB, want string) error {
	var got string
	if err := db.QueryRowContext(ctx, "SELECT current_database()").Scan(&got); err != nil {
		return preflightQueryError(ctx, err)
	}
	if got != want {
		return fmt.Errorf("database preflight: connected to %q, expected %q", got, want)
	}
	return nil
}

// preflightQueryError describes a failed preflight query without the driver's
// own error text, which can contain connection settings:
//   - a cancelled or expired context is returned wrapped;
//   - a server error shows its SQLSTATE, and its message only outside
//     classes 28 (invalid authorization) and 08 (connection exception),
//     whose messages can name the user or database;
//   - anything else gets a generic message.
func preflightQueryError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("database preflight: %w", ctxErr)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if hidesServerMessage(pgErr.Code) {
			return fmt.Errorf("database preflight: query current_database(): server error %s (message hidden: it can name the user or database)", pgErr.Code)
		}
		return fmt.Errorf("database preflight: query current_database(): server error %s: %s", pgErr.Code, pgErr.Message)
	}
	return errors.New("database preflight: query current_database() failed (details hidden to avoid exposing connection settings)")
}

// hidesServerMessage reports whether a SQLSTATE belongs to class 28 (invalid
// authorization specification) or 08 (connection exception).
func hidesServerMessage(code string) bool {
	return strings.HasPrefix(code, "28") || strings.HasPrefix(code, "08")
}

func OpenPostgres(dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf(
			"DATABASE_URL environment variable is required",
		)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to open database: %w", err,
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf(
			"failed to connect to PostgreSQL: %w", err,
		)
	}

	return db, nil
}
