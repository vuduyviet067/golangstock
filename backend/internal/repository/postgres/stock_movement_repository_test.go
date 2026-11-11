package postgres

import (
	"context"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/repository"
)

// These integration tests read the stock movement history through the
// isolated session from product_repository_stock_test.go, so totals and
// global ordering are exact: no other test's rows are visible.

// insertMovementAt writes a movement with an explicit created_at, so the
// tests control the global ordering. productID 0 stores NULL.
func (iso *isolatedStockDB) insertMovementAt(
	t *testing.T,
	productID int64,
	sku, movementType string,
	change, before, after int,
	userID int64,
	createdAt time.Time,
) int64 {
	t.Helper()

	var product any
	if productID != 0 {
		product = productID
	}
	var id int64
	err := iso.db.QueryRow(`
		INSERT INTO stock_movements
		    (product_id, product_sku, movement_type, quantity_change,
		     quantity_before, quantity_after, user_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		product, sku, movementType, change, before, after, userID, createdAt,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert movement: %v", err)
	}
	return id
}

func movementIDs(movements []domain.StockMovement) []int64 {
	ids := make([]int64, len(movements))
	for i, m := range movements {
		ids[i] = m.ID
	}
	return ids
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStockMovementRepository_ListByProduct(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	products := NewProductRepository(iso.db)
	repo := NewStockMovementRepository(iso.db)
	ctx := context.Background()
	actor := iso.createUser(t)

	a := newStockProduct("HIST-A", 5)
	if err := products.Create(ctx, a, actor); err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if _, err := products.StockIn(ctx, a.ID, 3, actor); err != nil {
		t.Fatalf("StockIn A: %v", err)
	}
	if _, err := products.StockOut(ctx, a.ID, 2, actor); err != nil {
		t.Fatalf("StockOut A: %v", err)
	}
	b := newStockProduct("HIST-B", 4)
	if err := products.Create(ctx, b, actor); err != nil {
		t.Fatalf("Create B: %v", err)
	}
	iso.check(t)

	got, total, err := repo.ListByProduct(ctx, a.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByProduct: %v", err)
	}
	iso.check(t)

	if total != 3 || len(got) != 3 {
		t.Fatalf("ListByProduct(A) = %d rows, total %d; want 3 and 3 (B's movement excluded)", len(got), total)
	}
	want := []struct {
		typ                   string
		change, before, after int
	}{
		{repository.MovementTypeOut, -2, 8, 6},
		{repository.MovementTypeIn, 3, 5, 8},
		{repository.MovementTypeInitial, 5, 0, 5},
	}
	for i, w := range want {
		m := got[i]
		if m.Type != w.typ || m.QuantityChange != w.change || m.QuantityBefore != w.before || m.QuantityAfter != w.after {
			t.Errorf("row %d = %s %d (%d -> %d), want %s %d (%d -> %d)",
				i, m.Type, m.QuantityChange, m.QuantityBefore, m.QuantityAfter, w.typ, w.change, w.before, w.after)
		}
		if m.ProductID == nil || *m.ProductID != a.ID || m.ProductSKU != "HIST-A" {
			t.Errorf("row %d product = %v %q, want %d HIST-A", i, m.ProductID, m.ProductSKU, a.ID)
		}
		if m.UserID == nil || *m.UserID != actor {
			t.Errorf("row %d user = %v, want %d", i, m.UserID, actor)
		}
		if m.CreatedAt.IsZero() {
			t.Errorf("row %d has no created_at", i)
		}
		if i > 0 && got[i-1].ID <= m.ID {
			t.Errorf("rows not ordered by id DESC: %d before %d", got[i-1].ID, m.ID)
		}
	}
}

func TestStockMovementRepository_ListByProductPagination(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	products := NewProductRepository(iso.db)
	repo := NewStockMovementRepository(iso.db)
	ctx := context.Background()
	actor := iso.createUser(t)

	p := newStockProduct("PAGED", 1)
	if err := products.Create(ctx, p, actor); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for i := 0; i < 4; i++ {
		if _, err := products.StockIn(ctx, p.ID, 1, actor); err != nil {
			t.Fatalf("StockIn: %v", err)
		}
	}
	iso.check(t)

	all, _, err := repo.ListByProduct(ctx, p.ID, 1, 100)
	if err != nil {
		t.Fatalf("ListByProduct all: %v", err)
	}
	iso.check(t)
	allIDs := movementIDs(all) // newest first, 5 rows

	pages := []struct {
		page    int
		wantIDs []int64
	}{
		{1, allIDs[0:2]},
		{2, allIDs[2:4]},
		{3, allIDs[4:5]},
		{4, []int64{}},
	}
	for _, pg := range pages {
		got, total, err := repo.ListByProduct(ctx, p.ID, pg.page, 2)
		if err != nil {
			t.Fatalf("page %d: %v", pg.page, err)
		}
		iso.check(t)
		if total != 5 {
			t.Errorf("page %d total = %d, want 5", pg.page, total)
		}
		if ids := movementIDs(got); !equalIDs(ids, pg.wantIDs) {
			t.Errorf("page %d ids = %v, want %v", pg.page, ids, pg.wantIDs)
		}
	}
}

// TestStockMovementRepository_ListOrderAndDeletedProducts checks the global
// order (created_at DESC, then id DESC for equal timestamps), pagination and
// that movements of a deleted product stay listed with a NULL product_id and
// their SKU snapshot.
func TestStockMovementRepository_ListOrderAndDeletedProducts(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	products := NewProductRepository(iso.db)
	repo := NewStockMovementRepository(iso.db)
	ctx := context.Background()
	actor := iso.createUser(t)

	kept := newStockProduct("KEPT", 0)
	if err := products.Create(ctx, kept, actor); err != nil {
		t.Fatalf("Create KEPT: %v", err)
	}
	gone := newStockProduct("GONE", 0)
	if err := products.Create(ctx, gone, actor); err != nil {
		t.Fatalf("Create GONE: %v", err)
	}
	iso.check(t)

	later := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	earlier := later.Add(-time.Hour)

	// Inserted in this id order; expected global order: b, a (same time,
	// higher id first), c (earlier time despite the highest id).
	a := iso.insertMovementAt(t, kept.ID, "KEPT", "initial", 5, 0, 5, actor, later)
	b := iso.insertMovementAt(t, gone.ID, "GONE", "initial", 2, 0, 2, actor, later)
	c := iso.insertMovementAt(t, kept.ID, "KEPT", "in", 1, 5, 6, actor, earlier)
	// SQL metacharacters in stored data come back verbatim.
	odd := "X');DROP TABLE stock_movements;--"
	d := iso.insertMovementAt(t, 0, odd, "initial", 1, 0, 1, actor, earlier.Add(-time.Hour))

	if _, err := iso.db.Exec(`DELETE FROM products WHERE id = $1`, gone.ID); err != nil {
		t.Fatalf("delete product: %v", err)
	}

	got, total, err := repo.List(ctx, 1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	iso.check(t)

	if want := []int64{b, a, c, d}; !equalIDs(movementIDs(got), want) || total != 4 {
		t.Fatalf("List ids = %v total %d, want %v total 4", movementIDs(got), total, want)
	}

	deleted := got[0]
	if deleted.ProductID != nil {
		t.Errorf("movement of deleted product has product_id %d, want NULL", *deleted.ProductID)
	}
	if deleted.ProductSKU != "GONE" || deleted.Type != "initial" || deleted.QuantityAfter != 2 {
		t.Errorf("movement of deleted product = %+v, want its GONE snapshot", deleted)
	}
	if got[3].ProductSKU != odd || got[3].ProductID != nil {
		t.Errorf("odd row = %q product %v, want SKU %q and NULL product", got[3].ProductSKU, got[3].ProductID, odd)
	}

	// The deleted product's history is no longer reachable by product id.
	byGone, goneTotal, err := repo.ListByProduct(ctx, gone.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByProduct(deleted): %v", err)
	}
	iso.check(t)
	if len(byGone) != 0 || goneTotal != 0 {
		t.Errorf("ListByProduct(deleted) = %d rows total %d, want none", len(byGone), goneTotal)
	}

	// Global pagination.
	for _, pg := range []struct {
		page    int
		wantIDs []int64
	}{
		{1, []int64{b, a}},
		{2, []int64{c, d}},
		{3, []int64{}},
	} {
		page, total, err := repo.List(ctx, pg.page, 2)
		if err != nil {
			t.Fatalf("List page %d: %v", pg.page, err)
		}
		iso.check(t)
		if total != 4 || !equalIDs(movementIDs(page), pg.wantIDs) {
			t.Errorf("List page %d = %v total %d, want %v total 4", pg.page, movementIDs(page), total, pg.wantIDs)
		}
	}

	// The table is untouched by reads.
	if n := iso.count(t, `SELECT COUNT(*) FROM stock_movements`); n != 4 {
		t.Errorf("stock_movements has %d rows after reads, want 4", n)
	}
}

func TestStockMovementRepository_EmptyHistory(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	repo := NewStockMovementRepository(iso.db)
	ctx := context.Background()

	got, total, err := repo.List(ctx, 1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	iso.check(t)
	if got == nil || len(got) != 0 || total != 0 {
		t.Errorf("List on empty history = %#v total %d, want empty non-nil slice and 0", got, total)
	}

	got, total, err = repo.ListByProduct(ctx, 1, 1, 10)
	if err != nil {
		t.Fatalf("ListByProduct: %v", err)
	}
	iso.check(t)
	if got == nil || len(got) != 0 || total != 0 {
		t.Errorf("ListByProduct on empty history = %#v total %d, want empty non-nil slice and 0", got, total)
	}
}
