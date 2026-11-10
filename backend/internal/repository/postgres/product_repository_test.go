package postgres

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"gostock/internal/domain"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
)

const productUpdateSQL = `UPDATE products SET sku = \$1, name = \$2, price = \$3, updated_at = NOW\(\) WHERE id = \$4 RETURNING quantity`

// setQuantityPattern matches any assignment to the quantity column.
var setQuantityPattern = regexp.MustCompile(`(?i)quantity\s*=`)

// TestProductRepository_Update_DoesNotWriteQuantity guards CR-15: product
// edits must never write stock. The statement has no quantity assignment, takes
// exactly four arguments, and reports the stored quantity via RETURNING.
func TestProductRepository_Update_DoesNotWriteQuantity(t *testing.T) {
	var executedSQL string
	matcher := sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
		executedSQL = actualSQL
		return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
	})

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewProductRepository(db)

	// WithArgs fails the expectation if any extra argument (such as a
	// quantity) is passed.
	mock.ExpectQuery(productUpdateSQL).
		WithArgs("SKU001", "Renamed", 100.5, int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(7))

	product := &domain.Product{
		ID:       1,
		SKU:      "SKU001",
		Name:     "Renamed",
		Price:    100.5,
		Quantity: 10, // stale value; must be neither sent nor kept
	}

	if err := repo.Update(context.Background(), product); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if setQuantityPattern.MatchString(executedSQL) {
		t.Errorf("UPDATE writes quantity: %s", executedSQL)
	}
	if product.Quantity != 7 {
		t.Errorf("quantity = %d, want stored 7 from RETURNING", product.Quantity)
	}
	if product.SKU != "SKU001" || product.Name != "Renamed" || product.Price != 100.5 {
		t.Errorf("editable fields changed unexpectedly: %+v", product)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestProductRepository_Update_Errors(t *testing.T) {
	dbFailure := errors.New("connection reset")

	tests := []struct {
		name    string
		setup   func(mock sqlmock.Sqlmock)
		wantErr error
	}{
		{
			name: "missing product maps to ErrProductNotFound",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(productUpdateSQL).
					WithArgs("SKU001", "Keyboard", 100.0, int64(99)).
					WillReturnRows(sqlmock.NewRows([]string{"quantity"}))
			},
			wantErr: domain.ErrProductNotFound,
		},
		{
			name: "duplicate SKU maps to ErrProductSKUExists",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(productUpdateSQL).
					WithArgs("SKU001", "Keyboard", 100.0, int64(99)).
					WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "products_sku_key"})
			},
			wantErr: domain.ErrProductSKUExists,
		},
		{
			name: "other database errors are returned unchanged",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(productUpdateSQL).
					WithArgs("SKU001", "Keyboard", 100.0, int64(99)).
					WillReturnError(dbFailure)
			},
			wantErr: dbFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to create sqlmock: %v", err)
			}
			defer db.Close()

			tt.setup(mock)

			product := &domain.Product{ID: 99, SKU: "SKU001", Name: "Keyboard", Price: 100}
			err = NewProductRepository(db).Update(context.Background(), product)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet sql expectations: %v", err)
			}
		})
	}
}
