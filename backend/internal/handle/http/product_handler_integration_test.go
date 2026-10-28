package http

import (
	"context"
	"database/sql"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	stdhttp "net/http"

	"gostock/internal/repository/postgres"
	"gostock/internal/usecase"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestProductHandlerConcurrentStockOut(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}

	sku := fmt.Sprintf("INTEGRATION-%d", time.Now().UnixNano())

	var id int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO products (sku, name, price, quantity)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, sku, "Concurrent Stock Test", 100, 5).Scan(&id)
	if err != nil {
		t.Fatalf("create test product: %v", err)
	}

	defer func() {
		_, err := db.Exec(
			`DELETE FROM products WHERE id = $1`, id,
		)
		if err != nil {
			t.Errorf("cleanup test product: %v", err)
		}
	}()

	repo := postgres.NewProductRepository(db)
	uc := usecase.NewProductUseCase(repo)
	handler := NewProductHandler(uc)

	const customers = 2
	start := make(chan struct{})
	statuses := make(chan int, customers)
	errs := make(chan error, customers)

	var wg sync.WaitGroup

	for i := 0; i < customers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start

			path := fmt.Sprintf(
				"/api/v1/products/%d/stock-out", id,
			)
			req := httptest.NewRequest(
				stdhttp.MethodPost,
				path,
				strings.NewReader(`{"quantity":4}`),
			)
			rec := httptest.NewRecorder()

			handler.StockOut(rec, req)

			statuses <- rec.Code
		}()
	}

	close(start)
	wg.Wait()
	close(statuses)
	close(errs)

	successCount := 0
	conflictCount := 0

	for status := range statuses {
		switch status {
		case stdhttp.StatusOK:
			successCount++
		case stdhttp.StatusConflict:
			conflictCount++
		default:
			t.Errorf("unexpected HTTP status: %d", status)
		}
	}

	for err := range errs {
		t.Error(err)
	}

	if successCount != 1 {
		t.Errorf("expected 1 success, got %d", successCount)
	}
	if conflictCount != 1 {
		t.Errorf("expected 1 conflict, got %d", conflictCount)
	}

	var quantity int
	err = db.QueryRowContext(
		context.Background(),
		`SELECT quantity FROM products WHERE id = $1`,
		id,
	).Scan(&quantity)
	if err != nil {
		t.Fatalf("read final quantity: %v", err)
	}

	if quantity != 1 {
		t.Errorf("expected final quantity 1, got %d", quantity)
	}
}
