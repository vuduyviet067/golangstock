package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestUserRoutes_ExactBodies pins the full response bodies of the user
// management endpoints through the real router (authentication, admin role,
// then the handler), complementing the status-only checks of
// TestUserRoutes_AuthorizationMatrix. User management is admin only; the role
// is checked before the query, the ID and the body.
//
// setupTestRouter's fakeUserRepo stores one user (ID 1, no timestamps) and
// returns an empty list.
func TestUserRoutes_ExactBodies(t *testing.T) {
	router := setupTestRouter()

	const (
		users = "/api/v1/users"
		user1 = "/api/v1/users/1"

		unauthenticated = `{"success":false,"code":"UNAUTHENTICATED","message":"authentication required","data":null}`
		invalidToken    = `{"success":false,"code":"UNAUTHENTICATED","message":"invalid or expired token","data":null}`
		forbidden       = `{"success":false,"code":"FORBIDDEN","message":"forbidden: admin role required","data":null}`
	)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string
		wantStatus int
		wantBody   string
	}{
		{"anonymous list", http.MethodGet, users, "", "", http.StatusUnauthorized, unauthenticated},
		{"anonymous get", http.MethodGet, user1, "", "", http.StatusUnauthorized, unauthenticated},
		{"anonymous update", http.MethodPatch, user1, `{"role":"admin"}`, "", http.StatusUnauthorized, unauthenticated},
		{"invalid token list", http.MethodGet, users, "", "bogus-token", http.StatusUnauthorized, invalidToken},

		{"user list", http.MethodGet, users, "", "user-token", http.StatusForbidden, forbidden},
		{"user list with invalid page", http.MethodGet, users + "?page=abc", "", "user-token", http.StatusForbidden, forbidden},
		{"user get", http.MethodGet, user1, "", "user-token", http.StatusForbidden, forbidden},
		{"user get with invalid id", http.MethodGet, "/api/v1/users/abc", "", "user-token", http.StatusForbidden, forbidden},
		{"user update", http.MethodPatch, user1, `{"role":"admin"}`, "user-token", http.StatusForbidden, forbidden},
		{"user update with invalid body", http.MethodPatch, user1, `{"password":"x"}`, "user-token", http.StatusForbidden, forbidden},

		{"admin list", http.MethodGet, users, "", "admin-token", http.StatusOK,
			`{"success":true,"message":"Users retrieved successfully","data":{"items":[],"pagination":{"page":1,"limit":10,"total":0,"total_pages":0}}}`},
		{"admin list with paging", http.MethodGet, users + "?page=3&limit=25&q=x", "", "admin-token", http.StatusOK,
			`{"success":true,"message":"Users retrieved successfully","data":{"items":[],"pagination":{"page":3,"limit":25,"total":0,"total_pages":0}}}`},
		{"admin list invalid page", http.MethodGet, users + "?page=0", "", "admin-token", http.StatusBadRequest,
			`{"success":false,"code":"INVALID_PAGE","message":"invalid page","data":null}`},
		{"admin list invalid limit", http.MethodGet, users + "?limit=101", "", "admin-token", http.StatusBadRequest,
			`{"success":false,"code":"INVALID_LIMIT","message":"invalid limit","data":null}`},
		{"admin get", http.MethodGet, user1, "", "admin-token", http.StatusOK,
			`{"success":true,"message":"User retrieved successfully","data":{"id":1,"email":"target@example.com","role":"user","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}}`},
		{"admin get missing", http.MethodGet, "/api/v1/users/2", "", "admin-token", http.StatusNotFound,
			`{"success":false,"code":"USER_NOT_FOUND","message":"user not found","data":null}`},
		{"admin get invalid id", http.MethodGet, "/api/v1/users/abc", "", "admin-token", http.StatusBadRequest,
			`{"success":false,"code":"INVALID_USER_ID","message":"invalid user ID","data":null}`},
		{"admin update role", http.MethodPatch, user1, `{"role":"admin"}`, "admin-token", http.StatusOK,
			`{"success":true,"message":"User updated successfully","data":{"id":1,"email":"target@example.com","role":"admin","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}}`},
		{"admin update invalid role", http.MethodPatch, user1, `{"role":"superadmin"}`, "admin-token", http.StatusBadRequest,
			`{"success":false,"code":"INVALID_USER_ROLE","message":"invalid user role","data":null}`},
		{"admin update unknown field", http.MethodPatch, user1, `{"password":"x"}`, "admin-token", http.StatusBadRequest,
			`{"success":false,"code":"VALIDATION_ERROR","message":"invalid request body","data":null}`},
		{"admin update missing", http.MethodPatch, "/api/v1/users/2", `{"role":"admin"}`, "admin-token", http.StatusNotFound,
			`{"success":false,"code":"USER_NOT_FOUND","message":"user not found","data":null}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
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
			got := strings.TrimSuffix(rec.Body.String(), "\n")
			if got != tt.wantBody {
				t.Errorf("body mismatch\n got: %s\nwant: %s", got, tt.wantBody)
			}
			if strings.Contains(strings.ToLower(got), "password_hash") {
				t.Errorf("body exposes password_hash: %s", got)
			}
		})
	}
}
