package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gostock/internal/domain"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestRefreshTokenRepository_CreateSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().Truncate(time.Second)
	token := &domain.RefreshToken{
		UserID:    10,
		TokenHash: "hash-123",
		ExpiresAt: now.Add(7 * 24 * time.Hour),
		CreatedAt: now,
	}

	mock.ExpectQuery(`INSERT INTO refresh_tokens`).
		WithArgs(token.UserID, token.TokenHash, token.ExpiresAt, token.CreatedAt, token.RevokedAt, token.ReplacedByHash).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err = repo.Create(context.Background(), token)
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	if token.ID != 1 {
		t.Errorf("token.ID = %d, want 1", token.ID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_GetByTokenHashSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().Truncate(time.Second)
	expiresAt := now.Add(24 * time.Hour)

	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1`).
		WithArgs("valid-hash").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
		}).AddRow(1, 10, "valid-hash", expiresAt, now, nil, nil))

	token, err := repo.GetByTokenHash(context.Background(), "valid-hash")
	if err != nil {
		t.Fatalf("GetByTokenHash() unexpected error: %v", err)
	}

	if token == nil {
		t.Fatal("expected token, got nil")
	}

	if token.ID != 1 || token.UserID != 10 || token.TokenHash != "valid-hash" {
		t.Errorf("token data mismatch: %+v", token)
	}

	if token.IsRevoked() {
		t.Error("expected token to not be revoked")
	}

	if token.IsReplaced() {
		t.Error("expected token to not be replaced")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_GetByTokenHashNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)

	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1`).
		WithArgs("missing-hash").
		WillReturnError(sql.ErrNoRows)

	token, err := repo.GetByTokenHash(context.Background(), "missing-hash")
	if !errors.Is(err, domain.ErrRefreshTokenNotFound) {
		t.Fatalf("GetByTokenHash() error = %v, want ErrRefreshTokenNotFound", err)
	}
	if token != nil {
		t.Errorf("expected nil token, got %+v", token)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RotateSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	oldExpiry := now.Add(24 * time.Hour)
	newExpiry := now.Add(7 * 24 * time.Hour)

	oldHash := "old-hash"
	newToken := &domain.RefreshToken{
		TokenHash: "new-hash",
		ExpiresAt: newExpiry,
		CreatedAt: now,
	}

	mock.ExpectBegin()
	// Lock old token
	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
		WithArgs(oldHash).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
		}).AddRow(1, 10, oldHash, oldExpiry, now.Add(-time.Hour), nil, nil))

	// Update old token
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \$1, replaced_by_hash = \$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), newToken.TokenHash, 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Insert new token
	mock.ExpectQuery(`INSERT INTO refresh_tokens`).
		WithArgs(10, newToken.TokenHash, newToken.ExpiresAt, newToken.CreatedAt, (*time.Time)(nil), (*string)(nil)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))

	mock.ExpectCommit()

	err = repo.Rotate(context.Background(), oldHash, newToken)
	if err != nil {
		t.Fatalf("Rotate() unexpected error: %v", err)
	}

	if newToken.ID != 2 {
		t.Errorf("newToken.ID = %d, want 2", newToken.ID)
	}
	if newToken.UserID != 10 {
		t.Errorf("newToken.UserID = %d, want 10", newToken.UserID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RotateNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
		WithArgs("not-found-hash").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	newToken := &domain.RefreshToken{TokenHash: "new-hash"}
	err = repo.Rotate(context.Background(), "not-found-hash", newToken)
	if !errors.Is(err, domain.ErrRefreshTokenNotFound) {
		t.Fatalf("Rotate() error = %v, want ErrRefreshTokenNotFound", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RotateReused(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().Truncate(time.Second)

	mock.ExpectBegin()
	// Old token already has replaced_by_hash set
	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
		WithArgs("reused-hash").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
		}).AddRow(1, 10, "reused-hash", now.Add(time.Hour), now.Add(-time.Hour), now, "earlier-replacement-hash"))
	mock.ExpectRollback()

	newToken := &domain.RefreshToken{TokenHash: "new-hash"}
	err = repo.Rotate(context.Background(), "reused-hash", newToken)
	if !errors.Is(err, domain.ErrRefreshTokenReused) {
		t.Fatalf("Rotate() error = %v, want ErrRefreshTokenReused", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RotateRevoked(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().Truncate(time.Second)

	mock.ExpectBegin()
	// Old token has revoked_at set (without replacement, e.g. from logout)
	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
		WithArgs("revoked-hash").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
		}).AddRow(1, 10, "revoked-hash", now.Add(time.Hour), now.Add(-time.Hour), now, nil))
	mock.ExpectRollback()

	newToken := &domain.RefreshToken{TokenHash: "new-hash"}
	err = repo.Rotate(context.Background(), "revoked-hash", newToken)
	if !errors.Is(err, domain.ErrRefreshTokenRevoked) {
		t.Fatalf("Rotate() error = %v, want ErrRefreshTokenRevoked", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RotateUserIDMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	oldExpiry := now.Add(24 * time.Hour)
	newExpiry := now.Add(7 * 24 * time.Hour)

	oldHash := "old-hash"
	newToken := &domain.RefreshToken{
		UserID:    99, // Mismatched UserID
		TokenHash: "new-hash",
		ExpiresAt: newExpiry,
		CreatedAt: now,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
		WithArgs(oldHash).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
		}).AddRow(1, 10, oldHash, oldExpiry, now.Add(-time.Hour), nil, nil))
	mock.ExpectRollback()

	err = repo.Rotate(context.Background(), oldHash, newToken)
	if !errors.Is(err, domain.ErrTokenOwnerMismatch) {
		t.Fatalf("Rotate() error = %v, want ErrTokenOwnerMismatch", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RotateUserIDExplicitMatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	oldExpiry := now.Add(24 * time.Hour)
	newExpiry := now.Add(7 * 24 * time.Hour)

	oldHash := "old-hash"
	newToken := &domain.RefreshToken{
		UserID:    10, // Explicitly matches old token UserID
		TokenHash: "new-hash",
		ExpiresAt: newExpiry,
		CreatedAt: now,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
		WithArgs(oldHash).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
		}).AddRow(1, 10, oldHash, oldExpiry, now.Add(-time.Hour), nil, nil))

	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \$1, replaced_by_hash = \$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), newToken.TokenHash, 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(`INSERT INTO refresh_tokens`).
		WithArgs(10, newToken.TokenHash, newToken.ExpiresAt, newToken.CreatedAt, (*time.Time)(nil), (*string)(nil)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))

	mock.ExpectCommit()

	err = repo.Rotate(context.Background(), oldHash, newToken)
	if err != nil {
		t.Fatalf("Rotate() unexpected error: %v", err)
	}

	if newToken.UserID != 10 {
		t.Errorf("newToken.UserID = %d, want 10", newToken.UserID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RotateInvalidInput(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	oldExpiry := now.Add(24 * time.Hour)
	oldHash := "old-hash"

	t.Run("empty or whitespace old token hash does not start transaction", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewRefreshTokenRepository(db)
		newToken := &domain.RefreshToken{
			TokenHash: "new-hash",
			ExpiresAt: now.Add(24 * time.Hour),
		}

		for _, badOldHash := range []string{"", "   ", "\t\n"} {
			err := repo.Rotate(context.Background(), badOldHash, newToken)
			if !errors.Is(err, domain.ErrInvalidRefreshToken) {
				t.Fatalf("Rotate(%q) error = %v, want ErrInvalidRefreshToken", badOldHash, err)
			}
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations (no tx should have started): %v", err)
		}
	})

	t.Run("self-replacement matching old token hash does not start transaction", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewRefreshTokenRepository(db)
		newToken := &domain.RefreshToken{
			TokenHash: oldHash, // Exact same hash as oldTokenHash
			ExpiresAt: now.Add(24 * time.Hour),
		}

		err = repo.Rotate(context.Background(), oldHash, newToken)
		if !errors.Is(err, domain.ErrInvalidRefreshToken) {
			t.Fatalf("Rotate(self-replacement) error = %v, want ErrInvalidRefreshToken", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations (no tx should have started): %v", err)
		}
	})

	t.Run("nil new token", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewRefreshTokenRepository(db)
		err = repo.Rotate(context.Background(), oldHash, nil)
		if !errors.Is(err, domain.ErrInvalidRefreshToken) {
			t.Fatalf("Rotate(nil) error = %v, want ErrInvalidRefreshToken", err)
		}
	})

	t.Run("empty new token hash", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewRefreshTokenRepository(db)
		newToken := &domain.RefreshToken{
			TokenHash: "",
			ExpiresAt: now.Add(24 * time.Hour),
			CreatedAt: now,
		}
		err = repo.Rotate(context.Background(), oldHash, newToken)
		if !errors.Is(err, domain.ErrInvalidRefreshToken) {
			t.Fatalf("Rotate(empty hash) error = %v, want ErrInvalidRefreshToken", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("zero new token expiry", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewRefreshTokenRepository(db)
		mock.ExpectBegin()

		mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
			WithArgs(oldHash).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
			}).AddRow(1, 10, oldHash, oldExpiry, now.Add(-time.Hour), nil, nil))

		mock.ExpectRollback()
		newToken := &domain.RefreshToken{
			TokenHash: "valid-hash",
			CreatedAt: now,
		}
		err = repo.Rotate(context.Background(), oldHash, newToken)
		if !errors.Is(err, domain.ErrInvalidRefreshToken) {
			t.Fatalf("Rotate(zero expiry) error = %v, want ErrInvalidRefreshToken", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("past new token expiry", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewRefreshTokenRepository(db)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
			WithArgs(oldHash).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
			}).AddRow(1, 10, oldHash, oldExpiry, now.Add(-time.Hour), nil, nil))
		mock.ExpectRollback()

		newToken := &domain.RefreshToken{
			TokenHash: "valid-hash",
			ExpiresAt: now.Add(-time.Hour),
			CreatedAt: now,
		}
		err = repo.Rotate(context.Background(), oldHash, newToken)
		if !errors.Is(err, domain.ErrInvalidRefreshToken) {
			t.Fatalf("Rotate(past expiry) error = %v, want ErrInvalidRefreshToken", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})
}

func TestRefreshTokenRepository_RotateExpired(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().Truncate(time.Second)
	expiredAt := now.Add(-10 * time.Minute)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
		WithArgs("expired-hash").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
		}).AddRow(1, 10, "expired-hash", expiredAt, now.Add(-time.Hour), nil, nil))
	mock.ExpectRollback()

	newToken := &domain.RefreshToken{TokenHash: "new-hash", CreatedAt: now}
	err = repo.Rotate(context.Background(), "expired-hash", newToken)
	if !errors.Is(err, domain.ErrRefreshTokenExpired) {
		t.Fatalf("Rotate() error = %v, want ErrRefreshTokenExpired", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RotateExpired_CallerSuppliedCreatedAtCannotBypassExpiry(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	// Old token expired 5 minutes ago relative to true now
	expiredAt := now.Add(-5 * time.Minute)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
		WithArgs("expired-hash").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
		}).AddRow(1, 10, "expired-hash", expiredAt, now.Add(-24*time.Hour), nil, nil))
	mock.ExpectRollback()

	// Caller attempts to pass a CreatedAt in the past (e.g. 2 hours ago, before expiredAt)
	// to make the expired old token appear unexpired.
	newToken := &domain.RefreshToken{
		TokenHash: "new-hash",
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now.Add(-2 * time.Hour),
	}
	err = repo.Rotate(context.Background(), "expired-hash", newToken)
	if !errors.Is(err, domain.ErrRefreshTokenExpired) {
		t.Fatalf("Rotate() error = %v, want ErrRefreshTokenExpired", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RotateRollbackOnInsertFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	oldExpiry := now.Add(24 * time.Hour)
	newExpiry := now.Add(7 * 24 * time.Hour)

	oldHash := "old-hash"
	newToken := &domain.RefreshToken{
		TokenHash: "new-hash",
		ExpiresAt: newExpiry,
		CreatedAt: now,
	}

	mock.ExpectBegin()
	// Lock old token
	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
		WithArgs(oldHash).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
		}).AddRow(1, 10, oldHash, oldExpiry, now.Add(-time.Hour), nil, nil))

	// Update old token
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \$1, replaced_by_hash = \$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), newToken.TokenHash, 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Insert fails (e.g. disk failure or constraint error)
	insertErr := errors.New("db error inserting new token")
	mock.ExpectQuery(`INSERT INTO refresh_tokens`).
		WithArgs(10, newToken.TokenHash, newToken.ExpiresAt, newToken.CreatedAt, (*time.Time)(nil), (*string)(nil)).
		WillReturnError(insertErr)

	// Rollback must be executed
	mock.ExpectRollback()

	err = repo.Rotate(context.Background(), oldHash, newToken)
	if !errors.Is(err, insertErr) {
		t.Fatalf("Rotate() error = %v, want %v", err, insertErr)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RotateAbnormalRowsAffected(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	oldExpiry := now.Add(24 * time.Hour)
	newExpiry := now.Add(7 * 24 * time.Hour)

	oldHash := "old-hash"
	newToken := &domain.RefreshToken{
		TokenHash: "new-hash",
		ExpiresAt: newExpiry,
		CreatedAt: now,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
		WithArgs(oldHash).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
		}).AddRow(1, 10, oldHash, oldExpiry, now.Add(-time.Hour), nil, nil))

	// Update returns 0 rows affected
	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \$1, replaced_by_hash = \$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), newToken.TokenHash, 1).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectRollback()

	err = repo.Rotate(context.Background(), oldHash, newToken)
	if err == nil {
		t.Fatal("expected error on abnormal rows affected (0), got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

// CR-T05: Database errors and transaction rollbacks during Rotate
func TestRefreshTokenRepository_Rotate_BeginTxError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	beginErr := errors.New("cannot begin transaction: pool exhausted")
	mock.ExpectBegin().WillReturnError(beginErr)

	newToken := &domain.RefreshToken{
		TokenHash: "new-hash",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	err = repo.Rotate(context.Background(), "old-hash", newToken)
	if !errors.Is(err, beginErr) {
		t.Fatalf("Rotate() error = %v, want %v", err, beginErr)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_Rotate_SelectForUpdateDBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	queryErr := errors.New("disk i/o failure during row lock")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
		WithArgs("old-hash").
		WillReturnError(queryErr)
	mock.ExpectRollback()

	newToken := &domain.RefreshToken{
		TokenHash: "new-hash",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	err = repo.Rotate(context.Background(), "old-hash", newToken)
	if !errors.Is(err, queryErr) {
		t.Fatalf("Rotate() error = %v, want %v", err, queryErr)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_Rotate_UpdateOldTokenDBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	oldExpiry := now.Add(24 * time.Hour)
	newExpiry := now.Add(7 * 24 * time.Hour)

	oldHash := "old-hash"
	newToken := &domain.RefreshToken{
		TokenHash: "new-hash",
		ExpiresAt: newExpiry,
		CreatedAt: now,
	}

	updateErr := errors.New("deadlock or timeout during update")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
		WithArgs(oldHash).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
		}).AddRow(1, 10, oldHash, oldExpiry, now.Add(-time.Hour), nil, nil))

	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \$1, replaced_by_hash = \$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), newToken.TokenHash, 1).
		WillReturnError(updateErr)
	mock.ExpectRollback()

	err = repo.Rotate(context.Background(), oldHash, newToken)
	if !errors.Is(err, updateErr) {
		t.Fatalf("Rotate() error = %v, want %v", err, updateErr)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_Rotate_ForeignKeyViolationMapped(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	oldExpiry := now.Add(24 * time.Hour)
	newExpiry := now.Add(7 * 24 * time.Hour)

	oldHash := "old-hash"
	newToken := &domain.RefreshToken{
		TokenHash: "new-hash",
		ExpiresAt: newExpiry,
		CreatedAt: now,
	}

	pgErr := &pgconn.PgError{
		Code:           "23503",
		ConstraintName: "refresh_tokens_user_id_fkey",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at, replaced_by_hash FROM refresh_tokens WHERE token_hash = \$1 FOR UPDATE`).
		WithArgs(oldHash).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "token_hash", "expires_at", "created_at", "revoked_at", "replaced_by_hash",
		}).AddRow(1, 10, oldHash, oldExpiry, now.Add(-time.Hour), nil, nil))

	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = \$1, replaced_by_hash = \$2 WHERE id = \$3`).
		WithArgs(sqlmock.AnyArg(), newToken.TokenHash, 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(`INSERT INTO refresh_tokens`).
		WithArgs(10, newToken.TokenHash, newToken.ExpiresAt, newToken.CreatedAt, (*time.Time)(nil), (*string)(nil)).
		WillReturnError(pgErr)
	mock.ExpectRollback()

	err = repo.Rotate(context.Background(), oldHash, newToken)
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("Rotate() error = %v, want ErrUserNotFound", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_Create_ForeignKeyViolationMapped(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().Truncate(time.Second)
	token := &domain.RefreshToken{
		UserID:    999,
		TokenHash: "hash-orphan",
		ExpiresAt: now.Add(24 * time.Hour),
		CreatedAt: now,
	}

	pgErr := &pgconn.PgError{
		Code:           "23503",
		ConstraintName: "refresh_tokens_user_id_fkey",
	}

	mock.ExpectQuery(`INSERT INTO refresh_tokens`).
		WithArgs(token.UserID, token.TokenHash, token.ExpiresAt, token.CreatedAt, token.RevokedAt, token.ReplacedByHash).
		WillReturnError(pgErr)

	err = repo.Create(context.Background(), token)
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("Create() error = %v, want ErrUserNotFound", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_Revoke_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().Truncate(time.Second)
	dbErr := errors.New("db connection lost")

	mock.ExpectQuery(`UPDATE refresh_tokens SET revoked_at = COALESCE\(revoked_at, \$1\) WHERE token_hash = \$2 RETURNING id`).
		WithArgs(now, "target-hash").
		WillReturnError(dbErr)

	err = repo.Revoke(context.Background(), "target-hash", now)
	if !errors.Is(err, dbErr) {
		t.Fatalf("Revoke() error = %v, want %v", err, dbErr)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RevokeAllForUser_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().Truncate(time.Second)
	dbErr := errors.New("db connection lost")

	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = COALESCE\(revoked_at, \$1\) WHERE user_id = \$2`).
		WithArgs(now, 10).
		WillReturnError(dbErr)

	err = repo.RevokeAllForUser(context.Background(), 10, now)
	if !errors.Is(err, dbErr) {
		t.Fatalf("RevokeAllForUser() error = %v, want %v", err, dbErr)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RevokeSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().Truncate(time.Second)

	mock.ExpectQuery(`UPDATE refresh_tokens SET revoked_at = COALESCE\(revoked_at, \$1\) WHERE token_hash = \$2 RETURNING id`).
		WithArgs(now, "target-hash").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err = repo.Revoke(context.Background(), "target-hash", now)
	if err != nil {
		t.Fatalf("Revoke() unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RevokeNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().Truncate(time.Second)

	mock.ExpectQuery(`UPDATE refresh_tokens SET revoked_at = COALESCE\(revoked_at, \$1\) WHERE token_hash = \$2 RETURNING id`).
		WithArgs(now, "missing-hash").
		WillReturnError(sql.ErrNoRows)

	err = repo.Revoke(context.Background(), "missing-hash", now)
	if !errors.Is(err, domain.ErrRefreshTokenNotFound) {
		t.Fatalf("Revoke() error = %v, want ErrRefreshTokenNotFound", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestRefreshTokenRepository_RevokeAllForUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	now := time.Now().Truncate(time.Second)

	mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = COALESCE\(revoked_at, \$1\) WHERE user_id = \$2`).
		WithArgs(now, 10).
		WillReturnResult(sqlmock.NewResult(0, 3))

	err = repo.RevokeAllForUser(context.Background(), 10, now)
	if err != nil {
		t.Fatalf("RevokeAllForUser() unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

// Integration test against real PostgreSQL when DATABASE_URL is set
func TestRefreshTokenRepository_IntegrationConcurrentRotation(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set; skipping live Postgres integration concurrency test")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}

	// 1. Create a temporary user for testing
	testEmail := fmt.Sprintf("test-token-%d@example.com", time.Now().UnixNano())
	var userID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, role)
		VALUES ($1, 'hash', 'user')
		RETURNING id
	`, testEmail).Scan(&userID)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM refresh_tokens WHERE user_id = $1`, userID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	}()

	repo := NewRefreshTokenRepository(db)
	initialHash := fmt.Sprintf("initial-token-hash-%d", time.Now().UnixNano())
	now := time.Now().Truncate(time.Second)

	initialToken := &domain.RefreshToken{
		UserID:    userID,
		TokenHash: initialHash,
		ExpiresAt: now.Add(7 * 24 * time.Hour),
		CreatedAt: now,
	}
	if err := repo.Create(ctx, initialToken); err != nil {
		t.Fatalf("create initial token: %v", err)
	}

	// 2. Launch 2 concurrent goroutines attempting to rotate the exact same initialToken
	const concurrentAttempts = 2
	start := make(chan struct{})
	errCh := make(chan error, concurrentAttempts)
	var wg sync.WaitGroup

	for i := 0; i < concurrentAttempts; i++ {
		wg.Add(1)
		index := i
		go func() {
			defer wg.Done()
			<-start

			newToken := &domain.RefreshToken{
				UserID:    userID,
				TokenHash: fmt.Sprintf("new-token-hash-%d-%d", time.Now().UnixNano(), index),
				ExpiresAt: now.Add(7 * 24 * time.Hour),
				CreatedAt: time.Now(),
			}

			errCh <- repo.Rotate(context.Background(), initialHash, newToken)
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	successCount := 0
	reusedCount := 0
	for err := range errCh {
		if err == nil {
			successCount++
		} else if errors.Is(err, domain.ErrRefreshTokenReused) {
			reusedCount++
		} else {
			t.Errorf("unexpected error from concurrent Rotate: %v", err)
		}
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 rotation success, got %d", successCount)
	}
	if reusedCount != 1 {
		t.Errorf("expected exactly 1 rotation rejected with ErrRefreshTokenReused, got %d", reusedCount)
	}

	// 3. Verify exactly 1 active new token in database
	var activeCount int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM refresh_tokens
		WHERE user_id = $1 AND revoked_at IS NULL AND replaced_by_hash IS NULL
	`, userID).Scan(&activeCount)
	if err != nil {
		t.Fatalf("count active tokens: %v", err)
	}

	if activeCount != 1 {
		t.Errorf("expected exactly 1 active new token in database, got %d", activeCount)
	}

	// 4. Verify initial token was revoked and replaced by winning token
	var initialRevokedAt sql.NullTime
	var initialReplacedByHash sql.NullString
	err = db.QueryRowContext(ctx, `
		SELECT revoked_at, replaced_by_hash
		FROM refresh_tokens
		WHERE token_hash = $1
	`, initialHash).Scan(&initialRevokedAt, &initialReplacedByHash)
	if err != nil {
		t.Fatalf("fetch initial token status: %v", err)
	}

	if !initialRevokedAt.Valid {
		t.Error("expected initial token to be revoked (revoked_at IS NOT NULL)")
	}
	if !initialReplacedByHash.Valid || initialReplacedByHash.String == "" {
		t.Fatal("expected initial token to have replaced_by_hash set")
	}

	// 5. Verify the replacement hash matches the single active winning token
	var winningTokenHash string
	err = db.QueryRowContext(ctx, `
		SELECT token_hash FROM refresh_tokens
		WHERE user_id = $1 AND revoked_at IS NULL AND replaced_by_hash IS NULL
	`, userID).Scan(&winningTokenHash)
	if err != nil {
		t.Fatalf("fetch winning token hash: %v", err)
	}

	if initialReplacedByHash.String != winningTokenHash {
		t.Errorf("initial token replaced_by_hash %q != active winning token hash %q", initialReplacedByHash.String, winningTokenHash)
	}
}
