package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	stdhttp "net/http"

	"gostock/internal/dto"
	"gostock/internal/repository/postgres"
	"gostock/internal/testdb"
	"gostock/internal/usecase"
)

func TestProductHandlerConcurrentStockOut(t *testing.T) {
	db := testdb.Open(t)

	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	sku := fmt.Sprintf("INTEGRATION-%d", time.Now().UnixNano())

	var id int64
	err := db.QueryRowContext(ctx, `
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
	actorID := createIntegrationActor(t, db)

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

			handler.StockOut(rec, withUserID(req, actorID))

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

	// Only the successful stock-out is recorded; the rejected one left no row.
	movements := readMovements(t, db, sku)
	if len(movements) != 1 {
		t.Fatalf("movements = %+v, want exactly one", movements)
	}
	want := movementRow{Type: "out", Change: -4, Before: 5, After: 1, UserID: actorID}
	if movements[0] != want {
		t.Errorf("movement = %+v, want %+v", movements[0], want)
	}
}

// createIntegrationActor inserts a user to act as the authenticated actor of
// stock changes. Cleanup removes the user's stock movements first (the user
// foreign key is ON DELETE RESTRICT), then the user; it runs after the
// test's deferred product cleanup.
func createIntegrationActor(t *testing.T, db *sql.DB) int64 {
	t.Helper()

	var id int64
	email := fmt.Sprintf("stock-actor-%d@example.test", time.Now().UnixNano())
	if err := db.QueryRow(
		`INSERT INTO users (email, password_hash, role) VALUES ($1, 'x', 'user') RETURNING id`, email,
	).Scan(&id); err != nil {
		t.Fatalf("create actor: %v", err)
	}

	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM stock_movements WHERE user_id = $1`, id); err != nil {
			t.Errorf("cleanup actor movements: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM users WHERE id = $1`, id); err != nil {
			t.Errorf("cleanup actor: %v", err)
		}
	})
	return id
}

// movementRow is the part of a stock movement the integration tests compare.
type movementRow struct {
	Type   string
	Change int
	Before int
	After  int
	UserID int64
}

// readMovements returns the movements recorded for sku in insertion order.
func readMovements(t *testing.T, db *sql.DB, sku string) []movementRow {
	t.Helper()

	rows, err := db.Query(`
		SELECT movement_type, quantity_change, quantity_before, quantity_after, COALESCE(user_id, 0)
		FROM stock_movements WHERE product_sku = $1 ORDER BY id`, sku)
	if err != nil {
		t.Fatalf("read movements: %v", err)
	}
	defer rows.Close()

	var movements []movementRow
	for rows.Next() {
		var m movementRow
		if err := rows.Scan(&m.Type, &m.Change, &m.Before, &m.After, &m.UserID); err != nil {
			t.Fatalf("scan movement: %v", err)
		}
		movements = append(movements, m)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("movements: %v", err)
	}
	return movements
}

