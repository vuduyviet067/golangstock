package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/usecase"
)

type fakeDashboardRepository struct {
	summary    domain.InventorySummary
	summaryErr error
	buckets    []domain.StockMovementBucket
	bucketsErr error
	calls      int
}

func (f *fakeDashboardRepository) InventorySummary(ctx context.Context) (domain.InventorySummary, error) {
	f.calls++
	return f.summary, f.summaryErr
}

func (f *fakeDashboardRepository) StockMovementBuckets(
	ctx context.Context,
	granularity, timezone string,
	from, to time.Time,
) ([]domain.StockMovementBucket, error) {
	f.calls++
	return f.buckets, f.bucketsErr
}

func newTestDashboardHandler(repo *fakeDashboardRepository) *DashboardHandler {
	return NewDashboardHandler(usecase.NewDashboardUseCase(repo))
}

// TestDashboardEndpoints_JSONContract pins the exact response bodies of both
// dashboard endpoints.
func TestDashboardEndpoints_JSONContract(t *testing.T) {
	tests := []struct {
		name     string
		repo     *fakeDashboardRepository
		path     string
		serve    func(h *DashboardHandler) http.HandlerFunc
		wantBody string
	}{
		{
			name: "summary",
			repo: &fakeDashboardRepository{summary: domain.InventorySummary{
				TotalProducts: 50, TotalStock: 12840, Normal: 40, LowStock: 6, OutOfStock: 4,
			}},
			path:  "/api/v1/dashboard/summary",
			serve: func(h *DashboardHandler) http.HandlerFunc { return h.Summary },
			wantBody: `{"success":true,"message":"Dashboard summary retrieved successfully","data":{` +
				`"total_products":50,"total_stock":12840,"low_stock_products":6,"out_of_stock_products":4,` +
				`"status_breakdown":[{"status":"normal","count":40},{"status":"low_stock","count":6},{"status":"out_of_stock","count":4}]}}`,
		},
		{
			name:  "empty summary",
			repo:  &fakeDashboardRepository{},
			path:  "/api/v1/dashboard/summary",
			serve: func(h *DashboardHandler) http.HandlerFunc { return h.Summary },
			wantBody: `{"success":true,"message":"Dashboard summary retrieved successfully","data":{` +
				`"total_products":0,"total_stock":0,"low_stock_products":0,"out_of_stock_products":0,` +
				`"status_breakdown":[{"status":"normal","count":0},{"status":"low_stock","count":0},{"status":"out_of_stock","count":0}]}}`,
		},
		{
			name: "daily chart",
			repo: &fakeDashboardRepository{buckets: []domain.StockMovementBucket{
				{Period: "2026-11-02", StockIn: 120, StockOut: 45},
			}},
			path:  "/api/v1/dashboard/stock-movements?from=2026-11-01&to=2026-11-03&tz=Asia/Ho_Chi_Minh",
			serve: func(h *DashboardHandler) http.HandlerFunc { return h.StockMovementChart },
			wantBody: `{"success":true,"message":"Stock movement chart retrieved successfully","data":{` +
				`"from":"2026-11-01","to":"2026-11-03","granularity":"day","timezone":"Asia/Ho_Chi_Minh",` +
				`"points":[{"period":"2026-11-01","stock_in":0,"stock_out":0},{"period":"2026-11-02","stock_in":120,"stock_out":45},{"period":"2026-11-03","stock_in":0,"stock_out":0}],` +
				`"totals":{"stock_in":120,"stock_out":45}}}`,
		},
		{
			// "+" must be sent as %2B; a bare "+" in a query string is a space.
			name:  "encoded plus in tz",
			repo:  &fakeDashboardRepository{},
			path:  "/api/v1/dashboard/stock-movements?from=2026-11-01&to=2026-11-01&tz=Etc/GMT%2B7",
			serve: func(h *DashboardHandler) http.HandlerFunc { return h.StockMovementChart },
			wantBody: `{"success":true,"message":"Stock movement chart retrieved successfully","data":{` +
				`"from":"2026-11-01","to":"2026-11-01","granularity":"day","timezone":"Etc/GMT+7",` +
				`"points":[{"period":"2026-11-01","stock_in":0,"stock_out":0}],` +
				`"totals":{"stock_in":0,"stock_out":0}}}`,
		},
		{
			name:  "monthly chart without movements",
			repo:  &fakeDashboardRepository{},
			path:  "/api/v1/dashboard/stock-movements?from=2026-01-15&to=2026-03-20",
			serve: func(h *DashboardHandler) http.HandlerFunc { return h.StockMovementChart },
			wantBody: `{"success":true,"message":"Stock movement chart retrieved successfully","data":{` +
				`"from":"2026-01-15","to":"2026-03-20","granularity":"month","timezone":"UTC",` +
				`"points":[{"period":"2026-01-01","stock_in":0,"stock_out":0},{"period":"2026-02-01","stock_in":0,"stock_out":0},{"period":"2026-03-01","stock_in":0,"stock_out":0}],` +
				`"totals":{"stock_in":0,"stock_out":0}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.serve(newTestDashboardHandler(tt.repo))(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			if got := strings.TrimSuffix(rec.Body.String(), "\n"); got != tt.wantBody {
				t.Errorf("body mismatch\n got: %s\nwant: %s", got, tt.wantBody)
			}
		})
	}
}

func TestDashboardHandler_RejectsInvalidRequests(t *testing.T) {
	summary := func(h *DashboardHandler) http.HandlerFunc { return h.Summary }
	chart := func(h *DashboardHandler) http.HandlerFunc { return h.StockMovementChart }

	tests := []struct {
		name       string
		method     string
		path       string
		serve      func(h *DashboardHandler) http.HandlerFunc
		repo       *fakeDashboardRepository
		wantStatus int
		wantBody   string
	}{
		{"chart: bad from", http.MethodGet, "/api/v1/dashboard/stock-movements?from=01-11-2026", chart, &fakeDashboardRepository{}, http.StatusBadRequest,
			`{"success":false,"message":"invalid query parameters","data":{"fields":{"from":"must be a date in YYYY-MM-DD format"}}}`},
		{"chart: bad to", http.MethodGet, "/api/v1/dashboard/stock-movements?to=2026-13-01", chart, &fakeDashboardRepository{}, http.StatusBadRequest,
			`{"success":false,"message":"invalid query parameters","data":{"fields":{"to":"must be a date in YYYY-MM-DD format"}}}`},
		{"chart: reversed range", http.MethodGet, "/api/v1/dashboard/stock-movements?from=2026-11-02&to=2026-11-01", chart, &fakeDashboardRepository{}, http.StatusBadRequest,
			`{"success":false,"message":"invalid query parameters","data":{"fields":{"from":"must be on or before to"}}}`},
		{"chart: range too long", http.MethodGet, "/api/v1/dashboard/stock-movements?from=2026-01-01&to=2026-06-01&granularity=day", chart, &fakeDashboardRepository{}, http.StatusBadRequest,
			`{"success":false,"message":"invalid query parameters","data":{"fields":{"to":"range must be at most 92 days by day or 731 days by month"}}}`},
		{"chart: bad granularity", http.MethodGet, "/api/v1/dashboard/stock-movements?granularity=hour", chart, &fakeDashboardRepository{}, http.StatusBadRequest,
			`{"success":false,"message":"invalid query parameters","data":{"fields":{"granularity":"must be auto, day or month"}}}`},
		{"chart: bad tz", http.MethodGet, "/api/v1/dashboard/stock-movements?tz=Not/AZone", chart, &fakeDashboardRepository{}, http.StatusBadRequest,
			`{"success":false,"message":"invalid query parameters","data":{"fields":{"tz":"must be an IANA time zone name"}}}`},
		{"chart: abbreviation tz", http.MethodGet, "/api/v1/dashboard/stock-movements?tz=CET", chart, &fakeDashboardRepository{}, http.StatusBadRequest,
			`{"success":false,"message":"invalid query parameters","data":{"fields":{"tz":"must be an IANA time zone name"}}}`},
		{"chart: unencoded plus in tz", http.MethodGet, "/api/v1/dashboard/stock-movements?tz=Etc/GMT+7", chart, &fakeDashboardRepository{}, http.StatusBadRequest,
			`{"success":false,"message":"invalid query parameters","data":{"fields":{"tz":"must be an IANA time zone name"}}}`},
		{"chart: SQL in tz", http.MethodGet, "/api/v1/dashboard/stock-movements?tz=UTC%27%3B%20DROP%20TABLE%20products%3B--", chart, &fakeDashboardRepository{}, http.StatusBadRequest,
			`{"success":false,"message":"invalid query parameters","data":{"fields":{"tz":"must be an IANA time zone name"}}}`},
		{"chart: repository error", http.MethodGet, "/api/v1/dashboard/stock-movements", chart, &fakeDashboardRepository{bucketsErr: errors.New("connection reset")}, http.StatusInternalServerError,
			`{"success":false,"message":"Internal server error","data":null}`},
		{"chart: wrong method", http.MethodPost, "/api/v1/dashboard/stock-movements", chart, &fakeDashboardRepository{}, http.StatusMethodNotAllowed,
			`{"success":false,"message":"method not allowed","data":null}`},
		{"summary: repository error", http.MethodGet, "/api/v1/dashboard/summary", summary, &fakeDashboardRepository{summaryErr: errors.New("connection reset")}, http.StatusInternalServerError,
			`{"success":false,"message":"Internal server error","data":null}`},
		{"summary: inconsistent counts", http.MethodGet, "/api/v1/dashboard/summary", summary, &fakeDashboardRepository{summary: domain.InventorySummary{TotalProducts: 2, Normal: 1}}, http.StatusInternalServerError,
			`{"success":false,"message":"Internal server error","data":null}`},
		{"summary: wrong method", http.MethodPut, "/api/v1/dashboard/summary", summary, &fakeDashboardRepository{}, http.StatusMethodNotAllowed,
			`{"success":false,"message":"method not allowed","data":null}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.serve(newTestDashboardHandler(tt.repo))(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
				t.Errorf("body = %s\nwant %s", got, tt.wantBody)
			}
			if tt.wantStatus == http.StatusMethodNotAllowed {
				if rec.Header().Get("Allow") != http.MethodGet {
					t.Errorf("Allow = %q, want GET", rec.Header().Get("Allow"))
				}
				if tt.repo.calls != 0 {
					t.Error("repository must not be called for an unsupported method")
				}
			}
			if tt.wantStatus == http.StatusBadRequest && tt.repo.calls != 0 {
				t.Error("repository must not be called for invalid query parameters")
			}
		})
	}
}
