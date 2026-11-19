package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gostock/internal/usecase"
)

// XG-03b: POST /api/v1/auth/register with registration off. The handler
// still checks the request first (Content-Type 415, malformed JSON 400);
// a well-formed request then gets 403 REGISTRATION_DISABLED from the use
// case, whatever its email and password.
func TestRegister_DisabledResponses(t *testing.T) {
	uc := usecase.NewAuthUseCase(&mockUserRepo{}, &mockRefreshRepo{}, &mockTokenMgr{},
		15*time.Minute, 7*24*time.Hour, 15*time.Second) // registration off by default
	handler := NewAuthHandler(uc, NewCookieManager(false, http.SameSiteLaxMode, 15*time.Minute, 7*24*time.Hour))

	const disabled = `{"success":false,"code":"REGISTRATION_DISABLED","message":"registration is disabled","data":null}`
	tests := []struct {
		name, contentType, body string
		wantStatus              int
		wantBody                string
	}{
		{"valid request", "application/json", `{"email":"new@example.com","password":"valid-password"}`,
			http.StatusForbidden, disabled},
		{"invalid email and password", "application/json", `{"email":"x","password":"y"}`,
			http.StatusForbidden, disabled},
		{"malformed JSON is still 400", "application/json", `{"email":`,
			http.StatusBadRequest, `{"success":false,"code":"VALIDATION_ERROR","message":"invalid JSON request","data":null}`},
		{"two JSON objects are still 400", "application/json", `{"email":"a@b.co","password":"valid-password"}{}`,
			http.StatusBadRequest, `{"success":false,"code":"VALIDATION_ERROR","message":"request must contain exactly one JSON object","data":null}`},
		{"wrong Content-Type is still 415", "text/plain", `{"email":"new@example.com","password":"valid-password"}`,
			http.StatusUnsupportedMediaType, `{"success":false,"code":"UNSUPPORTED_MEDIA_TYPE","message":"Content-Type must be application/json","data":null}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			rec := httptest.NewRecorder()
			handler.Register(rec, req)
			contractAssertResponse(t, rec, tt.wantStatus, tt.wantBody)
			contractAssertNoCookies(t, rec)
		})
	}
}
