package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/usecase"
)

// Auth handler contract after the use case started returning response DTOs
// (Mapper refactor Step 3.5). It complements the Step 1 characterization
// tests (auth_user_contract_test.go) with the remaining error paths and the
// cookie rules: a refresh that fails authentication (401) clears both cookies,
// a concurrent refresh (409) and a server error (500) leave them alone.

// failingLookupUserRepo fails every user lookup, as a broken database would.
type failingLookupUserRepo struct {
	mockUserRepo
	err error
}

func (r *failingLookupUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return nil, r.err
}

func TestAuthContract_ServerErrorsKeepCookies(t *testing.T) {
	const internal = `{"success":false,"code":"INTERNAL_ERROR","message":"Internal server error","data":null}`
	dbErr := errors.New("connection reset")

	t.Run("register repository error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		contractAuthHandler(&contractRegisterRepo{createErr: dbErr}).
			Register(rec, contractJSONRequest(http.MethodPost, "/api/v1/auth/register", `{"email":"a@example.com","password":"Password123"}`))

		contractAssertResponse(t, rec, http.StatusInternalServerError, internal)
		contractAssertNoCookies(t, rec)
	})

	t.Run("login repository error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		contractAuthHandler(&failingLookupUserRepo{err: dbErr}).
			Login(rec, contractJSONRequest(http.MethodPost, "/api/v1/auth/login", `{"email":"a@example.com","password":"Password123"}`))

		contractAssertResponse(t, rec, http.StatusInternalServerError, internal)
		contractAssertNoCookies(t, rec)
	})
}

