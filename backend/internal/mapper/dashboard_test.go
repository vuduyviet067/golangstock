package mapper_test

import (
	"reflect"
	"testing"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/mapper"
)

func TestToDashboardSummaryResponse(t *testing.T) {
	summary := domain.InventorySummary{TotalProducts: 10, TotalStock: 250, Normal: 5, LowStock: 3, OutOfStock: 2}

	got := mapper.ToDashboardSummaryResponse(summary)

	want := dto.DashboardSummaryResponse{
		TotalProducts:      10,
		TotalStock:         250,
		LowStockProducts:   3,
		OutOfStockProducts: 2,
		StatusBreakdown: []dto.InventoryStatusCount{
			{Status: "normal", Count: 5},
			{Status: "low_stock", Count: 3},
			{Status: "out_of_stock", Count: 2},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToDashboardSummaryResponse = %+v, want %+v", got, want)
	}
	assertJSON(t, got, `{"total_products":10,"total_stock":250,"low_stock_products":3,"out_of_stock_products":2,"status_breakdown":[{"status":"normal","count":5},{"status":"low_stock","count":3},{"status":"out_of_stock","count":2}]}`)
}

// The breakdown always has the three statuses in the same order, including
// zero counts; it is never null or empty.
func TestToDashboardSummaryResponse_Empty(t *testing.T) {
	assertJSON(t, mapper.ToDashboardSummaryResponse(domain.InventorySummary{}),
		`{"total_products":0,"total_stock":0,"low_stock_products":0,"out_of_stock_products":0,"status_breakdown":[{"status":"normal","count":0},{"status":"low_stock","count":0},{"status":"out_of_stock","count":0}]}`)
}

// Each count goes to its own status: distinct values catch a swap.
func TestToDashboardSummaryResponse_StatusMapping(t *testing.T) {
	got := mapper.ToDashboardSummaryResponse(domain.InventorySummary{Normal: 101, LowStock: 202, OutOfStock: 303})

	if n := len(got.StatusBreakdown); n != 3 {
		t.Fatalf("len(StatusBreakdown) = %d, want 3", n)
	}
	wantStatus := []string{"normal", "low_stock", "out_of_stock"}
	wantCount := []int64{101, 202, 303}
	for i, entry := range got.StatusBreakdown {
		if entry.Status != wantStatus[i] || entry.Count != wantCount[i] {
			t.Errorf("StatusBreakdown[%d] = %+v, want {%s %d}", i, entry, wantStatus[i], wantCount[i])
		}
	}
	if got.LowStockProducts != 202 || got.OutOfStockProducts != 303 {
		t.Errorf("LowStockProducts/OutOfStockProducts = %d/%d, want 202/303", got.LowStockProducts, got.OutOfStockProducts)
	}
}

// The status values are the domain constants and keep their wire values.
func TestInventoryStatusValues(t *testing.T) {
	if domain.InventoryStatusNormal != "normal" ||
		domain.InventoryStatusLowStock != "low_stock" ||
		domain.InventoryStatusOutOfStock != "out_of_stock" {
		t.Fatalf("inventory status constants changed: %q %q %q",
			domain.InventoryStatusNormal, domain.InventoryStatusLowStock, domain.InventoryStatusOutOfStock)
	}
}

func TestToStockMovementChartResponse(t *testing.T) {
	chart := domain.StockMovementChart{
		From: "2026-03-01", To: "2026-03-03", Granularity: "day", Timezone: "Asia/Ho_Chi_Minh",
		Points: []domain.StockMovementBucket{
			{Period: "2026-03-01", StockIn: 10, StockOut: 0},
			{Period: "2026-03-02", StockIn: 0, StockOut: 0},
			{Period: "2026-03-03", StockIn: 5, StockOut: 7},
		},
		TotalStockIn:  15,
		TotalStockOut: 7,
	}

	got := mapper.ToStockMovementChartResponse(chart)

	want := dto.StockMovementChartResponse{
		From: "2026-03-01", To: "2026-03-03", Granularity: "day", Timezone: "Asia/Ho_Chi_Minh",
		Points: []dto.StockMovementChartPoint{
			{Period: "2026-03-01", StockIn: 10, StockOut: 0},
			{Period: "2026-03-02", StockIn: 0, StockOut: 0},
			{Period: "2026-03-03", StockIn: 5, StockOut: 7},
		},
		Totals: dto.StockMovementChartTotals{StockIn: 15, StockOut: 7},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToStockMovementChartResponse = %+v, want %+v", got, want)
	}
	assertJSON(t, got, `{"from":"2026-03-01","to":"2026-03-03","granularity":"day","timezone":"Asia/Ho_Chi_Minh","points":[{"period":"2026-03-01","stock_in":10,"stock_out":0},{"period":"2026-03-02","stock_in":0,"stock_out":0},{"period":"2026-03-03","stock_in":5,"stock_out":7}],"totals":{"stock_in":15,"stock_out":7}}`)
}

// Totals are copied, not recomputed from the points, and points are never
// null.
func TestToStockMovementChartResponse_NoPoints(t *testing.T) {
	for name, points := range map[string][]domain.StockMovementBucket{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			got := mapper.ToStockMovementChartResponse(domain.StockMovementChart{
				From: "2025-01-01", To: "2025-12-31", Granularity: "month", Timezone: "UTC",
				Points: points, TotalStockIn: 3, TotalStockOut: 4,
			})
			if got.Points == nil {
				t.Fatal("Points is nil, want a non-nil empty slice")
			}
			assertJSON(t, got, `{"from":"2025-01-01","to":"2025-12-31","granularity":"month","timezone":"UTC","points":[],"totals":{"stock_in":3,"stock_out":4}}`)
		})
	}
}
