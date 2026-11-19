package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"gostock/internal/repository"
)

// pgForeignKeyViolation is the SQLSTATE of a foreign key violation.
const pgForeignKeyViolation = "23503"

// mapDeleteInUse turns the foreign key violation of a DELETE into
// repository.ErrInUse: another row still references the one being deleted.
// Any other error is returned unchanged.
func mapDeleteInUse(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
		return repository.ErrInUse
	}
	return err
}
