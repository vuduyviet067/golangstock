package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"gostock/internal/domain"
	"gostock/internal/repository"
	"gostock/internal/testdb"
)

const (
	ownLockSQL   = `SELECT id\s+FROM users\s+WHERE id = \$1\s+FOR UPDATE`
	ownUpdateSQL = `UPDATE users\s+SET password_hash = \$1,\s+updated_at = NOW\(\)\s+WHERE id = \$2`
	ownRevokeSQL = `UPDATE refresh_tokens\s+SET revoked_at = \$1\s+WHERE user_id = \$2\s+AND revoked_at IS NULL`
)

// The statements and their order: lock the user, update the hash, revoke the
// tokens still active, commit. Any failure rolls back.
func TestUserRepository_UpdateOwnPasswordAndRevokeTokens(t *testing.T) {
	revokedAt := time.Now().UTC().Truncate(time.Second)
	const userID int64 = 9
	const hash = "$2a$10$new-hash"
	dbErr := errors.New("connection reset")

	run := func(t *testing.T, setup func(mock sqlmock.Sqlmock)) error {
		t.Helper()
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		setup(mock)
		err = NewUserRepository(db).UpdateOwnPasswordAndRevokeTokens(context.Background(), userID, hash, revokedAt)
		if expErr := mock.ExpectationsWereMet(); expErr != nil {
			t.Errorf("unmet SQL expectations: %v", expErr)
		}
		return err
	}

	t.Run("any role: lock, update, revoke, commit", func(t *testing.T) {
		err := run(t, func(mock sqlmock.Sqlmock) {
			mock.ExpectBegin()
			mock.ExpectQuery(ownLockSQL).WithArgs(userID).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(userID))
			mock.ExpectExec(ownUpdateSQL).WithArgs(hash, userID).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(ownRevokeSQL).WithArgs(revokedAt, userID).WillReturnResult(sqlmock.NewResult(0, 2))
			mock.ExpectCommit()
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing user is ErrUserNotFound and rolls back", func(t *testing.T) {
		err := run(t, func(mock sqlmock.Sqlmock) {
			mock.ExpectBegin()
			mock.ExpectQuery(ownLockSQL).WithArgs(userID).WillReturnError(sql.ErrNoRows)
			mock.ExpectRollback()
		})
		if !errors.Is(err, repository.ErrUserNotFound) {
			t.Fatalf("err = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("update failure rolls back", func(t *testing.T) {
		err := run(t, func(mock sqlmock.Sqlmock) {
			mock.ExpectBegin()
			mock.ExpectQuery(ownLockSQL).WithArgs(userID).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(userID))
			mock.ExpectExec(ownUpdateSQL).WithArgs(hash, userID).WillReturnError(dbErr)
			mock.ExpectRollback()
		})
		if !errors.Is(err, dbErr) {
			t.Fatalf("err = %v, want the update error", err)
		}
	})

	t.Run("revoke failure rolls back the password update", func(t *testing.T) {
		err := run(t, func(mock sqlmock.Sqlmock) {
			mock.ExpectBegin()
			mock.ExpectQuery(ownLockSQL).WithArgs(userID).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(userID))
			mock.ExpectExec(ownUpdateSQL).WithArgs(hash, userID).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(ownRevokeSQL).WithArgs(revokedAt, userID).WillReturnError(dbErr)
			mock.ExpectRollback()
		})
		if !errors.Is(err, dbErr) {
			t.Fatalf("err = %v, want the revoke error", err)
		}
	})

	t.Run("begin failure", func(t *testing.T) {
		err := run(t, func(mock sqlmock.Sqlmock) { mock.ExpectBegin().WillReturnError(dbErr) })
		if !errors.Is(err, dbErr) {
			t.Fatalf("err = %v, want the begin error", err)
		}
	})
}

// TestUserRepository_UpdateOwnPasswordAndRevokeTokens_PostgreSQL runs the
// method on the integration test database: it works for a user and for an
// admin, revokes only that user's active refresh tokens (an already revoked
// token keeps its revocation time), and reports a missing ID.
func TestUserRepository_UpdateOwnPasswordAndRevokeTokens_PostgreSQL(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	tag := fmt.Sprintf("own-password-%d", time.Now().UnixNano())
	// Registered before anything is written; refresh tokens cascade.
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM users WHERE email LIKE $1`, tag+"-%"); err != nil {
			t.Errorf("cleanup users: %v", err)
		}
	})

	users := NewUserRepository(db)
	tokens := NewRefreshTokenRepository(db)
	create := func(name, role string) *domain.User {
		t.Helper()
		u := &domain.User{Email: tag + "-" + name + "@example.com", PasswordHash: "old-hash-" + name, Role: role}
		if err := users.Create(ctx, u); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return u
	}
	issue := func(u *domain.User, name string, revoked *time.Time) {
		t.Helper()
		tok := &domain.RefreshToken{UserID: u.ID, TokenHash: tag + "-" + name, ExpiresAt: time.Now().Add(24 * time.Hour), RevokedAt: revoked}
		if err := tokens.Create(ctx, tok); err != nil {
			t.Fatalf("create token %s: %v", name, err)
		}
	}
	revokedAtOf := func(name string) *time.Time {
		t.Helper()
		var at sql.NullTime
		if err := db.QueryRowContext(ctx, `SELECT revoked_at FROM refresh_tokens WHERE token_hash = $1`, tag+"-"+name).Scan(&at); err != nil {
			t.Fatalf("read token %s: %v", name, err)
		}
		if !at.Valid {
			return nil
		}
		return &at.Time
	}

	member := create("member", "user")
	admin := create("admin", "admin")
	other := create("other", "user")
	earlier := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	issue(member, "member-active-1", nil)
	issue(member, "member-active-2", nil)
	issue(member, "member-revoked", &earlier)
	issue(admin, "admin-active", nil)
	issue(other, "other-active", nil)

	revokedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := users.UpdateOwnPasswordAndRevokeTokens(ctx, member.ID, "new-hash-member", revokedAt); err != nil {
		t.Fatalf("member: %v", err)
	}
	if got, err := users.GetByID(ctx, member.ID); err != nil || got.PasswordHash != "new-hash-member" {
		t.Errorf("member hash = %v, %v; want new-hash-member", got, err)
	}
	for _, name := range []string{"member-active-1", "member-active-2"} {
		if at := revokedAtOf(name); at == nil || !at.Equal(revokedAt) {
			t.Errorf("%s revoked_at = %v, want %v", name, at, revokedAt)
		}
	}
	if at := revokedAtOf("member-revoked"); at == nil || !at.Equal(earlier) {
		t.Errorf("member-revoked revoked_at = %v, want the earlier %v", at, earlier)
	}
	if at := revokedAtOf("other-active"); at != nil {
		t.Errorf("another user's token was revoked at %v", at)
	}
	if at := revokedAtOf("admin-active"); at != nil {
		t.Errorf("the admin's token was revoked by the member's change at %v", at)
	}

	if err := users.UpdateOwnPasswordAndRevokeTokens(ctx, admin.ID, "new-hash-admin", revokedAt); err != nil {
		t.Fatalf("admin: %v", err)
	}
	if got, err := users.GetByID(ctx, admin.ID); err != nil || got.PasswordHash != "new-hash-admin" {
		t.Errorf("admin hash = %v, %v; want new-hash-admin", got, err)
	}
	if at := revokedAtOf("admin-active"); at == nil {
		t.Error("the admin's token was not revoked")
	}
	if got, err := users.GetByID(ctx, other.ID); err != nil || got.PasswordHash != "old-hash-other" {
		t.Errorf("other user's hash = %v, %v; want unchanged", got, err)
	}

	var missingID int64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) + 1000 FROM users`).Scan(&missingID); err != nil {
		t.Fatal(err)
	}
	if err := users.UpdateOwnPasswordAndRevokeTokens(ctx, missingID, "x", revokedAt); !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("missing user: err = %v, want ErrUserNotFound", err)
	}
}
