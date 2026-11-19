package app

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// TestStockMovementRoutes_ExactBodies pins the full response bodies of both
// history endpoints through the real router (authentication, the admin role
// for the global history, then the handler), complementing the substring
// checks of TestStockMovementRoutes_AuthorizationMatrix. Product history is
// open to every signed-in user; global history is admin only, checked before
// the query.
func TestStockMovementRoutes_ExactBodies(t *testing.T) {
	router := setupTestRouter()

	const (
		productHistory = "/api/v1/products/1/movements"
		globalHistory  = "/api/v1/stock-movements"

		unauthenticated = `{"success":false,"message":"authentication required","data":null}`
		forbidden       = `{"success":false,"message":"forbidden: admin role required","data":null}`
	)
	emptyPage := func(page, limit int) string {
		return `{"success":true,"message":"Stock movements retrieved successfully","data":{"items":[],"pagination":{"page":` +
			strconv.Itoa(page) + `,"limit":` + strconv.Itoa(limit) + `,"total":0,"total_pages":0}}}`
	}

	tests := []struct {
		name       string
		path       string
		token      string
		wantStatus int
		wantBody   string
	}{
		{"anonymous product history", productHistory, "", http.StatusUnauthorized, unauthenticated},
		{"anonymous global history", globalHistory, "", http.StatusUnauthorized, unauthenticated},

		{"user product history", productHistory, "user-token", http.StatusOK, emptyPage(1, 10)},
		{"user product history with paging", productHistory + "?page=3&limit=25", "user-token", http.StatusOK, emptyPage(3, 25)},
		{"user product history invalid page", productHistory + "?page=0", "user-token", http.StatusBadRequest,
			`{"success":false,"message":"invalid page","data":null}`},
		{"user global history", globalHistory, "user-token", http.StatusForbidden, forbidden},
		{"user global history invalid query", globalHistory + "?page=abc&limit=999", "user-token", http.StatusForbidden, forbidden},

		{"admin product history", productHistory, "admin-token", http.StatusOK, emptyPage(1, 10)},
		{"admin global history", globalHistory + "?page=2&limit=100", "admin-token", http.StatusOK, emptyPage(2, 100)},
		{"admin global history invalid page", globalHistory + "?page=abc", "admin-token", http.StatusBadRequest,
			`{"success":false,"message":"invalid page","data":null}`},
		{"admin global history invalid limit", globalHistory + "?limit=101", "admin-token", http.StatusBadRequest,
			`{"success":false,"message":"invalid limit","data":null}`},
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
