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
			req.Header.Set("Content-Type", "application/json")
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

// TestProductUpdateCannotRestoreStaleStock reproduces the CR-15 lost update
// against PostgreSQL: a product edit made after a stock-out must keep the
// current stock, and a PUT carrying a stale quantity is rejected unchanged.
func TestProductUpdateCannotRestoreStaleStock(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}

	sku := fmt.Sprintf("CR15-%d", time.Now().UnixNano())

	var id int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO products (sku, name, price, quantity)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, sku, "Stale Stock Test", 100, 10).Scan(&id)
	if err != nil {
		t.Fatalf("create test product: %v", err)
	}
	defer func() {
		if _, err := db.Exec(`DELETE FROM products WHERE id = $1`, id); err != nil {
			t.Errorf("cleanup test product: %v", err)
		}
	}()

	handler := NewProductHandler(usecase.NewProductUseCase(postgres.NewProductRepository(db)))
	productPath := fmt.Sprintf("/api/v1/products/%d", id)

	send := func(method, path, body string, serve func(stdhttp.ResponseWriter, *stdhttp.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		serve(rec, req)
		return rec
	}

	readProduct := func() (string, int) {
		var name string
		var quantity int
		if err := db.QueryRowContext(ctx,
			`SELECT name, quantity FROM products WHERE id = $1`, id,
		).Scan(&name, &quantity); err != nil {
			t.Fatalf("read product: %v", err)
		}
		return name, quantity
	}

	// 1. Another operator stocks out 3 units: 10 -> 7.
	if rec := send(stdhttp.MethodPost, productPath+"/stock-out", `{"quantity":3}`, handler.StockOut); rec.Code != stdhttp.StatusOK {
		t.Fatalf("stock-out status = %d; body = %s", rec.Code, rec.Body.String())
	}

	// 2. The admin's edit form still holds quantity 10; it must be rejected.
	rec := send(stdhttp.MethodPut, productPath,
		fmt.Sprintf(`{"sku":%q,"name":"Stale Edit","price":100,"quantity":10}`, sku), handler.Update)
	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("PUT with quantity status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	if name, quantity := readProduct(); name != "Stale Stock Test" || quantity != 7 {
		t.Fatalf("after rejected PUT: name=%q quantity=%d, want unchanged name and 7", name, quantity)
	}

	// 3. The same edit without quantity succeeds and keeps the current stock.
	rec = send(stdhttp.MethodPut, productPath,
		fmt.Sprintf(`{"sku":%q,"name":"Renamed","price":100}`, sku), handler.Update)
	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("PUT without quantity status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"quantity":7`) {
		t.Errorf("PUT response = %s, want stored quantity 7", rec.Body.String())
	}
	if name, quantity := readProduct(); name != "Renamed" || quantity != 7 {
		t.Errorf("after PUT: name=%q quantity=%d, want Renamed and 7", name, quantity)
	}
}
