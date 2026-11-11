package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	productHTTP "gostock/internal/handle/http"
	"gostock/internal/repository/postgres"
	"gostock/internal/testdb"
	"gostock/internal/usecase"
)

// openIsolatedDashboardDB returns a one-connection pool whose session shadows
// users, products and stock_movements with temporary tables (created before
// any other statement on the connection), so dashboard totals are exact even
// while other tests write to the shared test database. The returned check
// fails the test if the pool has switched connections or the tables no
// longer shadow the public ones.
func openIsolatedDashboardDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	db := testdb.Open(t)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	for _, stmt := range []string{
		`CREATE TEMP TABLE users (LIKE public.users INCLUDING ALL)`,
		`CREATE TEMP TABLE products (LIKE public.products INCLUDING ALL)`,
		`CREATE TEMP TABLE stock_movements (LIKE public.stock_movements INCLUDING ALL)`,
		`ALTER TABLE pg_temp.stock_movements ADD CONSTRAINT stock_movements_product_id_fkey
			FOREIGN KEY (product_id) REFERENCES pg_temp.products(id) ON DELETE SET NULL`,
		`ALTER TABLE pg_temp.stock_movements ADD CONSTRAINT stock_movements_user_id_fkey
			FOREIGN KEY (user_id) REFERENCES pg_temp.users(id) ON DELETE RESTRICT`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create temporary table: %v\n%s", err, stmt)
		}
	}

	var pid int
	if err := db.QueryRow(`SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("read backend pid: %v", err)
	}

	check := func() {
		t.Helper()
		var current int
		var shadowed bool
		if err := db.QueryRow(`
			SELECT pg_backend_pid(),
			       to_regclass('users') = to_regclass('pg_temp.users')
			   AND to_regclass('products') = to_regclass('pg_temp.products')
			   AND to_regclass('stock_movements') = to_regclass('pg_temp.stock_movements')`,
		).Scan(&current, &shadowed); err != nil {
			t.Fatalf("isolation check: %v", err)
		}
		if current != pid || !shadowed {
			t.Fatalf("isolation lost: backend pid %d -> %d, shadowed=%v", pid, current, shadowed)
		}
	}
	check()
	return db, check
}

// TestDashboard_EndToEnd builds an inventory through the API, places every
// stock movement at a fixed instant, and checks both dashboard endpoints
// against hand-computed figures and independent SQL aggregates. Nothing
// depends on the current time: every chart request names its dates (or only
// to), and instants are written in UTC.
func TestDashboard_EndToEnd(t *testing.T) {
	db, check := openIsolatedDashboardDB(t)

	utc := func(s string) time.Time {
		t.Helper()
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatalf("parse %s: %v", s, err)
		}
		return v
	}

	createUser := func(role string) *domain.User {
		var id int64
		email := role + "@dashboard.example.test"
		if err := db.QueryRow(`INSERT INTO users (email, password_hash, role) VALUES ($1, 'x', $2) RETURNING id`,
			email, role).Scan(&id); err != nil {
			t.Fatalf("create %s: %v", role, err)
		}
		return &domain.User{ID: id, Email: email, Role: role}
	}
	admin := createUser(usecase.RoleAdmin)
	user := createUser(usecase.RoleUser)

	productRepo := postgres.NewProductRepository(db)
	userRepo := postgres.NewUserRepository(db)
	authUC := usecase.NewAuthUseCase(userRepo, postgres.NewRefreshTokenRepository(db), &fakeTokenManager{},
		15*time.Minute, 7*24*time.Hour, 15*time.Second)
	router := NewRouter(
		db,
		productHTTP.NewProductHandler(usecase.NewProductUseCase(productRepo)),
		productHTTP.NewAuthHandler(authUC, nil),
		tokenAuthenticator{"admin": admin, "user": user},
		productHTTP.NewUserHandler(usecase.NewUserUseCase(userRepo)),
		productHTTP.NewStockMovementHandler(usecase.NewStockMovementUseCase(productRepo, postgres.NewStockMovementRepository(db))),
		productHTTP.NewDashboardHandler(usecase.NewDashboardUseCase(postgres.NewDashboardRepository(db))),
	)

	call := func(method, path, token, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		check()
		if rec.Code != want {
			t.Fatalf("%s %s: status = %d, want %d; body = %s", method, path, rec.Code, want, rec.Body.String())
		}
		return rec
	}
	createProduct := func(sku string, quantity, minimumStock int) int64 {
		t.Helper()
		rec := call(http.MethodPost, "/api/v1/products", "admin",
			fmt.Sprintf(`{"sku":%q,"name":"Dashboard","price":1,"quantity":%d,"minimum_stock":%d}`, sku, quantity, minimumStock),
			http.StatusCreated)
		var created struct {
			Data dto.ProductResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("decode create: %v", err)
		}
		return created.Data.ID
	}

	// Products in every status, built through the API (each with stock gets an
	// 'initial' movement, which the chart must ignore).
	a := createProduct("A", 10, 2) // + stock-in 5  -> 15, normal
	createProduct("B", 3, 5)       //                  3, low
	createProduct("C", 0, 0)       //                  0, out
	createProduct("D", 4, 4)       //                  4, low (quantity = minimum)
	e := createProduct("E", 8, 0)  // - stock-out 8  -> 0, out
	f := createProduct("F", 6, 1)  // - stock-out 2, then deleted

	call(http.MethodPost, fmt.Sprintf("/api/v1/products/%d/stock-in", a), "user", `{"quantity":5}`, http.StatusOK)
	call(http.MethodPost, fmt.Sprintf("/api/v1/products/%d/stock-out", e), "user", `{"quantity":8}`, http.StatusOK)
	call(http.MethodPost, fmt.Sprintf("/api/v1/products/%d/stock-out", f), "user", `{"quantity":2}`, http.StatusOK)
	call(http.MethodDelete, fmt.Sprintf("/api/v1/products/%d", f), "admin", "", http.StatusNoContent)

	// Pin the movements just made through the API to 4 Nov 2026 10:00 in Ho
	// Chi Minh City (UTC+7), so the chart does not depend on today's date.
	check()
	if _, err := db.Exec(`UPDATE pg_temp.stock_movements SET created_at = $1 WHERE movement_type <> 'initial'`,
		utc("2026-11-04T03:00:00Z")); err != nil {
		t.Fatalf("pin API movements: %v", err)
	}

	// Movements on earlier local days, around the range boundaries.
	insertAt := func(movementType string, change, before, after int, at time.Time) {
		t.Helper()
		if _, err := db.Exec(`
			INSERT INTO stock_movements (product_id, product_sku, movement_type, quantity_change,
			                             quantity_before, quantity_after, user_id, created_at)
			VALUES ($1, 'A', $2, $3, $4, $5, $6, $7)`,
			a, movementType, change, before, after, user.ID, at); err != nil {
			t.Fatalf("insert movement: %v", err)
		}
	}
	//                                                         Ho Chi Minh City time
	insertAt("in", 7, 0, 7, utc("2026-11-03T05:00:00Z"))        // 3 Nov 12:00
	insertAt("out", -3, 7, 4, utc("2026-10-31T17:00:00Z"))      // 1 Nov 00:00:00, first instant of the daily range
	insertAt("in", 9, 4, 13, utc("2026-10-31T16:59:59Z"))       // 31 Oct 23:59:59, just before the daily range
	insertAt("initial", 50, 0, 50, utc("2026-11-01T18:00:00Z")) // 2 Nov 01:00, initial: never charted

	readTable := func() (int, int64, int64) {
		var movements int
		var products, stock int64
		if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM stock_movements), (SELECT COUNT(*) FROM products),
		                              (SELECT COALESCE(SUM(quantity), 0) FROM products)`).Scan(&movements, &products, &stock); err != nil {
			t.Fatalf("read tables: %v", err)
		}
		return movements, products, stock
	}
	m0, p0, s0 := readTable()

	// Summary cards and status breakdown.
	wantSummary := dto.DashboardSummaryResponse{
		TotalProducts: 5, TotalStock: 22, LowStockProducts: 2, OutOfStockProducts: 2,
		StatusBreakdown: []dto.InventoryStatusCount{
			{Status: "normal", Count: 1}, {Status: "low_stock", Count: 2}, {Status: "out_of_stock", Count: 2},
		},
	}
	var independent dto.DashboardSummaryResponse
	var normal int64
	if err := db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(quantity), 0),
		       SUM(CASE WHEN quantity = 0 THEN 0 WHEN quantity <= minimum_stock THEN 1 ELSE 0 END),
		       SUM(CASE WHEN quantity = 0 THEN 1 ELSE 0 END),
		       SUM(CASE WHEN quantity > minimum_stock THEN 1 ELSE 0 END)
		FROM products`).Scan(&independent.TotalProducts, &independent.TotalStock,
		&independent.LowStockProducts, &independent.OutOfStockProducts, &normal); err != nil {
		t.Fatalf("independent summary: %v", err)
	}

	for _, token := range []string{"user", "admin"} {
		var got struct {
			Success bool                         `json:"success"`
			Data    dto.DashboardSummaryResponse `json:"data"`
		}
		rec := call(http.MethodGet, "/api/v1/dashboard/summary", token, "", http.StatusOK)
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode summary: %v", err)
		}
		if !got.Success || fmt.Sprint(got.Data) != fmt.Sprint(wantSummary) {
			t.Errorf("%s summary = %+v, want %+v", token, got.Data, wantSummary)
		}
		if got.Data.TotalProducts != independent.TotalProducts || got.Data.TotalStock != independent.TotalStock ||
			got.Data.LowStockProducts != independent.LowStockProducts || got.Data.OutOfStockProducts != independent.OutOfStockProducts ||
			got.Data.StatusBreakdown[0].Count != normal {
			t.Errorf("%s summary %+v disagrees with independent SQL %+v (normal %d)", token, got.Data, independent, normal)
		}
	}

	// Stock movement chart over four local days, 1-4 Nov 2026 in Ho Chi Minh
	// City = [2026-10-31T17:00:00Z, 2026-11-04T17:00:00Z).
	path := "/api/v1/dashboard/stock-movements?from=2026-11-01&to=2026-11-04&tz=Asia/Ho_Chi_Minh"
	wantPoints := []dto.StockMovementChartPoint{
		{Period: "2026-11-01", StockIn: 0, StockOut: 3},
		{Period: "2026-11-02", StockIn: 0, StockOut: 0},
		{Period: "2026-11-03", StockIn: 7, StockOut: 0},
		{Period: "2026-11-04", StockIn: 5, StockOut: 10}, // stock-in A; stock-out E and deleted F
	}

	independentPoints := map[string]dto.StockMovementChartPoint{}
	rows, err := db.Query(`
		SELECT to_char((created_at AT TIME ZONE 'Asia/Ho_Chi_Minh')::date, 'YYYY-MM-DD'),
		       SUM(CASE WHEN movement_type = 'in' THEN quantity_change ELSE 0 END),
		       SUM(CASE WHEN movement_type = 'out' THEN -quantity_change ELSE 0 END)
		FROM stock_movements
		WHERE movement_type <> 'initial' AND created_at >= $1 AND created_at < $2
		GROUP BY 1`, utc("2026-10-31T17:00:00Z"), utc("2026-11-04T17:00:00Z"))
	if err != nil {
		t.Fatalf("independent chart: %v", err)
	}
	for rows.Next() {
		var p dto.StockMovementChartPoint
		if err := rows.Scan(&p.Period, &p.StockIn, &p.StockOut); err != nil {
			t.Fatalf("scan: %v", err)
		}
		independentPoints[p.Period] = p
	}
	rows.Close()
	check()

	getChart := func(path, token string) dto.StockMovementChartResponse {
		t.Helper()
		var got struct {
			Data dto.StockMovementChartResponse `json:"data"`
		}
		rec := call(http.MethodGet, path, token, "", http.StatusOK)
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode chart: %v", err)
		}
		return got.Data
	}

	for _, token := range []string{"user", "admin"} {
		c := getChart(path, token)
		if c.From != "2026-11-01" || c.To != "2026-11-04" || c.Granularity != "day" || c.Timezone != "Asia/Ho_Chi_Minh" {
			t.Errorf("%s chart header = %s..%s %s %s", token, c.From, c.To, c.Granularity, c.Timezone)
		}
		if fmt.Sprint(c.Points) != fmt.Sprint(wantPoints) {
			t.Errorf("%s chart points = %+v, want %+v", token, c.Points, wantPoints)
		}
		for _, p := range c.Points {
			ind := independentPoints[p.Period] // missing means a zero bucket
			ind.Period = p.Period
			if p != ind {
				t.Errorf("%s chart point %+v disagrees with independent SQL %+v", token, p, ind)
			}
		}
		if c.Totals.StockIn != 12 || c.Totals.StockOut != 13 {
			t.Errorf("%s chart totals = %+v, want in 12 / out 13", token, c.Totals)
		}
	}

	// Monthly charts. The 31 Oct 23:59:59 stock-in (9) now falls inside the
	// range, in October; everything else is in November.
	monthly := []struct {
		name       string
		path       string
		wantFrom   string
		wantPoints []dto.StockMovementChartPoint
	}{
		{
			name:     "explicit month",
			path:     "/api/v1/dashboard/stock-movements?from=2026-10-01&to=2026-11-30&granularity=month&tz=Asia/Ho_Chi_Minh",
			wantFrom: "2026-10-01",
			wantPoints: []dto.StockMovementChartPoint{
				{Period: "2026-10-01", StockIn: 9, StockOut: 0},
				{Period: "2026-11-01", StockIn: 12, StockOut: 13},
			},
		},
		{
			name:     "auto becomes monthly over 62 days",
			path:     "/api/v1/dashboard/stock-movements?from=2026-09-01&to=2026-11-30&tz=Asia/Ho_Chi_Minh",
			wantFrom: "2026-09-01",
			wantPoints: []dto.StockMovementChartPoint{
				{Period: "2026-09-01", StockIn: 0, StockOut: 0},
				{Period: "2026-10-01", StockIn: 9, StockOut: 0},
				{Period: "2026-11-01", StockIn: 12, StockOut: 13},
			},
		},
	}
	for _, m := range monthly {
		c := getChart(m.path, "user")
		if c.From != m.wantFrom || c.To != "2026-11-30" || c.Granularity != "month" || c.Timezone != "Asia/Ho_Chi_Minh" {
			t.Errorf("%s: header = %s..%s %s %s", m.name, c.From, c.To, c.Granularity, c.Timezone)
		}
		if fmt.Sprint(c.Points) != fmt.Sprint(m.wantPoints) {
			t.Errorf("%s: points = %+v, want %+v", m.name, c.Points, m.wantPoints)
		}
		if c.Totals.StockIn != 21 || c.Totals.StockOut != 13 {
			t.Errorf("%s: totals = %+v, want in 21 / out 13", m.name, c.Totals)
		}
	}

	// Only to: the 30 days ending on 4 Nov, i.e. from 6 Oct.
	c := getChart("/api/v1/dashboard/stock-movements?to=2026-11-04&tz=Asia/Ho_Chi_Minh", "user")
	if c.From != "2026-10-06" || c.To != "2026-11-04" || c.Granularity != "day" || len(c.Points) != 30 {
		t.Errorf("only to: %s..%s %s with %d points, want 2026-10-06..2026-11-04 day with 30", c.From, c.To, c.Granularity, len(c.Points))
	} else {
		byPeriod := map[string]dto.StockMovementChartPoint{}
		for _, p := range c.Points {
			byPeriod[p.Period] = p
		}
		if p := byPeriod["2026-10-31"]; p.StockIn != 9 {
			t.Errorf("only to: 31 Oct = %+v, want stock-in 9", p)
		}
		if p := byPeriod["2026-11-04"]; p.StockIn != 5 || p.StockOut != 10 {
			t.Errorf("only to: 4 Nov = %+v, want in 5 / out 10", p)
		}
	}
	if c.Totals.StockIn != 21 || c.Totals.StockOut != 13 {
		t.Errorf("only to: totals = %+v, want in 21 / out 13", c.Totals)
	}

	// Invalid parameters and authentication through the real router.
	call(http.MethodGet, "/api/v1/dashboard/stock-movements?from=2026-11-04&to=2026-11-03", "user", "", http.StatusBadRequest)
	call(http.MethodGet, "/api/v1/dashboard/stock-movements?from=2026-11-01&to=2026-11-04&tz=CET", "user", "", http.StatusBadRequest)
	call(http.MethodGet, "/api/v1/dashboard/summary", "", "", http.StatusUnauthorized)

	// Reading the dashboard changed nothing.
	if m, p, s := readTable(); m != m0 || p != p0 || s != s0 {
		t.Errorf("tables changed by dashboard reads: movements %d->%d, products %d->%d, stock %d->%d", m0, m, p0, p, s0, s)
	}
}
