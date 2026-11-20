package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gostock/internal/domain"
	"gostock/internal/usecase"
)

// fakeOwnPasswordRepo records UpdateOwnPasswordAndRevokeTokens calls.
type fakeOwnPasswordRepo struct {
	calls int
	hash  string
}

func (f *fakeOwnPasswordRepo) UpdateOwnPasswordAndRevokeTokens(_ context.Context, _ int64, hash string, _ time.Time) error {
	f.calls++
	f.hash = hash
	return nil
}

const changePWCurrent = "current-password"

func changePasswordHandler(t *testing.T, repo *fakeOwnPasswordRepo) *AuthHandler {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(changePWCurrent), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	users := &mockUserRepo{user: &domain.User{ID: testActorID, Email: "actor@example.test", Role: usecase.RoleUser, PasswordHash: string(hash)}}
	uc := usecase.NewAuthUseCase(users, &mockRefreshRepo{}, &mockTokenMgr{}, 15*time.Minute, 7*24*time.Hour, 15*time.Second,
		usecase.WithPasswordChange(repo))
	return NewAuthHandler(uc, NewCookieManager(false, http.SameSiteLaxMode, 15*time.Minute, 7*24*time.Hour))
}

func changePasswordRequest(body, contentType string, withUser bool) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if withUser {
		req = withTestUser(req)
	}
	return req
}

func TestChangePassword_Success(t *testing.T) {
	repo := &fakeOwnPasswordRepo{}
	rec := httptest.NewRecorder()
	changePasswordHandler(t, repo).ChangePassword(rec, changePasswordRequest(
		`{"current_password":"current-password","new_password":"brand-new-password"}`, "application/json", true))

	contractAssertResponse(t, rec, http.StatusOK, `{"success":true,"message":"password changed","data":null}`)
	if repo.calls != 1 || bcrypt.CompareHashAndPassword([]byte(repo.hash), []byte("brand-new-password")) != nil {
		t.Errorf("repository calls %d; want one update with the new password's hash", repo.calls)
	}

	// Both auth cookies are cleared, with the attributes logout uses.
	cleared := map[string]string{AccessTokenCookieName: AccessTokenCookiePath, RefreshTokenCookieName: RefreshTokenCookiePath}
	for _, c := range rec.Result().Cookies() {
		path, ok := cleared[c.Name]
		if !ok {
			t.Errorf("unexpected cookie %q", c.Name)
			continue
		}
		if c.Value != "" || c.MaxAge >= 0 || c.Path != path || !c.HttpOnly {
			t.Errorf("cookie %s = %+v; want an empty, expired, HttpOnly cookie on %s", c.Name, c, path)
		}
		delete(cleared, c.Name)
	}
	for name := range cleared {
		t.Errorf("cookie %s was not cleared", name)
	}
}

func TestChangePassword_Errors(t *testing.T) {
	tests := []struct {
		name, body, contentType string
		withUser                bool
		wantStatus              int
		wantBody                string
	}{
		{"wrong current password", `{"current_password":"wrong-password","new_password":"brand-new-password"}`, "application/json", true,
			http.StatusBadRequest, `{"success":false,"code":"INVALID_CURRENT_PASSWORD","message":"current password is incorrect","data":null}`},
		{"new password equal to the current one", `{"current_password":"current-password","new_password":"current-password"}`, "application/json", true,
			http.StatusBadRequest, `{"success":false,"code":"PASSWORD_UNCHANGED","message":"new password must differ from the current password","data":null}`},
		{"new password too short", `{"current_password":"current-password","new_password":"short"}`, "application/json", true,
			http.StatusBadRequest, `{"success":false,"code":"INVALID_PASSWORD","message":"invalid password","data":null}`},
		{"new password too long", `{"current_password":"current-password","new_password":"` + strings.Repeat("p", 73) + `"}`, "application/json", true,
			http.StatusBadRequest, `{"success":false,"code":"INVALID_PASSWORD","message":"invalid password","data":null}`},
		{"not authenticated", `{"current_password":"current-password","new_password":"brand-new-password"}`, "application/json", false,
			http.StatusUnauthorized, `{"success":false,"code":"UNAUTHENTICATED","message":"authentication required","data":null}`},
		{"wrong Content-Type", `{"current_password":"current-password","new_password":"brand-new-password"}`, "text/plain", true,
			http.StatusUnsupportedMediaType, `{"success":false,"code":"UNSUPPORTED_MEDIA_TYPE","message":"Content-Type must be application/json","data":null}`},
		{"missing Content-Type", `{}`, "", true,
			http.StatusUnsupportedMediaType, `{"success":false,"code":"UNSUPPORTED_MEDIA_TYPE","message":"Content-Type must be application/json","data":null}`},
		{"malformed JSON", `{"current_password":`, "application/json", true,
			http.StatusBadRequest, `{"success":false,"code":"VALIDATION_ERROR","message":"invalid JSON request","data":null}`},
		{"unknown field", `{"current_password":"current-password","new_password":"brand-new-password","role":"admin"}`, "application/json", true,
			http.StatusBadRequest, `{"success":false,"code":"VALIDATION_ERROR","message":"invalid JSON request","data":null}`},
		{"two JSON objects", `{"current_password":"current-password","new_password":"brand-new-password"}{}`, "application/json", true,
			http.StatusBadRequest, `{"success":false,"code":"VALIDATION_ERROR","message":"request must contain exactly one JSON object","data":null}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeOwnPasswordRepo{}
			rec := httptest.NewRecorder()
			changePasswordHandler(t, repo).ChangePassword(rec, changePasswordRequest(tt.body, tt.contentType, tt.withUser))
			contractAssertResponse(t, rec, tt.wantStatus, tt.wantBody)
			contractAssertNoCookies(t, rec)
			if repo.calls != 0 {
				t.Errorf("repository called %d time(s)", repo.calls)
			}
		})
	}
}