func TestAuthContract_RefreshFailures(t *testing.T) {
	now := time.Now().UTC()
	justRotated := now.Add(-2 * time.Second)
	longAgo := now.Add(-time.Hour)
	replaced := "replaced-hash"
	user := &domain.User{ID: 9, Email: "refresh@example.com", Role: "user"}

	tests := []struct {
		name       string
		users      *mockUserRepo
		token      *domain.RefreshToken
		wantStatus int
		wantBody   string
		wantClear  bool
	}{
		{"revoked token", &mockUserRepo{user: user},
			&domain.RefreshToken{ID: 1, UserID: 9, ExpiresAt: now.Add(time.Hour), RevokedAt: &longAgo},
			http.StatusUnauthorized, `{"success":false,"code":"INVALID_REFRESH_TOKEN","message":"invalid or expired refresh token","data":null}`, true},
		{"reused token after the grace period", &mockUserRepo{user: user},
			&domain.RefreshToken{ID: 1, UserID: 9, ExpiresAt: now.Add(time.Hour), RevokedAt: &longAgo, ReplacedByHash: &replaced},
			http.StatusUnauthorized, `{"success":false,"code":"INVALID_REFRESH_TOKEN","message":"invalid or expired refresh token","data":null}`, true},
		{"deleted user", &mockUserRepo{},
			&domain.RefreshToken{ID: 1, UserID: 9, ExpiresAt: now.Add(time.Hour)},
			http.StatusUnauthorized, `{"success":false,"code":"INVALID_SESSION","message":"invalid or expired session","data":null}`, true},
		{"concurrent refresh within the grace period", &mockUserRepo{user: user},
			&domain.RefreshToken{ID: 1, UserID: 9, ExpiresAt: now.Add(time.Hour), RevokedAt: &justRotated, ReplacedByHash: &replaced},
			http.StatusConflict, `{"success":false,"code":"CONCURRENT_REFRESH","message":"concurrent refresh in progress","data":null}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := setupTestAuthHandler(tt.users, &mockRefreshRepo{token: tt.token})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
			req.AddCookie(&http.Cookie{Name: RefreshTokenCookieName, Value: "some-refresh-token"})
			rec := httptest.NewRecorder()

			handler.Refresh(rec, req)

			contractAssertResponse(t, rec, tt.wantStatus, tt.wantBody)
			if tt.wantClear {
				contractAssertClearedCookies(t, rec)
			} else {
				contractAssertNoCookies(t, rec)
			}
		})
	}
}

func TestAuthContract_WrongMethods(t *testing.T) {
	handler := setupTestAuthHandler(&mockUserRepo{}, &mockRefreshRepo{})
	for _, tc := range []struct {
		name, method, allow string
		serve               http.HandlerFunc
	}{
		{"login", http.MethodGet, http.MethodPost, handler.Login},
		{"refresh", http.MethodGet, http.MethodPost, handler.Refresh},
		{"me", http.MethodPost, http.MethodGet, handler.Me},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			serveRoute(rec, httptest.NewRequest(tc.method, "/api/v1/auth/"+tc.name, nil), tc.allow, "/api/v1/auth/"+tc.name, tc.serve)

			contractAssertResponse(t, rec, http.StatusMethodNotAllowed, `{"success":false,"code":"METHOD_NOT_ALLOWED","message":"method not allowed","data":null}`)
			contractAssertNoCookies(t, rec)
			if got := rec.Header().Get("Allow"); got != tc.allow {
				t.Errorf("Allow = %q, want %q", got, tc.allow)
			}
		})
	}
}

// With the production-style cookie settings (Secure, SameSite=Strict) the
// same paths and lifetimes are kept, and the body is unchanged.
func TestAuthContract_SecureStrictCookies(t *testing.T) {
	newHandler := func(users *mockUserRepo, tokens *mockRefreshRepo) *AuthHandler {
		uc := usecase.NewAuthUseCase(users, tokens, &mockTokenMgr{}, 15*time.Minute, 7*24*time.Hour, 15*time.Second)
		return NewAuthHandler(uc, NewCookieManager(true, http.SameSiteStrictMode, 15*time.Minute, 7*24*time.Hour))
	}

	t.Run("login sets secure strict cookies", func(t *testing.T) {
		hash := contractBcryptHash(t, "Password123")
		handler := newHandler(&mockUserRepo{user: &domain.User{ID: 7, Email: "a@example.com", PasswordHash: hash, Role: "user"}}, &mockRefreshRepo{})
		rec := httptest.NewRecorder()
		handler.Login(rec, contractJSONRequest(http.MethodPost, "/api/v1/auth/login", `{"email":"a@example.com","password":"Password123"}`))

		contractAssertResponse(t, rec, http.StatusOK,
			`{"success":true,"message":"Login successful","data":{"id":7,"email":"a@example.com","role":"user","created_at":""}}`)
		raw := rec.Header().Values("Set-Cookie")
		patterns := []*regexp.Regexp{
			regexp.MustCompile(`^access_token=test-jwt-token; Path=/; Expires=[^;]+ GMT; Max-Age=900; HttpOnly; Secure; SameSite=Strict$`),
			regexp.MustCompile(`^refresh_token=[A-Za-z0-9_-]{43}; Path=/api/v1/auth; Expires=[^;]+ GMT; Max-Age=604800; HttpOnly; Secure; SameSite=Strict$`),
		}
		if len(raw) != len(patterns) {
			t.Fatalf("Set-Cookie = %q, want access_token then refresh_token", raw)
		}
		for i, re := range patterns {
			if !re.MatchString(raw[i]) {
				t.Errorf("Set-Cookie[%d] = %q, want match %s", i, raw[i], re)
			}
		}
		contractAssertNoSensitiveData(t, rec.Body.String(), hash, "Password123", "test-jwt-token")
	})

	t.Run("logout clears secure strict cookies", func(t *testing.T) {
		handler := newHandler(&mockUserRepo{}, &mockRefreshRepo{})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.AddCookie(&http.Cookie{Name: RefreshTokenCookieName, Value: "active-token"})
		rec := httptest.NewRecorder()
		handler.Logout(rec, req)

		contractAssertResponse(t, rec, http.StatusOK, `{"success":true,"message":"logged out","data":{"message":"logged out"}}`)
		want := []string{
			"access_token=; Path=/; " + contractClearedExpires + "; Max-Age=0; HttpOnly; Secure; SameSite=Strict",
			"refresh_token=; Path=/api/v1/auth; " + contractClearedExpires + "; Max-Age=0; HttpOnly; Secure; SameSite=Strict",
		}
		got := rec.Header().Values("Set-Cookie")
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("Set-Cookie = %q, want %q", got, want)
		}
	})
}
