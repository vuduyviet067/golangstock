package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"gostock/internal/testdb"

	"github.com/jackc/pgx/v5/pgconn"
)

type movementFixture struct {
	userID    int64
	productID int64
	sku       string
}

// withMovementTx runs fn in a transaction that already holds a fresh user and
// product, then rolls everything back.
func withMovementTx(t *testing.T, db *sql.DB, fn func(ctx context.Context, tx *sql.Tx, fx movementFixture) error) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()

	suffix := time.Now().UnixNano()
	fx := movementFixture{sku: fmt.Sprintf("SM-SCHEMA-%d", suffix)}

	if err := tx.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash, role) VALUES ($1, 'x', 'user') RETURNING id`,
		fmt.Sprintf("sm-schema-%d@example.test", suffix),
	).Scan(&fx.userID); err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO products (sku, name, price, quantity) VALUES ($1, 'schema test', 1, 5) RETURNING id`,
		fx.sku,
	).Scan(&fx.productID); err != nil {
		t.Fatalf("create fixture product: %v", err)
	}

	return fn(ctx, tx, fx)
}

func insertMovement(ctx context.Context, tx *sql.Tx, productID any, sku any, movementType string, change, before, after int, userID any) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO stock_movements
		    (product_id, product_sku, movement_type, quantity_change, quantity_before, quantity_after, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		productID, sku, movementType, change, before, after, userID)
	return err
}

// pgError returns the SQLSTATE and constraint name of err, or fails the test
// if err is not a PostgreSQL error.
func pgError(t *testing.T, err error) (string, string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("error = %v, want a PostgreSQL error", err)
	}
	return pgErr.Code, pgErr.ConstraintName
}

