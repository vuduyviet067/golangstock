package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"gostock/internal/domain"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetByTokenHashNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create mock database: %v", err)
	}
	defer db.Close()

	repo := NewSessionRepository(db)

	tokenHash := "non-existent-token-hash"

	mock.ExpectQuery("SELECT id, user_id").
		WithArgs(tokenHash).
		WillReturnError(sql.ErrNoRows)

	session, err := repo.GetByTokenHash(
		context.Background(),
		tokenHash,
	)

	if session != nil {
		t.Errorf("expected nil session, got: %+v", session)
	}

	if !errors.Is(err, domain.ErrSessionNotFound) {
		t.Errorf(
			"expected ErrSessionNotFound, got: %v",
			err,
		)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet database expectations: %v", err)
	}
}
