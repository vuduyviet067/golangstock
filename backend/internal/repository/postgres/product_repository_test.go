package postgres

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
)

const productUpdateSQL = `UPDATE products SET sku = \$1, name = \$2, price = \$3, minimum_stock = COALESCE\(\$5, minimum_stock\), updated_at = NOW\(\) WHERE id = \$4 RETURNING quantity, minimum_stock`

// setQuantityPattern matches any assignment to the quantity column.
var setQuantityPattern = regexp.MustCompile(`(?i)quantity\s*=`)

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
		WithArgs("SKU001", "Renamed", 100.5, int64(1), nil).
		WillReturnRows(sqlmock.NewRows([]string{"quantity", "minimum_stock"}).AddRow(7, 4))

	product := &domain.Product{
		ID:       1,
		SKU:      "SKU001",
		Name:     "Renamed",
		Price:    100.5,
		Quantity: 10, // stale value; must be neither sent nor kept
	}

	if err := repo.Update(context.Background(), product, nil); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if setQuantityPattern.MatchString(executedSQL) {
		t.Errorf("UPDATE writes quantity: %s", executedSQL)
	}
	if product.Quantity != 7 {
		t.Errorf("quantity = %d, want stored 7 from RETURNING", product.Quantity)
	}
	if product.MinimumStock != 4 {
		t.Errorf("minimum stock = %d, want stored 4 from RETURNING", product.MinimumStock)
	}
	if product.SKU != "SKU001" || product.Name != "Renamed" || product.Price != 100.5 {
		t.Errorf("editable fields changed unexpectedly: %+v", product)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

// TestProductRepository_Update_MinimumStockArgument checks how the optional
// threshold is bound: nil binds NULL (COALESCE keeps the stored value) and a
// value, including 0, is bound as is.
func TestProductRepository_Update_MinimumStockArgument(t *testing.T) {
	zero, three := 0, 3

	tests := []struct {
		name    string
		arg     *int
		wantArg any
	}{
		{"nil binds NULL", nil, nil},
		{"zero binds 0", &zero, int64(0)},
		{"value binds value", &three, int64(3)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to create sqlmock: %v", err)
			}
			defer db.Close()

			mock.ExpectQuery(productUpdateSQL).
				WithArgs("SKU001", "Keyboard", 100.0, int64(1), tt.wantArg).
				WillReturnRows(sqlmock.NewRows([]string{"quantity", "minimum_stock"}).AddRow(2, 3))

			product := &domain.Product{ID: 1, SKU: "SKU001", Name: "Keyboard", Price: 100}
			if err := NewProductRepository(db).Update(context.Background(), product, tt.arg); err != nil {
				t.Fatalf("Update returned error: %v", err)
			}
			if product.MinimumStock != 3 {
				t.Errorf("minimum stock = %d, want 3 from RETURNING", product.MinimumStock)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet sql expectations: %v", err)
			}
		})
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
					WithArgs("SKU001", "Keyboard", 100.0, int64(99), nil).
					WillReturnRows(sqlmock.NewRows([]string{"quantity", "minimum_stock"}))
			},
			wantErr: repository.ErrProductNotFound,
		},
		{
			name: "duplicate SKU maps to ErrProductSKUExists",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(productUpdateSQL).
					WithArgs("SKU001", "Keyboard", 100.0, int64(99), nil).
					WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "products_sku_key"})
			},
			wantErr: repository.ErrProductSKUExists,
		},
		{
			name: "other database errors are returned unchanged",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(productUpdateSQL).
					WithArgs("SKU001", "Keyboard", 100.0, int64(99), nil).
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
			err = NewProductRepository(db).Update(context.Background(), product, nil)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet sql expectations: %v", err)
			}
		})
	}
}