func TestStockMovementsSchema_NamedObjects(t *testing.T) {
	db := testdb.Open(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wantConstraints := map[string]string{
		"stock_movements_pkey":                  "p",
		"stock_movements_product_id_fkey":       "f",
		"stock_movements_user_id_fkey":          "f",
		"stock_movements_type_check":            "c",
		"stock_movements_sign_check":            "c",
		"stock_movements_quantity_before_check": "c",
		"stock_movements_quantity_after_check":  "c",
		"stock_movements_balance_check":         "c",
		"stock_movements_actor_check":           "c",
	}

	rows, err := db.QueryContext(ctx, `
		SELECT conname, contype::text, confdeltype::text
		FROM pg_constraint
		WHERE conrelid = 'public.stock_movements'::regclass AND contype <> 'n'`)
	if err != nil {
		t.Fatalf("query constraints (is the test database migrated to the latest version?): %v", err)
	}
	defer rows.Close()

	got := map[string]string{}
	deleteActions := map[string]string{}
	for rows.Next() {
		var name, kind, onDelete string
		if err := rows.Scan(&name, &kind, &onDelete); err != nil {
			t.Fatalf("scan constraint: %v", err)
		}
		got[name] = kind
		deleteActions[name] = onDelete
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("constraints: %v", err)
	}

	for name, kind := range wantConstraints {
		if got[name] != kind {
			t.Errorf("constraint %s: type %q, want %q", name, got[name], kind)
		}
	}
	for name := range got {
		if _, ok := wantConstraints[name]; !ok {
			t.Errorf("unexpected constraint %s", name)
		}
	}
	// confdeltype: 'n' = SET NULL, 'r' = RESTRICT.
	if deleteActions["stock_movements_product_id_fkey"] != "n" {
		t.Errorf("product FK ON DELETE = %q, want SET NULL (n)", deleteActions["stock_movements_product_id_fkey"])
	}
	if deleteActions["stock_movements_user_id_fkey"] != "r" {
		t.Errorf("user FK ON DELETE = %q, want RESTRICT (r)", deleteActions["stock_movements_user_id_fkey"])
	}

	wantIndexes := map[string]string{
		"stock_movements_pkey":           "CREATE UNIQUE INDEX stock_movements_pkey ON public.stock_movements USING btree (id)",
		"stock_movements_product_id_idx": "CREATE INDEX stock_movements_product_id_idx ON public.stock_movements USING btree (product_id, id DESC)",
		"stock_movements_created_at_idx": "CREATE INDEX stock_movements_created_at_idx ON public.stock_movements USING btree (created_at)",
		"stock_movements_user_id_idx":    "CREATE INDEX stock_movements_user_id_idx ON public.stock_movements USING btree (user_id)",
	}
	indexRows, err := db.QueryContext(ctx,
		`SELECT indexname, indexdef FROM pg_indexes WHERE schemaname = 'public' AND tablename = 'stock_movements'`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer indexRows.Close()

	gotIndexes := map[string]string{}
	for indexRows.Next() {
		var name, def string
		if err := indexRows.Scan(&name, &def); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		gotIndexes[name] = def
	}
	if err := indexRows.Err(); err != nil {
		t.Fatalf("indexes: %v", err)
	}
	if len(gotIndexes) != len(wantIndexes) {
		t.Errorf("indexes = %v, want exactly %d", gotIndexes, len(wantIndexes))
	}
	for name, def := range wantIndexes {
		if gotIndexes[name] != def {
			t.Errorf("index %s = %q, want %q", name, gotIndexes[name], def)
		}
	}
}

func TestStockMovementsSchema_ValidRows(t *testing.T) {
	db := testdb.Open(t)

	tests := []struct {
		name                  string
		movementType          string
		change, before, after int
		withUser              bool
	}{
		{"initial without user (opening balance)", "initial", 5, 0, 5, false},
		{"initial with user", "initial", 5, 0, 5, true},
		{"in", "in", 3, 5, 8, true},
		{"out", "out", -2, 5, 3, true},
		{"out to zero", "out", -5, 5, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := withMovementTx(t, db, func(ctx context.Context, tx *sql.Tx, fx movementFixture) error {
				var user any
				if tt.withUser {
					user = fx.userID
				}
				return insertMovement(ctx, tx, fx.productID, fx.sku, tt.movementType, tt.change, tt.before, tt.after, user)
			})
			if err != nil {
				t.Fatalf("insert valid movement: %v", err)
			}
		})
	}
}

func TestStockMovementsSchema_RejectsInvalidRows(t *testing.T) {
	db := testdb.Open(t)

	const (
		checkViolation   = "23514"
		notNullViolation = "23502"
		fkViolation      = "23503"
	)

	tests := []struct {
		name                  string
		movementType          string
		change, before, after int
		user                  string // "fixture", "null" or "missing"
		product               string // "fixture" or "missing"
		nullSKU               bool
		wantCode              string
		// wantConstraints lists acceptable constraint names. PostgreSQL checks
		// CHECK constraints in name order, so a row breaking two of them
		// reports the first.
		wantConstraints []string
	}{
		{"unknown movement type", "adjust", 1, 5, 6, "fixture", "fixture", false, checkViolation,
			[]string{"stock_movements_sign_check", "stock_movements_type_check"}},
		{"in with zero change", "in", 0, 5, 5, "fixture", "fixture", false, checkViolation,
			[]string{"stock_movements_sign_check"}},
		{"in with negative change", "in", -1, 5, 4, "fixture", "fixture", false, checkViolation,
			[]string{"stock_movements_sign_check"}},
		{"initial with negative change", "initial", -1, 1, 0, "fixture", "fixture", false, checkViolation,
			[]string{"stock_movements_sign_check"}},
		{"out with positive change", "out", 1, 5, 6, "fixture", "fixture", false, checkViolation,
			[]string{"stock_movements_sign_check"}},
		{"out with zero change", "out", 0, 5, 5, "fixture", "fixture", false, checkViolation,
			[]string{"stock_movements_sign_check"}},
		{"negative quantity_before", "in", 2, -1, 1, "fixture", "fixture", false, checkViolation,
			[]string{"stock_movements_quantity_before_check"}},
		{"negative quantity_after", "out", -2, 1, -1, "fixture", "fixture", false, checkViolation,
			[]string{"stock_movements_quantity_after_check"}},
		{"after != before + change", "in", 2, 1, 4, "fixture", "fixture", false, checkViolation,
			[]string{"stock_movements_balance_check"}},
		{"in without user", "in", 2, 5, 7, "null", "fixture", false, checkViolation,
			[]string{"stock_movements_actor_check"}},
		{"out without user", "out", -2, 5, 3, "null", "fixture", false, checkViolation,
			[]string{"stock_movements_actor_check"}},
		{"missing product_sku", "in", 2, 5, 7, "fixture", "fixture", true, notNullViolation, nil},
		{"unknown product", "in", 2, 5, 7, "fixture", "missing", false, fkViolation,
			[]string{"stock_movements_product_id_fkey"}},
		{"unknown user", "in", 2, 5, 7, "missing", "fixture", false, fkViolation,
			[]string{"stock_movements_user_id_fkey"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := withMovementTx(t, db, func(ctx context.Context, tx *sql.Tx, fx movementFixture) error {
				var user, product, sku any = fx.userID, fx.productID, fx.sku
				switch tt.user {
				case "null":
					user = nil
				case "missing":
					user = int64(-1)
				}
				if tt.product == "missing" {
					product = int64(-1)
				}
				if tt.nullSKU {
					sku = nil
				}
				return insertMovement(ctx, tx, product, sku, tt.movementType, tt.change, tt.before, tt.after, user)
			})
			if err == nil {
				t.Fatal("insert succeeded, want a constraint violation")
			}

			code, constraint := pgError(t, err)
			if code != tt.wantCode {
				t.Fatalf("SQLSTATE = %s (constraint %q), want %s", code, constraint, tt.wantCode)
			}
			if tt.wantConstraints == nil {
				return
			}
			for _, want := range tt.wantConstraints {
				if constraint == want {
					return
				}
			}
			t.Errorf("constraint = %q, want one of %v", constraint, tt.wantConstraints)
		})
	}
}

// TestStockMovementsSchema_ProductDeleteKeepsHistory checks ON DELETE SET NULL:
// deleting a product keeps its movements, with product_id cleared and the SKU
// snapshot intact.
func TestStockMovementsSchema_ProductDeleteKeepsHistory(t *testing.T) {
	db := testdb.Open(t)

	err := withMovementTx(t, db, func(ctx context.Context, tx *sql.Tx, fx movementFixture) error {
		if err := insertMovement(ctx, tx, fx.productID, fx.sku, "initial", 5, 0, 5, fx.userID); err != nil {
			return fmt.Errorf("insert initial: %w", err)
		}
		if err := insertMovement(ctx, tx, fx.productID, fx.sku, "out", -2, 5, 3, fx.userID); err != nil {
			return fmt.Errorf("insert out: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM products WHERE id = $1`, fx.productID); err != nil {
			return fmt.Errorf("delete product with history: %w", err)
		}

		var total, orphaned int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*), COUNT(*) FILTER (WHERE product_id IS NULL)
			FROM stock_movements WHERE product_sku = $1`, fx.sku,
		).Scan(&total, &orphaned); err != nil {
			return fmt.Errorf("read history: %w", err)
		}
		if total != 2 || orphaned != 2 {
			t.Errorf("after product delete: %d movements, %d with NULL product_id; want 2 and 2", total, orphaned)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestStockMovementsSchema_UserDeleteRestricted checks ON DELETE RESTRICT: a
// user who recorded a movement cannot be deleted.
func TestStockMovementsSchema_UserDeleteRestricted(t *testing.T) {
	db := testdb.Open(t)

	err := withMovementTx(t, db, func(ctx context.Context, tx *sql.Tx, fx movementFixture) error {
		if err := insertMovement(ctx, tx, fx.productID, fx.sku, "in", 2, 5, 7, fx.userID); err != nil {
			t.Fatalf("insert movement: %v", err)
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, fx.userID)
		return err
	})
	if err == nil {
		t.Fatal("deleting a user with movements succeeded, want a foreign key violation")
	}
	code, constraint := pgError(t, err)
	if code != "23503" || constraint != "stock_movements_user_id_fkey" {
		t.Errorf("error = %s %q, want 23503 stock_movements_user_id_fkey", code, constraint)
	}
}

// TestStockMovementsSchema_CreatedAtIsWriteTime checks that created_at
// defaults to clock_timestamp(): two rows written in one transaction get
// increasing times (NOW() would give both the transaction start time).
func TestStockMovementsSchema_CreatedAtIsWriteTime(t *testing.T) {
	db := testdb.Open(t)

	err := withMovementTx(t, db, func(ctx context.Context, tx *sql.Tx, fx movementFixture) error {
		if err := insertMovement(ctx, tx, fx.productID, fx.sku, "initial", 5, 0, 5, fx.userID); err != nil {
			return fmt.Errorf("insert first: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `SELECT pg_sleep(0.01)`); err != nil {
			return fmt.Errorf("sleep: %w", err)
		}
		if err := insertMovement(ctx, tx, fx.productID, fx.sku, "in", 1, 5, 6, fx.userID); err != nil {
			return fmt.Errorf("insert second: %w", err)
		}

		var first, second, txStart time.Time
		if err := tx.QueryRowContext(ctx, `
			SELECT MIN(created_at), MAX(created_at), NOW()
			FROM stock_movements WHERE product_sku = $1`, fx.sku,
		).Scan(&first, &second, &txStart); err != nil {
			return fmt.Errorf("read created_at: %w", err)
		}
		if !second.After(first) {
			t.Errorf("created_at %v then %v, want increasing write times", first, second)
		}
		if !first.After(txStart) {
			t.Errorf("created_at %v is not after transaction start %v; default is not clock_timestamp()", first, txStart)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
