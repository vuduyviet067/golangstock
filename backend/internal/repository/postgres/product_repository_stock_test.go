package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/repository"
	"gostock/internal/testdb"

	"github.com/jackc/pgx/v5/pgconn"
)

type isolatedStockDB struct {
	db  *sql.DB
	pid int
}

const rejectMovementSKUPrefix = "REJECT-MOVEMENT"

func openIsolatedStockDB(t *testing.T, deferUserFK bool) *isolatedStockDB {
	t.Helper()

	db := testdb.Open(t)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	userFK := `ALTER TABLE pg_temp.stock_movements
		ADD CONSTRAINT stock_movements_user_id_fkey
		FOREIGN KEY (user_id) REFERENCES pg_temp.users(id) ON DELETE RESTRICT`
	if deferUserFK {
		userFK = `ALTER TABLE pg_temp.stock_movements
			ADD CONSTRAINT stock_movements_user_id_fkey
			FOREIGN KEY (user_id) REFERENCES pg_temp.users(id)
			DEFERRABLE INITIALLY DEFERRED`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stmts := []string{
		`CREATE TEMP TABLE users (LIKE public.users INCLUDING ALL)`,
		`CREATE TEMP TABLE products (LIKE public.products INCLUDING ALL)`,
		`CREATE TEMP TABLE stock_movements (LIKE public.stock_movements INCLUDING ALL)`,
		// LIKE does not copy foreign keys; recreate them under the production
		// names, which the repository maps errors by.
		`ALTER TABLE pg_temp.stock_movements
			ADD CONSTRAINT stock_movements_product_id_fkey
			FOREIGN KEY (product_id) REFERENCES pg_temp.products(id) ON DELETE SET NULL`,
		userFK,
		// Test-only failure injection for the movement INSERT.
		`ALTER TABLE pg_temp.stock_movements
			ADD CONSTRAINT test_reject_movement_check
			CHECK (product_sku NOT LIKE '` + rejectMovementSKUPrefix + `%')`,
	}
	if deferUserFK {
		stmts = append(stmts,
			`ALTER TABLE pg_temp.products DROP CONSTRAINT products_sku_key`,
			`ALTER TABLE pg_temp.products
				ADD CONSTRAINT products_sku_key UNIQUE (sku) DEFERRABLE INITIALLY DEFERRED`)
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("create temporary table: %v\n%s", err, stmt)
		}
	}

	for _, c := range []struct{ table, name string }{
		{"pg_temp.stock_movements", "stock_movements_user_id_fkey"},
		{"pg_temp.products", "products_sku_key"},
	} {
		var deferrable, deferred bool
		if err := db.QueryRowContext(ctx, `
			SELECT condeferrable, condeferred FROM pg_constraint
			WHERE conrelid = $1::regclass AND conname = $2`, c.table, c.name,
		).Scan(&deferrable, &deferred); err != nil {
			t.Fatalf("read temporary constraint %s: %v", c.name, err)
		}
		if deferrable != deferUserFK || deferred != deferUserFK {
			t.Fatalf("temporary %s deferrable=%v deferred=%v, want %v", c.name, deferrable, deferred, deferUserFK)
		}
	}

	iso := &isolatedStockDB{db: db}
	if err := db.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&iso.pid); err != nil {
		t.Fatalf("read backend pid: %v", err)
	}
	iso.check(t)
	return iso
}

func (iso *isolatedStockDB) check(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var pid int
	var shadowed bool
	err := iso.db.QueryRowContext(ctx, `
		SELECT pg_backend_pid(),
		       to_regclass('pg_temp.users') IS NOT NULL
		   AND to_regclass('pg_temp.products') IS NOT NULL
		   AND to_regclass('pg_temp.stock_movements') IS NOT NULL
		   AND to_regclass('users') = to_regclass('pg_temp.users')
		   AND to_regclass('products') = to_regclass('pg_temp.products')
		   AND to_regclass('stock_movements') = to_regclass('pg_temp.stock_movements')`,
	).Scan(&pid, &shadowed)
	if err != nil {
		t.Fatalf("isolation check: %v", err)
	}
	if pid != iso.pid {
		t.Fatalf("backend pid changed from %d to %d: the pool replaced the connection and later statements may have used the public tables", iso.pid, pid)
	}
	if !shadowed {
		t.Fatal("temporary tables no longer shadow the public tables")
	}
}

