package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"gostock/internal/domain"
	"gostock/internal/repository"
	"gostock/internal/testdb"
)

// TestCatalogRepositories_PostgreSQL runs both catalog repositories on the
// integration test database (migration 000009): full CRUD, duplicate wood
// type names, the CHECK constraints when invalid data bypasses the use case,
// and a referenced row refused with ErrInUse.
func TestCatalogRepositories_PostgreSQL(t *testing.T) {
	db := testdb.Open(t)
	// One connection, so the TEMP table below (visible to its own session only)
	// is seen by every statement of the test, the repositories' included.
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tag := fmt.Sprintf("catalog-repo-%d", time.Now().UnixNano())
	// Registered before anything is written. The TEMP table goes first: it
	// references wood_types.
	t.Cleanup(func() {
		bg := context.Background()
		if _, err := db.ExecContext(bg, `DROP TABLE IF EXISTS pg_temp.catalog_wood_refs`); err != nil {
			t.Errorf("cleanup temp table: %v", err)
		}
		for _, table := range []string{"wood_types", "suppliers"} {
			if _, err := db.ExecContext(bg, `DELETE FROM `+table+` WHERE name LIKE $1`, tag+"%"); err != nil {
				t.Errorf("cleanup %s: %v", table, err)
			}
		}
	})

	woodTypes := NewWoodTypeRepository(db)
	suppliers := NewSupplierRepository(db)

	t.Run("wood types", func(t *testing.T) {
		b := &domain.WoodType{Name: tag + " B"}
		a1 := &domain.WoodType{Name: tag + " A"}
		a2 := &domain.WoodType{Name: tag + " A"} // same name: allowed
		for _, w := range []*domain.WoodType{b, a1, a2} {
			if err := woodTypes.Create(ctx, w); err != nil {
				t.Fatalf("create %q: %v", w.Name, err)
			}
			if w.ID == 0 || w.CreatedAt.IsZero() || !w.CreatedAt.Equal(w.UpdatedAt) {
				t.Fatalf("created %+v: want an ID and equal timestamps", w)
			}
		}

		// List: sorted by name, then ID (other tests may add rows; keep ours).
		list, err := woodTypes.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var ours []int64
		for _, w := range list {
			if strings.HasPrefix(w.Name, tag) {
				ours = append(ours, w.ID)
			}
		}
		if want := []int64{a1.ID, a2.ID, b.ID}; fmt.Sprint(ours) != fmt.Sprint(want) {
			t.Errorf("listed IDs %v, want %v (name, then ID)", ours, want)
		}

		got, err := woodTypes.GetByID(ctx, a1.ID)
		if err != nil || got.Name != a1.Name {
			t.Fatalf("GetByID = %+v, %v", got, err)
		}

		time.Sleep(10 * time.Millisecond) // so updated_at moves past created_at
		renamed := &domain.WoodType{ID: a1.ID, Name: tag + " C"}
		if err := woodTypes.Update(ctx, renamed); err != nil {
			t.Fatal(err)
		}
		if !renamed.CreatedAt.Equal(a1.CreatedAt) || !renamed.UpdatedAt.After(a1.UpdatedAt) {
			t.Errorf("after update created_at %v updated_at %v; want created_at kept, updated_at later than %v",
				renamed.CreatedAt, renamed.UpdatedAt, a1.UpdatedAt)
		}

		if err := woodTypes.Delete(ctx, a2.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := woodTypes.GetByID(ctx, a2.ID); !errors.Is(err, repository.ErrWoodTypeNotFound) {
			t.Errorf("GetByID after delete: %v", err)
		}
		if err := woodTypes.Delete(ctx, a2.ID); !errors.Is(err, repository.ErrWoodTypeNotFound) {
			t.Errorf("second delete: %v", err)
		}
		if err := woodTypes.Update(ctx, &domain.WoodType{ID: a2.ID, Name: "x"}); !errors.Is(err, repository.ErrWoodTypeNotFound) {
			t.Errorf("update deleted: %v", err)
		}

		// A row that references the wood type makes the delete fail with 23503.
		if _, err := db.ExecContext(ctx,
			`CREATE TEMP TABLE catalog_wood_refs (wood_type_id BIGINT NOT NULL REFERENCES wood_types(id))`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO catalog_wood_refs VALUES ($1)`, b.ID); err != nil {
			t.Fatal(err)
		}
		if err := woodTypes.Delete(ctx, b.ID); !errors.Is(err, repository.ErrInUse) {
			t.Errorf("delete referenced: err = %v, want ErrInUse", err)
		}
		if _, err := woodTypes.GetByID(ctx, b.ID); err != nil {
			t.Errorf("the referenced wood type was deleted: %v", err)
		}
		if _, err := db.ExecContext(ctx, `DROP TABLE pg_temp.catalog_wood_refs`); err != nil {
			t.Fatal(err)
		}
		if err := woodTypes.Delete(ctx, b.ID); err != nil {
			t.Errorf("delete once unreferenced: %v", err)
		}
	})

	t.Run("suppliers", func(t *testing.T) {
		phone := "0368122137"
		withPhone := &domain.Supplier{Name: tag + " An Phát", Phone: &phone}
		withoutPhone := &domain.Supplier{Name: tag + " Bình Minh"}
		for _, s := range []*domain.Supplier{withPhone, withoutPhone} {
			if err := suppliers.Create(ctx, s); err != nil {
				t.Fatalf("create %q: %v", s.Name, err)
			}
		}

		got, err := suppliers.GetByID(ctx, withPhone.ID)
		if err != nil || got.Phone == nil || *got.Phone != phone {
			t.Fatalf("GetByID = %+v, %v", got, err)
		}
		got, err = suppliers.GetByID(ctx, withoutPhone.ID)
		if err != nil || got.Phone != nil {
			t.Fatalf("GetByID without phone = %+v, %v", got, err)
		}

		// Remove the phone (NULL), then set another one.
		withPhone.Phone = nil
		if err := suppliers.Update(ctx, withPhone); err != nil {
			t.Fatal(err)
		}
		if got, _ := suppliers.GetByID(ctx, withPhone.ID); got.Phone != nil {
			t.Errorf("phone after removal = %q", *got.Phone)
		}
		other := "02223123456"
		withoutPhone.Phone = &other
		if err := suppliers.Update(ctx, withoutPhone); err != nil {
			t.Fatal(err)
		}
		if got, _ := suppliers.GetByID(ctx, withoutPhone.ID); got.Phone == nil || *got.Phone != other {
			t.Errorf("phone after update = %v", got.Phone)
		}

		list, err := suppliers.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, s := range list {
			if s.ID == withPhone.ID || s.ID == withoutPhone.ID {
				found++
			}
		}
		if found != 2 {
			t.Errorf("List contains %d of the 2 suppliers", found)
		}

		if err := suppliers.Delete(ctx, withPhone.ID); err != nil {
			t.Fatal(err)
		}
		if err := suppliers.Delete(ctx, withPhone.ID); !errors.Is(err, repository.ErrSupplierNotFound) {
			t.Errorf("second delete: %v", err)
		}
	})

	t.Run("CHECK constraints refuse data that bypasses the use case", func(t *testing.T) {
		badPhones := []string{"12345", "0036812213", "+84368122137", "0368 122 137", "036812213799"}
		for _, p := range badPhones {
			p := p
			err := suppliers.Create(ctx, &domain.Supplier{Name: tag + " bad phone", Phone: &p})
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "suppliers_phone_check" {
				t.Errorf("phone %q: err = %v, want a suppliers_phone_check violation", p, err)
			}
		}
		for name, value := range map[string]string{"blank": "   ", "101 characters": tag + strings.Repeat("x", 101)} {
			w := &domain.WoodType{Name: value}
			err := woodTypes.Create(ctx, w)
			if err == nil {
				// Not expected; remove the row now (the blank name has no tag).
				_ = woodTypes.Delete(ctx, w.ID)
			}
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "wood_types_name_check" {
				t.Errorf("wood type name %s: err = %v, want a wood_types_name_check violation", name, err)
			}
		}
	})
}
