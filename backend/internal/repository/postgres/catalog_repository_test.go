package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"

	"gostock/internal/domain"
	"gostock/internal/repository"
)

const (
	woodTypeListSQL   = `SELECT id, name, created_at, updated_at\s+FROM wood_types\s+ORDER BY name, id`
	woodTypeGetSQL    = `SELECT id, name, created_at, updated_at\s+FROM wood_types\s+WHERE id = \$1`
	woodTypeInsertSQL = `INSERT INTO wood_types \(name\)\s+VALUES \(\$1\)\s+RETURNING id, created_at, updated_at`
	woodTypeUpdateSQL = `UPDATE wood_types\s+SET name = \$1,\s+updated_at = NOW\(\)\s+WHERE id = \$2\s+RETURNING created_at, updated_at`
	woodTypeDeleteSQL = `DELETE FROM wood_types\s+WHERE id = \$1`

	supplierListSQL   = `SELECT id, name, phone, created_at, updated_at\s+FROM suppliers\s+ORDER BY name, id`
	supplierGetSQL    = `SELECT id, name, phone, created_at, updated_at\s+FROM suppliers\s+WHERE id = \$1`
	supplierInsertSQL = `INSERT INTO suppliers \(name, phone\)\s+VALUES \(\$1, \$2\)\s+RETURNING id, created_at, updated_at`
	supplierUpdateSQL = `UPDATE suppliers\s+SET name = \$1,\s+phone = \$2,\s+updated_at = NOW\(\)\s+WHERE id = \$3\s+RETURNING created_at, updated_at`
	supplierDeleteSQL = `DELETE FROM suppliers\s+WHERE id = \$1`
)

// newCatalogMock returns a sqlmock database whose expectations are checked
// when the test ends.
func newCatalogMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
		_ = db.Close()
	})
	return db, mock
}

var (
	catalogCreated = time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	catalogUpdated = catalogCreated.Add(time.Hour)
	fkViolation    = &pgconn.PgError{Code: "23503", ConstraintName: "pieces_wood_type_id_fkey"}
)