func (iso *isolatedStockDB) createUser(t *testing.T) int64 {
	t.Helper()
	var id int64
	err := iso.db.QueryRow(
		`INSERT INTO users (email, password_hash, role) VALUES ($1, 'x', 'user') RETURNING id`,
		fmt.Sprintf("repo-stock-%d@example.test", time.Now().UnixNano()),
	).Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}

func (iso *isolatedStockDB) quantity(t *testing.T, productID int64) int {
	t.Helper()
	var q int
	if err := iso.db.QueryRow(`SELECT quantity FROM products WHERE id = $1`, productID).Scan(&q); err != nil {
		t.Fatalf("read quantity: %v", err)
	}
	return q
}

func (iso *isolatedStockDB) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := iso.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// stockRow is the part of a stock movement these tests compare.
type stockRow struct {
	ProductID int64
	SKU       string
	Type      string
	Change    int
	Before    int
	After     int
	UserID    int64
}

// movements returns every movement in the isolated session in id order.
func (iso *isolatedStockDB) movements(t *testing.T) []stockRow {
	t.Helper()
	rows, err := iso.db.Query(`
		SELECT COALESCE(product_id, 0), product_sku, movement_type,
		       quantity_change, quantity_before, quantity_after, COALESCE(user_id, 0)
		FROM stock_movements ORDER BY id`)
	if err != nil {
		t.Fatalf("read movements: %v", err)
	}
	defer rows.Close()

	var out []stockRow
	for rows.Next() {
		var m stockRow
		if err := rows.Scan(&m.ProductID, &m.SKU, &m.Type, &m.Change, &m.Before, &m.After, &m.UserID); err != nil {
			t.Fatalf("scan movement: %v", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("movements: %v", err)
	}
	return out
}

func newStockProduct(sku string, quantity int) *domain.Product {
	return &domain.Product{SKU: sku, Name: "stock test", Price: 1, Quantity: quantity}
}

func assertMovements(t *testing.T, got, want []stockRow) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("movements = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("movement %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestProductRepositoryStock_CreateRecordsInitialMovement(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	repo := NewProductRepository(iso.db)
	ctx := context.Background()
	actor := iso.createUser(t)

	empty := newStockProduct("EMPTY", 0)
	if err := repo.Create(ctx, empty, actor); err != nil {
		t.Fatalf("Create quantity 0: %v", err)
	}
	iso.check(t)
	if empty.ID == 0 {
		t.Fatal("Create did not set the product ID")
	}
	if got := iso.movements(t); len(got) != 0 {
		t.Fatalf("quantity 0 created movements %+v, want none", got)
	}

	stocked := newStockProduct("STOCKED", 7)
	if err := repo.Create(ctx, stocked, actor); err != nil {
		t.Fatalf("Create quantity 7: %v", err)
	}
	iso.check(t)

	assertMovements(t, iso.movements(t), []stockRow{
		{ProductID: stocked.ID, SKU: "STOCKED", Type: repository.MovementTypeInitial, Change: 7, Before: 0, After: 7, UserID: actor},
	})
}

func TestProductRepositoryStock_StockInAndOutRecordMovements(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	repo := NewProductRepository(iso.db)
	ctx := context.Background()
	actor := iso.createUser(t)

	product := newStockProduct("FLOW", 5)
	if err := repo.Create(ctx, product, actor); err != nil {
		t.Fatalf("Create: %v", err)
	}
	iso.check(t)

	afterIn, err := repo.StockIn(ctx, product.ID, 3, actor)
	if err != nil {
		t.Fatalf("StockIn: %v", err)
	}
	iso.check(t)
	if afterIn.Quantity != 8 {
		t.Errorf("StockIn returned quantity %d, want 8", afterIn.Quantity)
	}

	afterOut, err := repo.StockOut(ctx, product.ID, 2, actor)
	if err != nil {
		t.Fatalf("StockOut: %v", err)
	}
	iso.check(t)
	if afterOut.Quantity != 6 {
		t.Errorf("StockOut returned quantity %d, want 6", afterOut.Quantity)
	}

	assertMovements(t, iso.movements(t), []stockRow{
		{ProductID: product.ID, SKU: "FLOW", Type: repository.MovementTypeInitial, Change: 5, Before: 0, After: 5, UserID: actor},
		{ProductID: product.ID, SKU: "FLOW", Type: repository.MovementTypeIn, Change: 3, Before: 5, After: 8, UserID: actor},
		{ProductID: product.ID, SKU: "FLOW", Type: repository.MovementTypeOut, Change: -2, Before: 8, After: 6, UserID: actor},
	})
	if got := iso.quantity(t, product.ID); got != 6 {
		t.Errorf("stored quantity = %d, want 6", got)
	}
	if sum := iso.count(t, `SELECT COALESCE(SUM(quantity_change), 0) FROM stock_movements WHERE product_id = $1`, product.ID); sum != 6 {
		t.Errorf("SUM(quantity_change) = %d, want stored quantity 6", sum)
	}
}

func TestProductRepositoryStock_NotFoundAndInsufficientStock(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	repo := NewProductRepository(iso.db)
	ctx := context.Background()
	actor := iso.createUser(t)

	if _, err := repo.StockIn(ctx, 999999, 1, actor); !errors.Is(err, repository.ErrProductNotFound) {
		t.Errorf("StockIn unknown product: error = %v, want ErrProductNotFound", err)
	}
	iso.check(t)
	if _, err := repo.StockOut(ctx, 999999, 1, actor); !errors.Is(err, repository.ErrProductNotFound) {
		t.Errorf("StockOut unknown product: error = %v, want ErrProductNotFound", err)
	}
	iso.check(t)

	product := newStockProduct("LOW", 2)
	if err := repo.Create(ctx, product, actor); err != nil {
		t.Fatalf("Create: %v", err)
	}
	iso.check(t)

	if _, err := repo.StockOut(ctx, product.ID, 3, actor); !errors.Is(err, repository.ErrInsufficientStock) {
		t.Errorf("StockOut beyond stock: error = %v, want ErrInsufficientStock", err)
	}
	iso.check(t)

	if got := iso.quantity(t, product.ID); got != 2 {
		t.Errorf("quantity after rejected stock-out = %d, want 2", got)
	}
	assertMovements(t, iso.movements(t), []stockRow{
		{ProductID: product.ID, SKU: "LOW", Type: repository.MovementTypeInitial, Change: 2, Before: 0, After: 2, UserID: actor},
	})
}

func TestProductRepositoryStock_DuplicateSKUWritesNothing(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	repo := NewProductRepository(iso.db)
	ctx := context.Background()
	actor := iso.createUser(t)

	if err := repo.Create(ctx, newStockProduct("DUP", 1), actor); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	iso.check(t)

	duplicate := newStockProduct("DUP", 9)
	if err := repo.Create(ctx, duplicate, actor); !errors.Is(err, repository.ErrProductSKUExists) {
		t.Fatalf("duplicate Create: error = %v, want ErrProductSKUExists", err)
	}
	iso.check(t)
	if duplicate.ID != 0 {
		t.Errorf("failed Create set product ID %d, want 0", duplicate.ID)
	}
	if n := len(iso.movements(t)); n != 1 {
		t.Errorf("movements = %d, want only the first product's initial movement", n)
	}
}

func TestProductRepositoryStock_MovementFailureRollsBack(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	repo := NewProductRepository(iso.db)
	ctx := context.Background()
	actor := iso.createUser(t)
	const missingActor = int64(999999)

	product := newStockProduct("ROLLBACK", 5)
	if err := repo.Create(ctx, product, actor); err != nil {
		t.Fatalf("Create: %v", err)
	}
	iso.check(t)
	before := iso.movements(t)

	if _, err := repo.StockIn(ctx, product.ID, 4, missingActor); !errors.Is(err, repository.ErrInvalidActor) {
		t.Errorf("StockIn with unknown actor: error = %v, want ErrInvalidActor", err)
	}
	iso.check(t)
	if _, err := repo.StockOut(ctx, product.ID, 4, missingActor); !errors.Is(err, repository.ErrInvalidActor) {
		t.Errorf("StockOut with unknown actor: error = %v, want ErrInvalidActor", err)
	}
	iso.check(t)

	if got := iso.quantity(t, product.ID); got != 5 {
		t.Errorf("quantity = %d after failed stock changes, want unchanged 5", got)
	}
	assertMovements(t, iso.movements(t), before)

	// Create: the actor check and the product INSERT succeed, the initial
	// movement INSERT is rejected.
	sku := rejectMovementSKUPrefix + "-CREATE"
	orphan := newStockProduct(sku, 3)
	err := repo.Create(ctx, orphan, actor)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "test_reject_movement_check" {
		t.Fatalf("Create with failing movement INSERT: error = %v, want test_reject_movement_check violation", err)
	}
	iso.check(t)
	if orphan.ID != 0 {
		t.Errorf("failed Create set product ID %d, want 0", orphan.ID)
	}
	if n := iso.count(t, `SELECT COUNT(*) FROM products WHERE sku = $1`, sku); n != 0 {
		t.Errorf("product rows for %s = %d, want 0 (insert must be rolled back)", sku, n)
	}
	assertMovements(t, iso.movements(t), before)

	// Nothing of the failed create survived: the SKU is free for a product
	// without stock (no movement, so the test CHECK does not apply).
	if err := repo.Create(ctx, newStockProduct(sku, 0), actor); err != nil {
		t.Errorf("Create after rollback: %v", err)
	}
	iso.check(t)
}

func TestProductRepositoryStock_CommitFailureRollsBack(t *testing.T) {
	iso := openIsolatedStockDB(t, true)
	repo := NewProductRepository(iso.db)
	ctx := context.Background()
	actor := iso.createUser(t)
	const missingActor = int64(999999)

	product := newStockProduct("COMMIT", 5)
	if err := repo.Create(ctx, product, actor); err != nil {
		t.Fatalf("Create: %v", err)
	}
	iso.check(t)
	before := iso.movements(t)

	if _, err := repo.StockIn(ctx, product.ID, 4, missingActor); !errors.Is(err, repository.ErrInvalidActor) {
		t.Errorf("StockIn with failing commit: error = %v, want ErrInvalidActor", err)
	}
	iso.check(t)
	if _, err := repo.StockOut(ctx, product.ID, 4, missingActor); !errors.Is(err, repository.ErrInvalidActor) {
		t.Errorf("StockOut with failing commit: error = %v, want ErrInvalidActor", err)
	}
	iso.check(t)

	if got := iso.quantity(t, product.ID); got != 5 {
		t.Errorf("quantity = %d after failed commits, want unchanged 5", got)
	}
	assertMovements(t, iso.movements(t), before)

	duplicate := newStockProduct("COMMIT", 3)
	if err := repo.Create(ctx, duplicate, actor); !errors.Is(err, repository.ErrProductSKUExists) {
		t.Fatalf("Create with failing commit: error = %v, want ErrProductSKUExists", err)
	}
	iso.check(t)
	if duplicate.ID != 0 {
		t.Errorf("failed Create set product ID %d, want 0", duplicate.ID)
	}
	if n := iso.count(t, `SELECT COUNT(*) FROM products WHERE sku = 'COMMIT'`); n != 1 {
		t.Errorf("product rows for COMMIT = %d, want only the original", n)
	}
	if got := iso.quantity(t, product.ID); got != 5 {
		t.Errorf("original product quantity = %d, want unchanged 5", got)
	}
	assertMovements(t, iso.movements(t), before)
}

func TestProductRepositoryStock_CreateValidatesActor(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	repo := NewProductRepository(iso.db)
	ctx := context.Background()
	actor := iso.createUser(t)
	const missingActor = int64(999999)

	tests := []struct {
		name          string
		sku           string
		quantity      int
		actorID       int64
		wantErr       error
		wantMovements []stockRow // for this SKU
	}{
		{"existing actor, quantity 0", "ACTOR-OK-0", 0, actor, nil, nil},
		{"existing actor, quantity 6", "ACTOR-OK-6", 6, actor, nil, []stockRow{
			{SKU: "ACTOR-OK-6", Type: repository.MovementTypeInitial, Change: 6, Before: 0, After: 6, UserID: actor},
		}},
		{"missing actor, quantity 0", "ACTOR-MISSING-0", 0, missingActor, repository.ErrInvalidActor, nil},
		{"missing actor, quantity 6", "ACTOR-MISSING-6", 6, missingActor, repository.ErrInvalidActor, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			product := newStockProduct(tt.sku, tt.quantity)
			err := repo.Create(ctx, product, tt.actorID)
			iso.check(t)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create error = %v, want %v", err, tt.wantErr)
			}

			products := iso.count(t, `SELECT COUNT(*) FROM products WHERE sku = $1`, tt.sku)
			var got []stockRow
			for _, m := range iso.movements(t) {
				if m.SKU == tt.sku {
					m.ProductID = 0 // compared separately below
					got = append(got, m)
				}
			}

			if tt.wantErr != nil {
				if product.ID != 0 {
					t.Errorf("product ID = %d, want 0", product.ID)
				}
				if products != 0 || len(got) != 0 {
					t.Errorf("failed Create left %d product rows and %d movements, want none", products, len(got))
				}
				return
			}

			if product.ID == 0 || products != 1 {
				t.Fatalf("product ID = %d, rows = %d; want a stored product", product.ID, products)
			}
			assertMovements(t, got, tt.wantMovements)
		})
	}
}

func TestProductRepositoryStock_CreateWaitsForConcurrentUserDelete(t *testing.T) {
	for _, commitDelete := range []bool{true, false} {
		name := "delete rolls back"
		if commitDelete {
			name = "delete commits"
		}
		t.Run(name, func(t *testing.T) {
			db := testdb.Open(t)
			repo := NewProductRepository(db)

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			suffix := time.Now().UnixNano()
			var actor int64
			if err := db.QueryRowContext(ctx,
				`INSERT INTO users (email, password_hash, role) VALUES ($1, 'x', 'user') RETURNING id`,
				fmt.Sprintf("delete-race-%d@example.test", suffix),
			).Scan(&actor); err != nil {
				t.Fatalf("create user: %v", err)
			}
			sku := fmt.Sprintf("DELETE-RACE-%d", suffix)
			t.Cleanup(func() {
				_, _ = db.Exec(`DELETE FROM products WHERE sku = $1`, sku)
				_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, actor)
			})

			// Start deleting the user, without committing yet.
			deleteTx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatalf("begin delete: %v", err)
			}
			defer deleteTx.Rollback()
			if _, err := deleteTx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, actor); err != nil {
				t.Fatalf("delete user: %v", err)
			}

			product := newStockProduct(sku, 0)
			done := make(chan error, 1)
			go func() { done <- repo.Create(ctx, product, actor) }()

			// Wait until Create is blocked on the user row lock.
			waitForLockWait(t, ctx, db, "FOR KEY SHARE", done)

			if commitDelete {
				if err := deleteTx.Commit(); err != nil {
					t.Fatalf("commit delete: %v", err)
				}
			} else if err := deleteTx.Rollback(); err != nil {
				t.Fatalf("roll back delete: %v", err)
			}

			var createErr error
			select {
			case createErr = <-done:
			case <-ctx.Done():
				t.Fatal("Create did not finish after the delete ended")
			}

			var products int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM products WHERE sku = $1`, sku).Scan(&products); err != nil {
				t.Fatalf("count products: %v", err)
			}

			if commitDelete {
				if !errors.Is(createErr, repository.ErrInvalidActor) {
					t.Errorf("Create after committed delete: error = %v, want ErrInvalidActor", createErr)
				}
				if products != 0 || product.ID != 0 {
					t.Errorf("Create after committed delete left %d products (ID %d), want none", products, product.ID)
				}
				return
			}
			if createErr != nil {
				t.Errorf("Create after rolled-back delete: %v", createErr)
			}
			if products != 1 {
				t.Errorf("products = %d after rolled-back delete, want 1", products)
			}
		})
	}
}

func waitForLockWait(t *testing.T, ctx context.Context, db *sql.DB, marker string, done <-chan error) {
	t.Helper()
	for {
		var waiting bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity
				WHERE datname = current_database()
				  AND wait_event_type = 'Lock'
				  AND query LIKE '%' || $1 || '%'
				  AND pid <> pg_backend_pid())`, marker,
		).Scan(&waiting); err != nil {
			t.Fatalf("read pg_stat_activity: %v", err)
		}
		if waiting {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("Create finished (error %v) without waiting for the concurrent delete", err)
		case <-ctx.Done():
			t.Fatal("Create never waited for the user row lock")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestProductRepositoryStock_IsolationDoesNotTouchPublicTables(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	repo := NewProductRepository(iso.db)
	ctx := context.Background()
	actor := iso.createUser(t)

	sku := fmt.Sprintf("ISOLATED-%d", time.Now().UnixNano())
	product := newStockProduct(sku, 4)
	if err := repo.Create(ctx, product, actor); err != nil {
		t.Fatalf("Create: %v", err)
	}
	iso.check(t)
	if _, err := repo.StockIn(ctx, product.ID, 1, actor); err != nil {
		t.Fatalf("StockIn: %v", err)
	}
	iso.check(t)

	if n := iso.count(t, `SELECT COUNT(*) FROM public.products WHERE sku = $1`, sku); n != 0 {
		t.Errorf("public.products has %d rows for %s, want 0", n, sku)
	}
	if n := iso.count(t, `SELECT COUNT(*) FROM public.stock_movements WHERE product_sku = $1`, sku); n != 0 {
		t.Errorf("public.stock_movements has %d rows for %s, want 0", n, sku)
	}
	if n := iso.count(t, `SELECT COUNT(*) FROM stock_movements WHERE product_sku = $1`, sku); n != 2 {
		t.Errorf("isolated stock_movements has %d rows for %s, want 2", n, sku)
	}
}