// TestProductUpdateCannotRestoreStaleStock reproduces the CR-15 lost update
// against PostgreSQL: a product edit made after a stock-out must keep the
// current stock, and a PUT carrying a stale quantity is rejected unchanged.
func TestProductUpdateCannotRestoreStaleStock(t *testing.T) {
	db := testdb.Open(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sku := fmt.Sprintf("CR15-%d", time.Now().UnixNano())

	var id int64
	err := db.QueryRowContext(ctx, `
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
	actorID := createIntegrationActor(t, db)
	productPath := fmt.Sprintf("/api/v1/products/%d", id)

	send := func(method, path, body string, serve func(stdhttp.ResponseWriter, *stdhttp.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		serve(rec, withUserID(req, actorID))
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

// TestProductMinimumStockRoundTrip checks minimum_stock against PostgreSQL
// through the real handler, use case and repository: create stores it, a PUT
// without the field or with null keeps it, 0 and positive values replace it, a
// negative value is rejected without changing anything, and quantity is never
// written by PUT.
func TestProductMinimumStockRoundTrip(t *testing.T) {
	db := testdb.Open(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sku := fmt.Sprintf("MINSTOCK-%d", time.Now().UnixNano())
	handler := NewProductHandler(usecase.NewProductUseCase(postgres.NewProductRepository(db)))
	actorID := createIntegrationActor(t, db)

	send := func(method, path, body string, serve func(stdhttp.ResponseWriter, *stdhttp.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		serve(rec, withUserID(req, actorID))
		return rec
	}

	decodeProduct := func(rec *httptest.ResponseRecorder) dto.ProductResponse {
		var envelope struct {
			Data dto.ProductResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode response %s: %v", rec.Body.String(), err)
		}
		return envelope.Data
	}

	// 1. Create with an explicit threshold.
	rec := send(stdhttp.MethodPost, "/api/v1/products",
		fmt.Sprintf(`{"sku":%q,"name":"Min Stock Test","price":10,"quantity":6,"minimum_stock":3}`, sku), handler.Create)
	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("create status = %d; body = %s", rec.Code, rec.Body.String())
	}
	created := decodeProduct(rec)
	id := created.ID
	defer func() {
		if _, err := db.Exec(`DELETE FROM products WHERE id = $1`, id); err != nil {
			t.Errorf("cleanup test product: %v", err)
		}
	}()
	if created.MinimumStock != 3 || created.Quantity != 6 {
		t.Fatalf("created product = %+v, want minimum_stock 3 and quantity 6", created)
	}

	productPath := fmt.Sprintf("/api/v1/products/%d", id)

	readStored := func() (int, int) {
		var quantity, minimumStock int
		if err := db.QueryRowContext(ctx,
			`SELECT quantity, minimum_stock FROM products WHERE id = $1`, id,
		).Scan(&quantity, &minimumStock); err != nil {
			t.Fatalf("read product: %v", err)
		}
		return quantity, minimumStock
	}

	steps := []struct {
		name        string
		body        string
		wantStatus  int
		wantMinimum int
	}{
		{"missing keeps value", fmt.Sprintf(`{"sku":%q,"name":"Renamed","price":11}`, sku), stdhttp.StatusOK, 3},
		{"null keeps value", fmt.Sprintf(`{"sku":%q,"name":"Renamed","price":11,"minimum_stock":null}`, sku), stdhttp.StatusOK, 3},
		{"zero replaces value", fmt.Sprintf(`{"sku":%q,"name":"Renamed","price":11,"minimum_stock":0}`, sku), stdhttp.StatusOK, 0},
		{"positive replaces value", fmt.Sprintf(`{"sku":%q,"name":"Renamed","price":11,"minimum_stock":5}`, sku), stdhttp.StatusOK, 5},
		{"negative is rejected", fmt.Sprintf(`{"sku":%q,"name":"Renamed","price":11,"minimum_stock":-1}`, sku), stdhttp.StatusBadRequest, 5},
	}

	for _, step := range steps {
		rec := send(stdhttp.MethodPut, productPath, step.body, handler.Update)
		if rec.Code != step.wantStatus {
			t.Fatalf("%s: PUT status = %d, want %d; body = %s", step.name, rec.Code, step.wantStatus, rec.Body.String())
		}
		if step.wantStatus == stdhttp.StatusOK {
			if got := decodeProduct(rec); got.MinimumStock != step.wantMinimum || got.Quantity != 6 {
				t.Errorf("%s: PUT response = %+v, want minimum_stock %d and quantity 6", step.name, got, step.wantMinimum)
			}
		}
		quantity, minimumStock := readStored()
		if minimumStock != step.wantMinimum || quantity != 6 {
			t.Errorf("%s: stored quantity=%d minimum_stock=%d, want 6 and %d", step.name, quantity, minimumStock, step.wantMinimum)
		}
	}

	// 2. Reads and stock operations report the stored threshold.
	if got := decodeProduct(send(stdhttp.MethodGet, productPath, "", handler.GetByID)); got.MinimumStock != 5 {
		t.Errorf("GET minimum_stock = %d, want 5", got.MinimumStock)
	}
	if got := decodeProduct(send(stdhttp.MethodPost, productPath+"/stock-in", `{"quantity":2}`, handler.StockIn)); got.MinimumStock != 5 || got.Quantity != 8 {
		t.Errorf("stock-in response = %+v, want minimum_stock 5 and quantity 8", got)
	}
	if got := decodeProduct(send(stdhttp.MethodPost, productPath+"/stock-out", `{"quantity":1}`, handler.StockOut)); got.MinimumStock != 5 || got.Quantity != 7 {
		t.Errorf("stock-out response = %+v, want minimum_stock 5 and quantity 7", got)
	}

	listRec := send(stdhttp.MethodGet, "/api/v1/products?q="+sku, "", handler.List)
	var list struct {
		Data []dto.ProductResponse `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list %s: %v", listRec.Body.String(), err)
	}
	if len(list.Data) != 1 || list.Data[0].MinimumStock != 5 {
		t.Errorf("list = %+v, want one product with minimum_stock 5", list.Data)
	}

	// 3. Every stock change, and nothing else, was recorded: the five PUTs
	// (including the rejected one) and the reads added no movement.
	want := []movementRow{
		{Type: "initial", Change: 6, Before: 0, After: 6, UserID: actorID},
		{Type: "in", Change: 2, Before: 6, After: 8, UserID: actorID},
		{Type: "out", Change: -1, Before: 8, After: 7, UserID: actorID},
	}
	got := readMovements(t, db, sku)
	if len(got) != len(want) {
		t.Fatalf("movements = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("movement %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestProductCreateWithoutMinimumStockDefaultsToZero checks the backward
// compatible create path: an existing client that never sends minimum_stock
// stores 0.
func TestProductCreateWithoutMinimumStockDefaultsToZero(t *testing.T) {
	db := testdb.Open(t)

	sku := fmt.Sprintf("MINSTOCK-DEFAULT-%d", time.Now().UnixNano())
	handler := NewProductHandler(usecase.NewProductUseCase(postgres.NewProductRepository(db)))
	actorID := createIntegrationActor(t, db)

	req := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/products",
		strings.NewReader(fmt.Sprintf(`{"sku":%q,"name":"Default Min","price":1,"quantity":0}`, sku)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Create(rec, withUserID(req, actorID))

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("create status = %d; body = %s", rec.Code, rec.Body.String())
	}
	defer func() {
		if _, err := db.Exec(`DELETE FROM products WHERE sku = $1`, sku); err != nil {
			t.Errorf("cleanup test product: %v", err)
		}
	}()

	var minimumStock int
	if err := db.QueryRow(`SELECT minimum_stock FROM products WHERE sku = $1`, sku).Scan(&minimumStock); err != nil {
		t.Fatalf("read product: %v", err)
	}
	if minimumStock != 0 {
		t.Errorf("stored minimum_stock = %d, want 0", minimumStock)
	}
	if !strings.Contains(rec.Body.String(), `"minimum_stock":0`) {
		t.Errorf("create response = %s, want minimum_stock 0", rec.Body.String())
	}

	// A product created without stock has no opening movement.
	if movements := readMovements(t, db, sku); len(movements) != 0 {
		t.Errorf("movements = %+v, want none for a product created with quantity 0", movements)
	}
}

// TestProductHandlerConcurrentStockMovementsConsistent runs many concurrent
// stock-ins and stock-outs on one product through the real handler, use case
// and repository, then checks that the movement history exactly explains the
// final quantity: one row per successful request (none for rejected ones),
// a gap-free before/after chain in id order, no negative stock, and
// SUM(quantity_change) equal to the stored quantity.
func TestProductHandlerConcurrentStockMovementsConsistent(t *testing.T) {
	db := testdb.Open(t)

	sku := fmt.Sprintf("CONCURRENT-MOVES-%d", time.Now().UnixNano())
	handler := NewProductHandler(usecase.NewProductUseCase(postgres.NewProductRepository(db)))
	actorID := createIntegrationActor(t, db)

	post := func(path, body string, serve func(stdhttp.ResponseWriter, *stdhttp.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest(stdhttp.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		serve(rec, withUserID(req, actorID))
		return rec
	}

	rec := post("/api/v1/products",
		fmt.Sprintf(`{"sku":%q,"name":"Concurrent Moves","price":1,"quantity":10}`, sku), handler.Create)
	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("create status = %d; body = %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data dto.ProductResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	id := created.Data.ID
	defer func() {
		if _, err := db.Exec(`DELETE FROM products WHERE id = $1`, id); err != nil {
			t.Errorf("cleanup test product: %v", err)
		}
	}()

	const requests = 40
	type result struct {
		in     bool
		status int
	}
	results := make(chan result, requests)
	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			// Stock-outs outnumber and outweigh stock-ins, so some must be
			// rejected for insufficient stock.
			if i%3 == 0 {
				rec := post(fmt.Sprintf("/api/v1/products/%d/stock-in", id),
					fmt.Sprintf(`{"quantity":%d}`, 1+i%3), handler.StockIn)
				results <- result{in: true, status: rec.Code}
				return
			}
			rec := post(fmt.Sprintf("/api/v1/products/%d/stock-out", id),
				fmt.Sprintf(`{"quantity":%d}`, 2+i%4), handler.StockOut)
			results <- result{in: false, status: rec.Code}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	var inOK, outOK, outRejected int
	for r := range results {
		switch {
		case r.in && r.status == stdhttp.StatusOK:
			inOK++
		case !r.in && r.status == stdhttp.StatusOK:
			outOK++
		case !r.in && r.status == stdhttp.StatusConflict:
			outRejected++
		default:
			t.Errorf("unexpected result: stock-in=%v status=%d", r.in, r.status)
		}
	}
	if outRejected == 0 {
		t.Log("no stock-out was rejected; the run did not exercise insufficient stock")
	}

	var quantity int
	if err := db.QueryRow(`SELECT quantity FROM products WHERE id = $1`, id).Scan(&quantity); err != nil {
		t.Fatalf("read final quantity: %v", err)
	}
	if quantity < 0 {
		t.Fatalf("final quantity = %d, must never be negative", quantity)
	}

	movements := readMovements(t, db, sku)
	if want := 1 + inOK + outOK; len(movements) != want {
		t.Fatalf("movements = %d, want %d (1 initial + %d in + %d out); a movement is missing or duplicated",
			len(movements), want, inOK, outOK)
	}

	var ins, outs, sum int
	for i, m := range movements {
		if m.UserID != actorID {
			t.Errorf("movement %d actor = %d, want %d", i, m.UserID, actorID)
		}
		if m.After < 0 || m.Before < 0 {
			t.Errorf("movement %d has negative stock: %+v", i, m)
		}
		if m.After != m.Before+m.Change {
			t.Errorf("movement %d does not balance: %+v", i, m)
		}
		if i == 0 {
			if m.Type != "initial" || m.Before != 0 || m.After != 10 {
				t.Errorf("first movement = %+v, want initial 0 -> 10", m)
			}
		} else if m.Before != movements[i-1].After {
			t.Errorf("movement %d starts at %d but movement %d ended at %d: history has a gap",
				i, m.Before, i-1, movements[i-1].After)
		}
		switch m.Type {
		case "in":
			ins++
		case "out":
			outs++
		}
		sum += m.Change
	}

	if ins != inOK || outs != outOK {
		t.Errorf("recorded %d in / %d out, want %d / %d (successful requests)", ins, outs, inOK, outOK)
	}
	if sum != quantity {
		t.Errorf("SUM(quantity_change) = %d, stored quantity = %d", sum, quantity)
	}
	if last := movements[len(movements)-1].After; last != quantity {
		t.Errorf("last movement ends at %d, stored quantity = %d", last, quantity)
	}
	t.Logf("requests: %d stock-in ok, %d stock-out ok, %d stock-out rejected; final quantity %d",
		inOK, outOK, outRejected, quantity)
}
