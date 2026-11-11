package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
)

// fakeDashboardRepo returns fixed figures; the route tests only check
// authentication, authorization and routing.
type fakeDashboardRepo struct{}

func (f *fakeDashboardRepo) InventorySummary(ctx context.Context) (domain.InventorySummary, error) {
	return domain.InventorySummary{TotalProducts: 3, TotalStock: 12, Normal: 1, LowStock: 1, OutOfStock: 1}, nil
}

func (f *fakeDashboardRepo) StockMovementBuckets(ctx context.Context, granularity, timezone string, from, to time.Time) ([]domain.StockMovementBucket, error) {
	return []domain.StockMovementBucket{}, nil
}

func TestDashboardRoutes_AuthorizationMatrix(t *testing.T) {
	router := setupTestRouter()

	const (
		summary = "/api/v1/dashboard/summary"
		chart   = "/api/v1/dashboard/stock-movements"
	)

	tests := []struct {
		name       string
		method     string
		path       string
		token      string
		wantStatus int
		wantBody   string
	}{
		{"Anonymous - summary", http.MethodGet, summary, "", http.StatusUnauthorized, `"message":"authentication required"`},
		{"Anonymous - chart", http.MethodGet, chart, "", http.StatusUnauthorized, `"message":"authentication required"`},
		{"Invalid token - summary", http.MethodGet, summary, "bogus-token", http.StatusUnauthorized, `"message":"invalid or expired token"`},
		{"Invalid token - chart", http.MethodGet, chart, "bogus-token", http.StatusUnauthorized, `"message":"invalid or expired token"`},

		{"User - summary", http.MethodGet, summary, "user-token", http.StatusOK, `"message":"Dashboard summary retrieved successfully"`},
		{"User - chart", http.MethodGet, chart, "user-token", http.StatusOK, `"message":"Stock movement chart retrieved successfully"`},
		{"Admin - summary", http.MethodGet, summary, "admin-token", http.StatusOK, `"total_products":3`},
		{"Admin - chart", http.MethodGet, chart + "?from=2026-11-01&to=2026-11-03", "admin-token", http.StatusOK, `"granularity":"day"`},

		{"User - chart invalid from", http.MethodGet, chart + "?from=11/01/2026", "user-token", http.StatusBadRequest, `"fields":{"from":"must be a date in YYYY-MM-DD format"}`},
		{"User - chart invalid granularity", http.MethodGet, chart + "?granularity=week", "user-token", http.StatusBadRequest, `"fields":{"granularity":"must be auto, day or month"}`},
		{"Admin - chart invalid tz", http.MethodGet, chart + "?tz=Mars/Base", "admin-token", http.StatusBadRequest, `"fields":{"tz":"must be an IANA time zone name"}`},

		{"User - POST summary", http.MethodPost, summary, "user-token", http.StatusMethodNotAllowed, `"message":"method not allowed"`},
		{"Admin - DELETE chart", http.MethodDelete, chart, "admin-token", http.StatusMethodNotAllowed, `"message":"method not allowed"`},
		{"Admin - unknown dashboard path", http.MethodGet, "/api/v1/dashboard", "admin-token", http.StatusNotFound, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %s, want it to contain %s", rec.Body.String(), tt.wantBody)
			}
		})
	}
}