func TestWoodTypeRepository_SQL(t *testing.T) {
	ctx := context.Background()

	t.Run("list sorted by name", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectQuery(woodTypeListSQL).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_at", "updated_at"}).
			AddRow(2, "Gỗ lim", catalogCreated, catalogUpdated).AddRow(1, "Gỗ sến", catalogCreated, catalogCreated))
		got, err := NewWoodTypeRepository(db).List(ctx)
		if err != nil || len(got) != 2 || got[0].ID != 2 || !got[0].UpdatedAt.Equal(catalogUpdated) {
			t.Fatalf("List = %+v, %v", got, err)
		}
	})

	t.Run("empty list is not nil", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectQuery(woodTypeListSQL).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "created_at", "updated_at"}))
		got, err := NewWoodTypeRepository(db).List(ctx)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("List = %#v, %v", got, err)
		}
	})

	t.Run("get missing is ErrWoodTypeNotFound", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectQuery(woodTypeGetSQL).WithArgs(int64(9)).WillReturnError(sql.ErrNoRows)
		if _, err := NewWoodTypeRepository(db).GetByID(ctx, 9); !errors.Is(err, repository.ErrWoodTypeNotFound) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("create sets ID and timestamps", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectQuery(woodTypeInsertSQL).WithArgs("Gỗ lim").
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(5, catalogCreated, catalogCreated))
		w := &domain.WoodType{Name: "Gỗ lim"}
		if err := NewWoodTypeRepository(db).Create(ctx, w); err != nil || w.ID != 5 || !w.CreatedAt.Equal(catalogCreated) {
			t.Fatalf("Create: %v, %+v", err, w)
		}
	})

	t.Run("update writes the name and updated_at", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectQuery(woodTypeUpdateSQL).WithArgs("Gỗ sến", int64(5)).
			WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(catalogCreated, catalogUpdated))
		w := &domain.WoodType{ID: 5, Name: "Gỗ sến"}
		if err := NewWoodTypeRepository(db).Update(ctx, w); err != nil || !w.UpdatedAt.Equal(catalogUpdated) {
			t.Fatalf("Update: %v, %+v", err, w)
		}
	})

	t.Run("update missing is ErrWoodTypeNotFound", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectQuery(woodTypeUpdateSQL).WithArgs("x", int64(9)).WillReturnError(sql.ErrNoRows)
		if err := NewWoodTypeRepository(db).Update(ctx, &domain.WoodType{ID: 9, Name: "x"}); !errors.Is(err, repository.ErrWoodTypeNotFound) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectExec(woodTypeDeleteSQL).WithArgs(int64(5)).WillReturnResult(sqlmock.NewResult(0, 1))
		if err := NewWoodTypeRepository(db).Delete(ctx, 5); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("delete missing is ErrWoodTypeNotFound", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectExec(woodTypeDeleteSQL).WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 0))
		if err := NewWoodTypeRepository(db).Delete(ctx, 9); !errors.Is(err, repository.ErrWoodTypeNotFound) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("delete referenced (23503) is ErrInUse", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectExec(woodTypeDeleteSQL).WithArgs(int64(5)).WillReturnError(fkViolation)
		if err := NewWoodTypeRepository(db).Delete(ctx, 5); !errors.Is(err, repository.ErrInUse) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("delete other database error unchanged", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		other := &pgconn.PgError{Code: "57014"} // query_canceled
		mock.ExpectExec(woodTypeDeleteSQL).WithArgs(int64(5)).WillReturnError(other)
		if err := NewWoodTypeRepository(db).Delete(ctx, 5); !errors.Is(err, other) || errors.Is(err, repository.ErrInUse) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestSupplierRepository_SQL(t *testing.T) {
	ctx := context.Background()
	phone := "0368122137"

	t.Run("list maps NULL phone to nil", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectQuery(supplierListSQL).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "phone", "created_at", "updated_at"}).
			AddRow(1, "An Phát", phone, catalogCreated, catalogCreated).AddRow(2, "Bình Minh", nil, catalogCreated, catalogCreated))
		got, err := NewSupplierRepository(db).List(ctx)
		if err != nil || len(got) != 2 {
			t.Fatalf("List = %+v, %v", got, err)
		}
		if got[0].Phone == nil || *got[0].Phone != phone || got[1].Phone != nil {
			t.Errorf("phones = %v, %v", got[0].Phone, got[1].Phone)
		}
	})

	t.Run("get missing is ErrSupplierNotFound", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectQuery(supplierGetSQL).WithArgs(int64(9)).WillReturnError(sql.ErrNoRows)
		if _, err := NewSupplierRepository(db).GetByID(ctx, 9); !errors.Is(err, repository.ErrSupplierNotFound) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("create with and without phone", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectQuery(supplierInsertSQL).WithArgs("An Phát", phone).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(3, catalogCreated, catalogCreated))
		mock.ExpectQuery(supplierInsertSQL).WithArgs("Bình Minh", nil).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(4, catalogCreated, catalogCreated))
		repo := NewSupplierRepository(db)
		withPhone := &domain.Supplier{Name: "An Phát", Phone: &phone}
		withoutPhone := &domain.Supplier{Name: "Bình Minh"}
		if err := repo.Create(ctx, withPhone); err != nil || withPhone.ID != 3 {
			t.Fatalf("with phone: %v, %+v", err, withPhone)
		}
		if err := repo.Create(ctx, withoutPhone); err != nil || withoutPhone.ID != 4 {
			t.Fatalf("without phone: %v, %+v", err, withoutPhone)
		}
	})

	t.Run("update writes name, phone (NULL) and updated_at", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectQuery(supplierUpdateSQL).WithArgs("An Phát", nil, int64(3)).
			WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(catalogCreated, catalogUpdated))
		s := &domain.Supplier{ID: 3, Name: "An Phát"}
		if err := NewSupplierRepository(db).Update(ctx, s); err != nil || !s.UpdatedAt.Equal(catalogUpdated) {
			t.Fatalf("Update: %v, %+v", err, s)
		}
	})

	t.Run("update missing is ErrSupplierNotFound", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectQuery(supplierUpdateSQL).WithArgs("x", nil, int64(9)).WillReturnError(sql.ErrNoRows)
		if err := NewSupplierRepository(db).Update(ctx, &domain.Supplier{ID: 9, Name: "x"}); !errors.Is(err, repository.ErrSupplierNotFound) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("delete missing is ErrSupplierNotFound", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectExec(supplierDeleteSQL).WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 0))
		if err := NewSupplierRepository(db).Delete(ctx, 9); !errors.Is(err, repository.ErrSupplierNotFound) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("delete referenced (23503) is ErrInUse", func(t *testing.T) {
		db, mock := newCatalogMock(t)
		mock.ExpectExec(supplierDeleteSQL).WithArgs(int64(3)).WillReturnError(fkViolation)
		if err := NewSupplierRepository(db).Delete(ctx, 3); !errors.Is(err, repository.ErrInUse) {
			t.Fatalf("err = %v", err)
		}
	})
}
