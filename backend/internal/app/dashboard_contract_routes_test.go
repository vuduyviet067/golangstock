package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDashboardRoutes_ExactBodies pins the full response bodies of the
// dashboard endpoints through the real router (authentication, admin role,
// then the handler), complementing the substring checks of
// TestDashboardRoutes_AuthorizationMatrix. The role is checked before the
// query, so a user with an invalid query still gets 403.
func TestDashboardRoutes_ExactBodies(t *testing.T) {
	router := setupTestRouter()

	const (
		summary = "/api/v1/dashboard/summary"
		chart   = "/api/v1/dashboard/stock-movements"

		unauthenticated = `{"success":false,"message":"authentication required","data":null}`
		forbidden       = `{"success":false,"message":"forbidden: admin role required","data":null}`
	)

	tests := []struct {
		name       string
		path       string
		token      string
		wantStatus int
		wantBody   string
	}{
		{"anonymous summary", summary, "", http.StatusUnauthorized, unauthenticated},
		{"anonymous chart", chart, "", http.StatusUnauthorized, unauthenticated},
		{"anonymous chart with invalid query", chart + "?granularity=week", "", http.StatusUnauthorized, unauthenticated},

		{"user summary", summary, "user-token", http.StatusForbidden, forbidden},
		{"user chart", chart + "?from=2026-11-01&to=2026-11-03", "user-token", http.StatusForbidden, forbidden},
		{"user chart with invalid from", chart + "?from=11/01/2026", "user-token", http.StatusForbidden, forbidden},
		{"user chart with invalid granularity", chart + "?granularity=week", "user-token", http.StatusForbidden, forbidden},
		{"user chart with invalid tz", chart + "?tz=Mars/Base", "user-token", http.StatusForbidden, forbidden},

		{"admin chart with invalid from", chart + "?from=11/01/2026", "admin-token", http.StatusBadRequest,
			`{"success":false,"message":"invalid query parameters","data":{"fields":{"from":"must be a date in YYYY-MM-DD format"}}}`},
		{"admin chart with invalid granularity", chart + "?granularity=week", "admin-token", http.StatusBadRequest,
			`{"success":false,"message":"invalid query parameters","data":{"fields":{"granularity":"must be auto, day or month"}}}`},
		{"admin chart with invalid tz", chart + "?tz=Mars/Base", "admin-token", http.StatusBadRequest,
			`{"success":false,"message":"invalid query parameters","data":{"fields":{"tz":"must be an IANA time zone name"}}}`},

		{"admin summary", summary, "admin-token", http.StatusOK,
			`{"success":true,"message":"Dashboard summary retrieved successfully","data":{` +
				`"total_products":3,"total_stock":12,"low_stock_products":1,"out_of_stock_products":1,` +
				`"status_breakdown":[{"status":"normal","count":1},{"status":"low_stock","count":1},{"status":"out_of_stock","count":1}]}}`},
		{"admin chart", chart + "?from=2026-11-01&to=2026-11-03", "admin-token", http.StatusOK,
			`{"success":true,"message":"Stock movement chart retrieved successfully","data":{` +
				`"from":"2026-11-01","to":"2026-11-03","granularity":"day","timezone":"UTC",` +
				`"points":[{"period":"2026-11-01","stock_in":0,"stock_out":0},{"period":"2026-11-02","stock_in":0,"stock_out":0},{"period":"2026-11-03","stock_in":0,"stock_out":0}],` +
				`"totals":{"stock_in":0,"stock_out":0}}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
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
