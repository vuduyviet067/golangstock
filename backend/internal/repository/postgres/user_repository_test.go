package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"gostock/internal/domain"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestUserRepository_List(t *testing.T) {
	t.Run("success with rows and count", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		now := time.Now().Truncate(time.Second)

		// 1. Expect Count query
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE \(\$1 = '' OR email ILIKE '%' \|\| \$1 \|\| '%'\)`).
			WithArgs("admin").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		// 2. Expect List query
		mock.ExpectQuery(`SELECT id, email, password_hash, role, created_at, updated_at FROM users WHERE \(\$1 = '' OR email ILIKE '%' \|\| \$1 \|\| '%'\) ORDER BY id ASC LIMIT \$2 OFFSET \$3`).
			WithArgs("admin", 10, 0).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "role", "created_at", "updated_at"}).
				AddRow(1, "admin@example.com", "hash1", "admin", now, now))

		users, total, err := repo.List(context.Background(), "admin", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 1 {
			t.Errorf("expected total 1, got %d", total)
		}
		if len(users) != 1 {
			t.Fatalf("expected 1 user, got %d", len(users))
		}
		if users[0].Email != "admin@example.com" {
			t.Errorf("expected email admin@example.com, got %s", users[0].Email)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
			WithArgs("").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		mock.ExpectQuery(`SELECT id, email, password_hash, role, created_at, updated_at FROM users`).
			WithArgs("", 10, 0).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "role", "created_at", "updated_at"}))

		users, total, err := repo.List(context.Background(), "", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 0 {
			t.Errorf("expected total 0, got %d", total)
		}
		if len(users) != 0 {
			t.Errorf("expected 0 users, got %d", len(users))
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("count query error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
			WithArgs("").
			WillReturnError(errors.New("db count connection failed"))

		_, _, err = repo.List(context.Background(), "", 1, 10)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("list query error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
			WithArgs("").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

		mock.ExpectQuery(`SELECT id, email, password_hash, role, created_at, updated_at FROM users`).
			WithArgs("", 10, 0).
			WillReturnError(errors.New("db query error"))

		_, _, err = repo.List(context.Background(), "", 1, 10)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("scan error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
			WithArgs("").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		// Return row with invalid column type for id (string instead of int64)
		mock.ExpectQuery(`SELECT id, email, password_hash, role, created_at, updated_at FROM users`).
			WithArgs("", 10, 0).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "role", "created_at", "updated_at"}).
				AddRow("not-an-int", "email", "hash", "user", time.Now(), time.Now()))

		_, _, err = repo.List(context.Background(), "", 1, 10)
		if err == nil {
			t.Fatal("expected scan error, got nil")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("rows iteration error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		now := time.Now()

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
			WithArgs("").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

		rows := sqlmock.NewRows([]string{"id", "email", "password_hash", "role", "created_at", "updated_at"}).
			AddRow(1, "email1@example.com", "hash", "user", now, now).
			RowError(0, errors.New("stream reading error"))

		mock.ExpectQuery(`SELECT id, email, password_hash, role, created_at, updated_at FROM users`).
			WithArgs("", 10, 0).
			WillReturnRows(rows)

		_, _, err = repo.List(context.Background(), "", 1, 10)
		if err == nil {
			t.Fatal("expected rows error, got nil")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})
}

func TestUserRepository_Update(t *testing.T) {
	t.Run("success updates user and returns updated_at", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		now := time.Now().Truncate(time.Second)

		user := &domain.User{
			ID:    5,
			Email: "Updated@Example.com",
			Role:  "admin",
		}

		mock.ExpectQuery(`UPDATE users SET email = \$1, role = \$2, updated_at = NOW\(\) WHERE id = \$3 RETURNING updated_at`).
			WithArgs("updated@example.com", "admin", int64(5)).
			WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(now))

		err = repo.Update(context.Background(), user)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.UpdatedAt != now {
			t.Errorf("expected updated_at to be set to %v, got %v", now, user.UpdatedAt)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("user not found returns domain.ErrUserNotFound", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		user := &domain.User{
			ID:    99,
			Email: "nonexistent@example.com",
			Role:  "user",
		}

		mock.ExpectQuery(`UPDATE users SET email = \$1, role = \$2, updated_at = NOW\(\) WHERE id = \$3 RETURNING updated_at`).
			WithArgs("nonexistent@example.com", "user", int64(99)).
			WillReturnError(sql.ErrNoRows)

		err = repo.Update(context.Background(), user)
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("expected ErrUserNotFound, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("duplicate email maps to domain.ErrEmailExists", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		user := &domain.User{
			ID:    5,
			Email: "taken@example.com",
			Role:  "user",
		}

		pgErr := &pgconn.PgError{
			Code:           "23505",
			ConstraintName: "users_email_unique_idx",
		}

		mock.ExpectQuery(`UPDATE users SET email = \$1, role = \$2, updated_at = NOW\(\) WHERE id = \$3 RETURNING updated_at`).
			WithArgs("taken@example.com", "user", int64(5)).
			WillReturnError(pgErr)

		err = repo.Update(context.Background(), user)
		if !errors.Is(err, domain.ErrEmailExists) {
			t.Fatalf("expected ErrEmailExists, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("database error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		user := &domain.User{
			ID:    5,
			Email: "any@example.com",
			Role:  "user",
		}

		dbErr := errors.New("connection reset by peer")
		mock.ExpectQuery(`UPDATE users SET email = \$1, role = \$2, updated_at = NOW\(\) WHERE id = \$3 RETURNING updated_at`).
			WithArgs("any@example.com", "user", int64(5)).
			WillReturnError(dbErr)

		err = repo.Update(context.Background(), user)
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected dbErr, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})
}

func TestUserRepository_UpdateAndRevokeTokens(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	revokedAt := time.Now().UTC().Truncate(time.Second)

	t.Run("1. successful user update and token revocation followed by commit", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		user := &domain.User{
			ID:    5,
			Email: "Admin@Example.com",
			Role:  "admin",
		}

		mock.ExpectBegin()
		mock.ExpectQuery(`UPDATE users SET email = \$1, role = \$2, updated_at = NOW\(\) WHERE id = \$3 RETURNING updated_at`).
			WithArgs("admin@example.com", "admin", int64(5)).
			WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(now))
		mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = COALESCE\(revoked_at, \$1\) WHERE user_id = \$2`).
			WithArgs(revokedAt, int64(5)).
			WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectCommit()

		err = repo.UpdateAndRevokeTokens(context.Background(), user, revokedAt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.UpdatedAt != now {
			t.Errorf("expected updated_at to be set to %v, got %v", now, user.UpdatedAt)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("2. user update failure causes rollback", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		user := &domain.User{
			ID:    5,
			Email: "admin@example.com",
			Role:  "admin",
		}

		dbErr := errors.New("fatal query error on user update")
		mock.ExpectBegin()
		mock.ExpectQuery(`UPDATE users SET email = \$1, role = \$2, updated_at = NOW\(\) WHERE id = \$3 RETURNING updated_at`).
			WithArgs("admin@example.com", "admin", int64(5)).
			WillReturnError(dbErr)
		mock.ExpectRollback()

		err = repo.UpdateAndRevokeTokens(context.Background(), user, revokedAt)
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected dbErr, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("3. missing user causes rollback and returns ErrUserNotFound", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		user := &domain.User{
			ID:    99,
			Email: "missing@example.com",
			Role:  "admin",
		}

		mock.ExpectBegin()
		mock.ExpectQuery(`UPDATE users SET email = \$1, role = \$2, updated_at = NOW\(\) WHERE id = \$3 RETURNING updated_at`).
			WithArgs("missing@example.com", "admin", int64(99)).
			WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		err = repo.UpdateAndRevokeTokens(context.Background(), user, revokedAt)
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("expected ErrUserNotFound, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("4. duplicate email causes rollback and returns ErrEmailExists", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		user := &domain.User{
			ID:    5,
			Email: "taken@example.com",
			Role:  "admin",
		}

		pgErr := &pgconn.PgError{
			Code:           "23505",
			ConstraintName: "users_email_unique_idx",
		}

		mock.ExpectBegin()
		mock.ExpectQuery(`UPDATE users SET email = \$1, role = \$2, updated_at = NOW\(\) WHERE id = \$3 RETURNING updated_at`).
			WithArgs("taken@example.com", "admin", int64(5)).
			WillReturnError(pgErr)
		mock.ExpectRollback()

		err = repo.UpdateAndRevokeTokens(context.Background(), user, revokedAt)
		if !errors.Is(err, domain.ErrEmailExists) {
			t.Fatalf("expected ErrEmailExists, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("5. refresh-token revocation failure causes rollback", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		user := &domain.User{
			ID:    5,
			Email: "admin@example.com",
			Role:  "admin",
		}

		revokeErr := errors.New("refresh token table locked")
		mock.ExpectBegin()
		mock.ExpectQuery(`UPDATE users SET email = \$1, role = \$2, updated_at = NOW\(\) WHERE id = \$3 RETURNING updated_at`).
			WithArgs("admin@example.com", "admin", int64(5)).
			WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(now))
		mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = COALESCE\(revoked_at, \$1\) WHERE user_id = \$2`).
			WithArgs(revokedAt, int64(5)).
			WillReturnError(revokeErr)
		mock.ExpectRollback()

		err = repo.UpdateAndRevokeTokens(context.Background(), user, revokedAt)
		if !errors.Is(err, revokeErr) {
			t.Fatalf("expected revokeErr, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("6. commit failure is returned", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		user := &domain.User{
			ID:    5,
			Email: "admin@example.com",
			Role:  "admin",
		}

		commitErr := errors.New("serialization failure on commit")
		mock.ExpectBegin()
		mock.ExpectQuery(`UPDATE users SET email = \$1, role = \$2, updated_at = NOW\(\) WHERE id = \$3 RETURNING updated_at`).
			WithArgs("admin@example.com", "admin", int64(5)).
			WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(now))
		mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = COALESCE\(revoked_at, \$1\) WHERE user_id = \$2`).
			WithArgs(revokedAt, int64(5)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit().WillReturnError(commitErr)

		err = repo.UpdateAndRevokeTokens(context.Background(), user, revokedAt)
		if !errors.Is(err, commitErr) {
			t.Fatalf("expected commitErr, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})

	t.Run("7. existing revoked tokens retain their original revoked_at because of COALESCE", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		repo := NewUserRepository(db)
		user := &domain.User{
			ID:    5,
			Email: "admin@example.com",
			Role:  "admin",
		}

		// Verified by the exact SQL assertion using COALESCE(revoked_at, $1)
		mock.ExpectBegin()
		mock.ExpectQuery(`UPDATE users SET email = \$1, role = \$2, updated_at = NOW\(\) WHERE id = \$3 RETURNING updated_at`).
			WithArgs("admin@example.com", "admin", int64(5)).
			WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(now))
		mock.ExpectExec(`UPDATE refresh_tokens SET revoked_at = COALESCE\(revoked_at, \$1\) WHERE user_id = \$2`).
			WithArgs(revokedAt, int64(5)).
			WillReturnResult(sqlmock.NewResult(0, 4))
		mock.ExpectCommit()

		err = repo.UpdateAndRevokeTokens(context.Background(), user, revokedAt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sql expectations: %v", err)
		}
	})
}
