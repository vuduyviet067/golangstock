package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gostock/internal/domain"
	productHTTP "gostock/internal/handle/http"
	"gostock/internal/platform/jwt"
	"gostock/internal/repository/postgres"
	"gostock/internal/testdb"
	"gostock/internal/usecase"
)

// TestChangePassword_PostgreSQL (XG-03b): through the application router and
// the integration test database, a logged-in user changes their password;
// afterwards the old refresh token and the old password are refused and the
// new password logs in.
func TestChangePassword_PostgreSQL(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()

	email := fmt.Sprintf("change-password-%d@example.com", time.Now().UnixNano())
	// Registered before the user is created; refresh tokens cascade.
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM users WHERE email = $1`, email); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
	})

	const oldPW, newPW = "old-password-1", "new-password-2"
	hash, err := bcrypt.GenerateFromPassword([]byte(oldPW), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	userRepo := postgres.NewUserRepository(db)
	if err := userRepo.Create(ctx, &domain.User{Email: email, PasswordHash: string(hash), Role: usecase.RoleUser}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	jwtService, err := jwt.NewJWTService("a-very-long-and-secure-secret-key-32-bytes-minimum!", "gostock", "gostock-api", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	authUC := usecase.NewAuthUseCase(userRepo, postgres.NewRefreshTokenRepository(db), jwtService,
		15*time.Minute, 7*24*time.Hour, 15*time.Second, usecase.WithPasswordChange(userRepo))
	router := NewRouter(db,
		productHTTP.NewProductHandler(usecase.NewProductUseCase(postgres.NewProductRepository(db))),
		productHTTP.NewAuthHandler(authUC, productHTTP.NewCookieManager(false, http.SameSiteLaxMode, 15*time.Minute, 7*24*time.Hour)),
		authUC,
		productHTTP.NewUserHandler(usecase.NewUserUseCase(userRepo)),
		nil, nil, nil, nil)

	send := func(path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	login := func(password string) *httptest.ResponseRecorder {
		return send("/api/v1/auth/login", `{"email":"`+email+`","password":"`+password+`"}`)
	}
	cookieNamed := func(rec *httptest.ResponseRecorder, name string) *http.Cookie {
		for _, c := range rec.Result().Cookies() {
			if c.Name == name {
				return c
			}
		}
		return nil
	}

	// 1. Log in with the old password.
	rec := login(oldPW)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	access, refresh := cookieNamed(rec, productHTTP.AccessTokenCookieName), cookieNamed(rec, productHTTP.RefreshTokenCookieName)
	if access == nil || refresh == nil {
		t.Fatal("login set no auth cookies")
	}

	// 2. Change the password.
	rec = send("/api/v1/auth/change-password",
		`{"current_password":"`+oldPW+`","new_password":"`+newPW+`"}`, access, refresh)
	pgContractAssertBody(t, "change password", rec, http.StatusOK, `{"success":true,"message":"password changed","data":null}`)
	for _, name := range []string{productHTTP.AccessTokenCookieName, productHTTP.RefreshTokenCookieName} {
		if c := cookieNamed(rec, name); c == nil || c.Value != "" || c.MaxAge >= 0 {
			t.Errorf("change password did not clear cookie %s: %+v", name, c)
		}
	}
	for _, s := range []string{oldPW, newPW, "$2a$", "$2b$"} {
		if strings.Contains(rec.Body.String(), s) {
			t.Errorf("change password response contains %q", s)
		}
	}

	// 3. The refresh token issued before the change is revoked.
	rec = send("/api/v1/auth/refresh", ``, refresh)
	pgContractAssertBody(t, "refresh with the old token", rec, http.StatusUnauthorized,
		`{"success":false,"code":"INVALID_REFRESH_TOKEN","message":"invalid or expired refresh token","data":null}`)

	// 4. The old password no longer logs in; 5. the new one does.
	rec = login(oldPW)
	pgContractAssertBody(t, "login with the old password", rec, http.StatusUnauthorized,
		`{"success":false,"code":"INVALID_CREDENTIALS","message":"invalid email or password","data":null}`)
	if rec = login(newPW); rec.Code != http.StatusOK {
		t.Errorf("login with the new password: %d %s", rec.Code, rec.Body.String())
	}

	// A wrong current password is a 400 that keeps the session.
	access2 := cookieNamed(rec, productHTTP.AccessTokenCookieName)
	rec = send("/api/v1/auth/change-password", `{"current_password":"wrong-password","new_password":"another-password-3"}`, access2)
	pgContractAssertBody(t, "wrong current password", rec, http.StatusBadRequest,
		`{"success":false,"code":"INVALID_CURRENT_PASSWORD","message":"current password is incorrect","data":null}`)
	if len(rec.Result().Cookies()) != 0 {
		t.Error("a refused change cleared the cookies")
	}
}
