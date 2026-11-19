package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"gostock/internal/domain"
)

func (iso *isolatedStockDB) insertProduct(t *testing.T, sku string, quantity, minimumStock int) int64 {
	t.Helper()
	var id int64
	if err := iso.db.QueryRow(
		`INSERT INTO products (sku, name, price, quantity, minimum_stock) VALUES ($1, 'dashboard', 1, $2, $3) RETURNING id`,
		sku, quantity, minimumStock,
	).Scan(&id); err != nil {
		t.Fatalf("insert product %s: %v", sku, err)
	}
	return id
}

// independentSummary classifies products with CASE and GROUP BY instead of
// the repository's FILTER clauses.
func (iso *isolatedStockDB) independentSummary(t *testing.T) domain.InventorySummary {
	t.Helper()
	rows, err := iso.db.Query(`
		SELECT CASE WHEN quantity = 0 THEN 'out'
		            WHEN quantity <= minimum_stock THEN 'low'
		            ELSE 'normal' END,
		       COUNT(*), SUM(quantity)
		FROM products GROUP BY 1`)
	if err != nil {
		t.Fatalf("independent summary: %v", err)
	}
	defer rows.Close()

	var s domain.InventorySummary
	for rows.Next() {
		var status string
		var count, stock int64
		if err := rows.Scan(&status, &count, &stock); err != nil {
			t.Fatalf("scan: %v", err)
		}
		s.TotalProducts += count
		s.TotalStock += stock
		switch status {
		case "normal":
			s.Normal = count
		case "low":
			s.LowStock = count
		case "out":
			s.OutOfStock = count
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return s
}

func TestDashboardRepository_InventorySummary(t *testing.T) {
	type product struct {
		quantity, minimumStock int
	}

	tests := []struct {
		name     string
		products []product
		want     domain.InventorySummary
	}{
		{"no products", nil, domain.InventorySummary{}},
		{"all normal", []product{{10, 0}, {6, 5}, {1, 0}},
			domain.InventorySummary{TotalProducts: 3, TotalStock: 17, Normal: 3}},
		{"every status and boundary", []product{
			{10, 0}, // normal, minimum_stock 0
			{1, 0},  // normal: smallest stock with no threshold
			{6, 5},  // normal: quantity = minimum_stock + 1
			{5, 5},  // low: quantity = minimum_stock
			{1, 3},  // low
			{0, 0},  // out: minimum_stock 0 is still out of stock, not normal
			{0, 4},  // out, not low
		}, domain.InventorySummary{TotalProducts: 7, TotalStock: 23, Normal: 3, LowStock: 2, OutOfStock: 2}},
		{"only out of stock", []product{{0, 0}, {0, 9}},
			domain.InventorySummary{TotalProducts: 2, TotalStock: 0, OutOfStock: 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iso := openIsolatedStockDB(t, false)
			repo := NewDashboardRepository(iso.db)
			for i, p := range tt.products {
				iso.insertProduct(t, fmt.Sprintf("DASH-%d", i), p.quantity, p.minimumStock)
			}

			got, err := repo.InventorySummary(context.Background())
			if err != nil {
				t.Fatalf("InventorySummary: %v", err)
			}
			iso.check(t)

			if got != tt.want {
				t.Errorf("InventorySummary = %+v, want %+v", got, tt.want)
			}
			if independent := iso.independentSummary(t); got != independent {
				t.Errorf("InventorySummary = %+v, independent SQL = %+v", got, independent)
			}
			if got.Normal+got.LowStock+got.OutOfStock != got.TotalProducts {
				t.Errorf("statuses %d + %d + %d do not add up to %d", got.Normal, got.LowStock, got.OutOfStock, got.TotalProducts)
			}
		})
	}
}

// independentBuckets sums with CASE and casts to a local date instead of the
// repository's FILTER and date_trunc, over the instants [start, end) that the
// caller writes by hand in UTC (the repository computes them in SQL).
func (iso *isolatedStockDB) independentBuckets(t *testing.T, granularity, tz string, start, end time.Time) []domain.StockMovementBucket {
	t.Helper()
	period := `((created_at AT TIME ZONE $1)::date)`
	if granularity == "month" {
		period = `(date_trunc('month', (created_at AT TIME ZONE $1)::date)::date)`
	}
	rows, err := iso.db.Query(`
		SELECT to_char(`+period+`, 'YYYY-MM-DD'),
		       SUM(CASE WHEN movement_type = 'in' THEN quantity_change ELSE 0 END),
		       SUM(CASE WHEN movement_type = 'out' THEN -quantity_change ELSE 0 END)
		FROM stock_movements
		WHERE movement_type <> 'initial' AND created_at >= $2 AND created_at < $3
		GROUP BY 1 ORDER BY 1`, tz, start, end)
	if err != nil {
		t.Fatalf("independent buckets: %v", err)
	}
	defer rows.Close()

	buckets := make([]domain.StockMovementBucket, 0)
	for rows.Next() {
		var b domain.StockMovementBucket
		if err := rows.Scan(&b.Period, &b.StockIn, &b.StockOut); err != nil {
			t.Fatalf("scan: %v", err)
		}
		buckets = append(buckets, b)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return buckets
}

func assertBuckets(t *testing.T, got, want []domain.StockMovementBucket) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("buckets = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("bucket %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDashboardRepository_StockMovementBuckets(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	repo := NewDashboardRepository(iso.db)
	actor := iso.createUser(t)

	kept := iso.insertProduct(t, "KEPT", 0, 0)
	gone := iso.insertProduct(t, "GONE", 0, 0)
	utc := func(s string) time.Time { return mustParseUTC(t, s) }

	// Times are chosen around midnight in Asia/Ho_Chi_Minh (UTC+7) and UTC.
	//                                                                local +07          UTC
	iso.insertMovementAt(t, kept, "KEPT", "in", 5, 0, 5, actor, utc("2026-10-31T16:59:59Z"))          // 10-31 23:59:59   10-31
	iso.insertMovementAt(t, kept, "KEPT", "in", 7, 5, 12, actor, utc("2026-10-31T17:00:00Z"))         // 11-01 00:00:00   10-31
	iso.insertMovementAt(t, kept, "KEPT", "out", -3, 12, 9, actor, utc("2026-11-01T16:59:59.999Z"))   // 11-01 23:59:59   11-01
	iso.insertMovementAt(t, kept, "KEPT", "initial", 100, 0, 100, actor, utc("2026-11-02T03:00:00Z")) // excluded: initial
	iso.insertMovementAt(t, gone, "GONE", "out", -2, 2, 0, actor, utc("2026-11-02T05:00:00Z"))        // 11-02 12:00      11-02
	iso.insertMovementAt(t, kept, "KEPT", "in", 6, 9, 15, actor, utc("2026-11-02T16:59:59Z"))         // 11-02 23:59:59   11-02
	iso.insertMovementAt(t, kept, "KEPT", "in", 4, 15, 19, actor, utc("2026-11-02T17:00:00Z"))        // 11-03 00:00:00   11-02

	// The deleted product's movement must still be counted (no join).
	if _, err := iso.db.Exec(`DELETE FROM products WHERE id = $1`, gone); err != nil {
		t.Fatalf("delete product: %v", err)
	}
	var orphaned int
	if err := iso.db.QueryRow(`SELECT COUNT(*) FROM stock_movements WHERE product_id IS NULL`).Scan(&orphaned); err != nil || orphaned != 1 {
		t.Fatalf("orphaned movements = %d (%v), want 1", orphaned, err)
	}

	var before struct {
		rows int
		sum  int64
	}
	readTable := func() (int, int64) {
		var n int
		var s sql.NullInt64
		if err := iso.db.QueryRow(`SELECT COUNT(*), SUM(quantity_change) FROM stock_movements`).Scan(&n, &s); err != nil {
			t.Fatalf("read table: %v", err)
		}
		return n, s.Int64
	}
	before.rows, before.sum = readTable()

	// start and end are the expected instants of [from 00:00, to + 1 day
	// 00:00) in tz, written by hand in UTC.
	tests := []bucketCase{
		{
			name: "daily in Asia/Ho_Chi_Minh, 1-2 Nov", granularity: "day", tz: "Asia/Ho_Chi_Minh",
			from: "2026-11-01", to: "2026-11-02", start: "2026-10-31T17:00:00Z", end: "2026-11-02T17:00:00Z",
			want: []domain.StockMovementBucket{
				{Period: "2026-11-01", StockIn: 7, StockOut: 3},
				{Period: "2026-11-02", StockIn: 6, StockOut: 2},
			},
		},
		{
			name: "daily in UTC, 1-2 Nov", granularity: "day", tz: "UTC",
			from: "2026-11-01", to: "2026-11-02", start: "2026-11-01T00:00:00Z", end: "2026-11-03T00:00:00Z",
			want: []domain.StockMovementBucket{
				{Period: "2026-11-01", StockIn: 0, StockOut: 3},
				{Period: "2026-11-02", StockIn: 10, StockOut: 2},
			},
		},
		{
			name: "monthly in Asia/Ho_Chi_Minh, Oct-Nov", granularity: "month", tz: "Asia/Ho_Chi_Minh",
			from: "2026-10-01", to: "2026-11-30", start: "2026-09-30T17:00:00Z", end: "2026-11-30T17:00:00Z",
			want: []domain.StockMovementBucket{
				{Period: "2026-10-01", StockIn: 5, StockOut: 0},
				{Period: "2026-11-01", StockIn: 17, StockOut: 5},
			},
		},
		{
			name: "range without movements", granularity: "day", tz: "UTC",
			from: "2026-12-01", to: "2026-12-07", start: "2026-12-01T00:00:00Z", end: "2026-12-08T00:00:00Z",
			want: []domain.StockMovementBucket{},
		},
	}
	runBucketCases(t, iso, repo, tests)

	if rows, sum := readTable(); rows != before.rows || sum != before.sum {
		t.Errorf("stock_movements changed by reading: %d rows sum %d, was %d rows sum %d", rows, sum, before.rows, before.sum)
	}
}

func mustParseUTC(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return v
}

func mustParseDate(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return v
}

// bucketCase is one StockMovementBuckets call. from and to are the calendar
// dates passed to the repository; start and end are the instants the range
// must cover, written by hand in UTC as an oracle independent of the code.
type bucketCase struct {
	name        string
	granularity string
	tz          string
	from, to    string
	start, end  string
	want        []domain.StockMovementBucket
}

func runBucketCases(t *testing.T, iso *isolatedStockDB, repo *DashboardRepository, tests []bucketCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.StockMovementBuckets(context.Background(), tt.granularity, tt.tz,
				mustParseDate(t, tt.from), mustParseDate(t, tt.to))
			if err != nil {
				t.Fatalf("StockMovementBuckets: %v", err)
			}
			iso.check(t)

			assertBuckets(t, got, tt.want)
			independent := iso.independentBuckets(t, tt.granularity, tt.tz, mustParseUTC(t, tt.start), mustParseUTC(t, tt.end))
			assertBuckets(t, got, independent)
			for _, b := range got {
				if b.StockOut < 0 || b.StockIn < 0 {
					t.Errorf("bucket %+v has a negative total", b)
				}
			}
		})
	}
}

func TestDashboardRepository_StockMovementBuckets_DaylightSaving(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	repo := NewDashboardRepository(iso.db)
	actor := iso.createUser(t)
	p := iso.insertProduct(t, "DST", 0, 0)

	move := func(movementType string, change int, at string) {
		t.Helper()
		before := 1000
		iso.insertMovementAt(t, p, "DST", movementType, change, before, before+change, actor, mustParseUTC(t, at))
	}

	//                                       local time
	move("in", 1, "2026-09-05T03:59:59.999999Z")    // Santiago 4 Sep 23:59:59.999999 (-04)
	move("in", 2, "2026-09-05T04:00:00Z")           // Santiago 5 Sep 00:00 (-04)
	move("in", 4, "2026-09-06T03:30:00Z")           // Santiago 5 Sep 23:30 (-04), last hour of 5 Sep
	move("out", -8, "2026-09-06T03:59:59.999999Z")  // Santiago 5 Sep 23:59:59.999999 (-04)
	move("in", 16, "2026-09-06T04:00:00Z")          // Santiago 6 Sep 01:00 (-03), first instant of 6 Sep
	move("out", -32, "2026-09-07T02:59:59.999999Z") // Santiago 6 Sep 23:59:59.999999 (-03)
	move("in", 64, "2026-09-07T03:00:00Z")          // Santiago 7 Sep 00:00 (-03)

	move("in", 100, "2026-03-08T04:59:59.999999Z")   // New York 7 Mar 23:59:59.999999 (EST)
	move("in", 200, "2026-03-08T05:00:00Z")          // New York 8 Mar 00:00 (EST)
	move("out", -400, "2026-03-09T03:59:59.999999Z") // New York 8 Mar 23:59:59.999999 (EDT)
	move("in", 800, "2026-03-09T04:00:00Z")          // New York 9 Mar 00:00 (EDT)

	move("in", 3, "2026-11-01T06:59:59.999999Z") // Etc/GMT+7 31 Oct 23:59:59.999999
	move("out", -5, "2026-11-01T07:00:00Z")      // Etc/GMT+7 1 Nov 00:00

	runBucketCases(t, iso, repo, []bucketCase{
		{
			// Before the fix the range started at 03:00 UTC and took in the
			// last hour of 5 Sep, which fell outside the requested days.
			name: "Santiago, from on the DST start day", granularity: "day", tz: "America/Santiago",
			from: "2026-09-06", to: "2026-09-06", start: "2026-09-06T04:00:00Z", end: "2026-09-07T03:00:00Z",
			want: []domain.StockMovementBucket{{Period: "2026-09-06", StockIn: 16, StockOut: 32}},
		},
		{
			// Before the fix the range ended at 03:00 UTC and lost 23:00-24:00.
			name: "Santiago, to on the day before DST starts", granularity: "day", tz: "America/Santiago",
			from: "2026-09-05", to: "2026-09-05", start: "2026-09-05T04:00:00Z", end: "2026-09-06T04:00:00Z",
			want: []domain.StockMovementBucket{{Period: "2026-09-05", StockIn: 6, StockOut: 8}},
		},
		{
			name: "Santiago, across the DST start by day", granularity: "day", tz: "America/Santiago",
			from: "2026-09-05", to: "2026-09-07", start: "2026-09-05T04:00:00Z", end: "2026-09-08T03:00:00Z",
			want: []domain.StockMovementBucket{
				{Period: "2026-09-05", StockIn: 6, StockOut: 8},
				{Period: "2026-09-06", StockIn: 16, StockOut: 32},
				{Period: "2026-09-07", StockIn: 64, StockOut: 0},
			},
		},
		{
			name: "Santiago, by month from the DST start day", granularity: "month", tz: "America/Santiago",
			from: "2026-09-06", to: "2026-09-30", start: "2026-09-06T04:00:00Z", end: "2026-10-01T03:00:00Z",
			want: []domain.StockMovementBucket{{Period: "2026-09-01", StockIn: 80, StockOut: 32}},
		},
		{
			name: "Santiago, by month to the day before DST starts", granularity: "month", tz: "America/Santiago",
			from: "2026-08-01", to: "2026-09-05", start: "2026-08-01T04:00:00Z", end: "2026-09-06T04:00:00Z",
			want: []domain.StockMovementBucket{{Period: "2026-09-01", StockIn: 7, StockOut: 8}},
		},
		{
			name: "New York, the 23-hour DST start day", granularity: "day", tz: "America/New_York",
			from: "2026-03-08", to: "2026-03-08", start: "2026-03-08T05:00:00Z", end: "2026-03-09T04:00:00Z",
			want: []domain.StockMovementBucket{{Period: "2026-03-08", StockIn: 200, StockOut: 400}},
		},
		{
			name: "New York, by month", granularity: "month", tz: "America/New_York",
			from: "2026-03-01", to: "2026-03-31", start: "2026-03-01T05:00:00Z", end: "2026-04-01T04:00:00Z",
			want: []domain.StockMovementBucket{{Period: "2026-03-01", StockIn: 1100, StockOut: 400}},
		},
		{
			name: "Etc/GMT+7 is seven hours behind UTC", granularity: "day", tz: "Etc/GMT+7",
			from: "2026-11-01", to: "2026-11-01", start: "2026-11-01T07:00:00Z", end: "2026-11-02T07:00:00Z",
			want: []domain.StockMovementBucket{{Period: "2026-11-01", StockIn: 0, StockOut: 5}},
		},
	})
}

func TestDashboardRepository_StockMovementBuckets_RepeatedMidnight(t *testing.T) {
	iso := openIsolatedStockDB(t, false)
	repo := NewDashboardRepository(iso.db)
	actor := iso.createUser(t)
	p := iso.insertProduct(t, "HAV", 0, 0)

	move := func(movementType string, change int, at string) {
		t.Helper()
		before := 1000
		iso.insertMovementAt(t, p, "HAV", movementType, change, before, before+change, actor, mustParseUTC(t, at))
	}

	//                                                Havana local time
	move("in", 256, "2025-11-01T03:59:59.999999Z") // 31 Oct 23:59:59.999999 CDT
	move("in", 1, "2025-11-02T03:59:59.999999Z")   //  1 Nov 23:59:59.999999 CDT
	move("in", 2, "2025-11-02T04:00:00Z")          //  2 Nov 00:00 CDT, first occurrence, first instant of 2 Nov
	move("in", 4, "2025-11-02T04:30:00Z")          //  2 Nov 00:30 CDT, first occurrence
	move("out", -8, "2025-11-02T04:59:59.999999Z") //  2 Nov 00:59:59.999999 CDT, first occurrence
	move("in", 16, "2025-11-02T05:00:00Z")         //  2 Nov 00:00 CST, second occurrence
	move("out", -32, "2025-11-02T05:30:00Z")       //  2 Nov 00:30 CST, second occurrence
	move("in", 64, "2025-11-03T04:59:59.999999Z")  //  2 Nov 23:59:59.999999 CST
	move("in", 128, "2025-11-03T05:00:00Z")        //  3 Nov 00:00 CST

	move("in", 512, "2025-03-09T04:59:59.999999Z") //  8 Mar 23:59:59.999999 CST
	move("in", 5, "2025-03-09T05:00:00Z")          //  9 Mar 01:00 CDT, first instant of 9 Mar
	move("out", -6, "2025-03-10T03:59:59.999999Z") //  9 Mar 23:59:59.999999 CDT
	move("in", 7, "2025-03-10T04:00:00Z")          // 10 Mar 00:00 CDT

	runBucketCases(t, iso, repo, []bucketCase{
		{
			name: "Havana, from and to on the repeated-midnight day", granularity: "day", tz: "America/Havana",
			from: "2025-11-02", to: "2025-11-02", start: "2025-11-02T04:00:00Z", end: "2025-11-03T05:00:00Z",
			want: []domain.StockMovementBucket{{Period: "2025-11-02", StockIn: 2 + 4 + 16 + 64, StockOut: 8 + 32}},
		},
		{
			name: "Havana, to on the day before the repeated midnight", granularity: "day", tz: "America/Havana",
			from: "2025-11-01", to: "2025-11-01", start: "2025-11-01T04:00:00Z", end: "2025-11-02T04:00:00Z",
			want: []domain.StockMovementBucket{{Period: "2025-11-01", StockIn: 1, StockOut: 0}},
		},
		{
			name: "Havana, across the repeated midnight by day", granularity: "day", tz: "America/Havana",
			from: "2025-11-01", to: "2025-11-03", start: "2025-11-01T04:00:00Z", end: "2025-11-04T05:00:00Z",
			want: []domain.StockMovementBucket{
				{Period: "2025-11-01", StockIn: 1, StockOut: 0},
				{Period: "2025-11-02", StockIn: 86, StockOut: 40},
				{Period: "2025-11-03", StockIn: 128, StockOut: 0},
			},
		},
		{
			name: "Havana, by month from the repeated-midnight day", granularity: "month", tz: "America/Havana",
			from: "2025-11-02", to: "2025-11-30", start: "2025-11-02T04:00:00Z", end: "2025-12-01T05:00:00Z",
			want: []domain.StockMovementBucket{{Period: "2025-11-01", StockIn: 86 + 128, StockOut: 40}},
		},
		{
			name: "Havana, by month to the day before the repeated midnight", granularity: "month", tz: "America/Havana",
			from: "2025-10-01", to: "2025-11-01", start: "2025-10-01T04:00:00Z", end: "2025-11-02T04:00:00Z",
			want: []domain.StockMovementBucket{
				{Period: "2025-10-01", StockIn: 256, StockOut: 0},
				{Period: "2025-11-01", StockIn: 1, StockOut: 0},
			},
		},
		{
			name: "Havana, the skipped-midnight day", granularity: "day", tz: "America/Havana",
			from: "2025-03-09", to: "2025-03-09", start: "2025-03-09T05:00:00Z", end: "2025-03-10T04:00:00Z",
			want: []domain.StockMovementBucket{{Period: "2025-03-09", StockIn: 5, StockOut: 6}},
		},
		{
			name: "Havana, across the skipped midnight by day", granularity: "day", tz: "America/Havana",
			from: "2025-03-08", to: "2025-03-10", start: "2025-03-08T05:00:00Z", end: "2025-03-11T04:00:00Z",
			want: []domain.StockMovementBucket{
				{Period: "2025-03-08", StockIn: 512, StockOut: 0},
				{Period: "2025-03-09", StockIn: 5, StockOut: 6},
				{Period: "2025-03-10", StockIn: 7, StockOut: 0},
			},
		},
	})
}
