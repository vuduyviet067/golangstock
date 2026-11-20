package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	productHTTP "gostock/internal/handle/http"
	"gostock/internal/platform/jwt"
	"gostock/internal/repository/postgres"
	"gostock/internal/testdb"
	"gostock/internal/usecase"
)

// TestRegistrationDisabled_PostgreSQL (XG-03b): with ALLOW_REGISTRATION off
// (the default), POST /api/v1/auth/register through the application router
// answers 403 REGISTRATION_DISABLED and writes nothing. Registration with
// the flag on is covered by TestAuthUserContract_PostgreSQL.
func TestRegistrationDisabled_PostgreSQL(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()

	email := fmt.Sprintf("registration-off-%d@example.com", time.Now().UnixNano())
	// Registered before the request: should registration open by mistake,
	// the account is still removed.
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM users WHERE email = $1`, email); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
	})

	jwtService, err := jwt.NewJWTService("a-very-long-and-secure-secret-key-32-bytes-minimum!", "gostock", "gostock-api", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	userRepo := postgres.NewUserRepository(db)
	authUC := usecase.NewAuthUseCase(userRepo, postgres.NewRefreshTokenRepository(db), jwtService,
		15*time.Minute, 7*24*time.Hour, 15*time.Second, usecase.WithRegistration(false))
	router := NewRouter(db,
		productHTTP.NewProductHandler(usecase.NewProductUseCase(postgres.NewProductRepository(db))),
		productHTTP.NewAuthHandler(authUC, productHTTP.NewCookieManager(false, http.SameSiteLaxMode, 15*time.Minute, 7*24*time.Hour)),
		authUC,
		productHTTP.NewUserHandler(usecase.NewUserUseCase(userRepo)),
		nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		strings.NewReader(`{"email":"`+email+`","password":"valid-password-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	pgContractAssertBody(t, "register disabled", rec, http.StatusForbidden,
		`{"success":false,"code":"REGISTRATION_DISABLED","message":"registration is disabled","data":null}`)
	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Errorf("register disabled set %d cookie(s)", len(cookies))
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE email = $1`, email).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Errorf("%d user row(s) created with registration disabled", count)
	}
}
