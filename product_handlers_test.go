package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("Skipping integration test: DATABASE_URL is not set")
	}

	db, err := openDatabase(dsn)
	if err != nil {
		t.Fatalf("cannot connect to test database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func createTestProduct(t *testing.T, db *sql.DB, quantity int) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sku := fmt.Sprintf("TEST-%d", time.Now().UnixNano())

	var id int64
	err := db.QueryRowContext(
		ctx,
		`INSERT INTO products (sku, name, price, quantity)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		sku,
		"Test Product",
		10000,
		quantity,
	).Scan(&id)

	if err != nil {
		t.Fatalf("cannot create test product: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.ExecContext(
			context.Background(),
			`DELETE FROM products WHERE id = $1`,
			id,
		)
		if err != nil {
			t.Errorf("cannot clean up test product %d: %v", id, err)
		}
	})

	return id
}

func sendStockRequest(
	handler http.HandlerFunc,
	path string,
	body string,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(
		http.MethodPost,
		path,
		bytes.NewBufferString(body),
	)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler(rec, req)

	return rec
}

func assertQuantity(t *testing.T, db *sql.DB, id int64, want int) {
	t.Helper()

	var got int
	err := db.QueryRow(
		`SELECT quantity FROM products WHERE id = $1`,
		id,
	).Scan(&got)

	if err != nil {
		t.Fatalf("cannot read product quantity: %v", err)
	}

	if got != want {
		t.Fatalf("expected quantity %d, got %d", want, got)
	}
}

func TestStockInSuccess(t *testing.T) {
	db := openTestDB(t)
	id := createTestProduct(t, db, 10)

	rec := sendStockRequest(
		stockInHandler(db),
		fmt.Sprintf("/api/v1/products/%d/stock-in", id),
		`{"quantity":5}`,
	)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	assertQuantity(t, db, id, 15)
}

func TestStockOutSuccess(t *testing.T) {
	db := openTestDB(t)
	id := createTestProduct(t, db, 10)

	rec := sendStockRequest(
		stockOutHandler(db),
		fmt.Sprintf("/api/v1/products/%d/stock-out", id),
		`{"quantity":3}`,
	)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	assertQuantity(t, db, id, 7)
}

func TestStockOutInsufficientStock(t *testing.T) {
	db := openTestDB(t)
	id := createTestProduct(t, db, 5)

	rec := sendStockRequest(
		stockOutHandler(db),
		fmt.Sprintf("/api/v1/products/%d/stock-out", id),
		`{"quantity":8}`,
	)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d: %s", rec.Code, rec.Body.String())
	}

	assertQuantity(t, db, id, 5)
}

func TestStockInRejectsZeroQuantity(t *testing.T) {
	db := openTestDB(t)
	id := createTestProduct(t, db, 10)

	rec := sendStockRequest(
		stockInHandler(db),
		fmt.Sprintf("/api/v1/products/%d/stock-in", id),
		`{"quantity":0}`,
	)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}

	assertQuantity(t, db, id, 10)
}

func TestStockOutRejectsNegativeQuantity(t *testing.T) {
	db := openTestDB(t)
	id := createTestProduct(t, db, 10)

	rec := sendStockRequest(
		stockOutHandler(db),
		fmt.Sprintf("/api/v1/products/%d/stock-out", id),
		`{"quantity":-2}`,
	)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}

	assertQuantity(t, db, id, 10)
}

func TestStockOutProductNotFound(t *testing.T) {
	db := openTestDB(t)

	rec := sendStockRequest(
		stockOutHandler(db),
		"/api/v1/products/999999999/stock-out",
		`{"quantity":1}`,
	)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestConcurrentStockOut(t *testing.T) {
	db := openTestDB(t)
	id := createTestProduct(t, db, 5)

	const customers = 2
	start := make(chan struct{})
	var wg sync.WaitGroup

	statuses := make(chan int, customers)

	for i := 0; i < customers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Đợi đến khi cả hai goroutine sẵn sàng.
			<-start

			rec := sendStockRequest(
				stockOutHandler(db),
				fmt.Sprintf("/api/v1/products/%d/stock-out", id),
				`{"quantity":4}`,
			)

			statuses <- rec.Code
		}()
	}

	// Cho hai request bắt đầu gần như đồng thời.
	close(start)
	wg.Wait()
	close(statuses)

	successCount := 0
	conflictCount := 0

	for status := range statuses {
		switch status {
		case http.StatusOK:
			successCount++
		case http.StatusConflict:
			conflictCount++
		default:
			t.Errorf("unexpected HTTP status: %d", status)
		}
	}

	if successCount != 1 {
		t.Errorf("expected 1 successful request, got %d", successCount)
	}

	if conflictCount != 1 {
		t.Errorf("expected 1 conflict, got %d", conflictCount)
	}

	assertQuantity(t, db, id, 1)
}
