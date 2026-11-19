package http

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/repository"
	"gostock/internal/usecase"

	"golang.org/x/crypto/bcrypt"
)

// Characterization tests for the Auth and User Management HTTP contract.
//
// They pin the exact response bodies (envelope, field names, field order,
// value types, null/[]/""/zero-value rules), status codes and cookies that the
// handlers produce today, so that a later refactor of the domain -> DTO mapping
// cannot change the API contract unnoticed. Expected bodies are hand-written
// literals; they are never built by calling the mapping code under test.
//
// Timestamp rules pinned here (they differ on purpose and must not be merged):
//   - Auth (dto.AuthUserResponse.created_at): string, time.RFC3339 to the
//     second in the value's own offset; zero time -> "".
//   - Users (dto.UserResponse.created_at/updated_at): time.Time JSON, i.e.
//     RFC3339Nano with trailing zeros trimmed, in the value's own offset;
//     zero time -> "0001-01-01T00:00:00Z".

var (
	// contractCreatedAt has nanoseconds so the two timestamp rules differ.
	contractCreatedAt = time.Date(2026, 3, 4, 5, 6, 7, 891234567, time.UTC)
	// contractUpdatedAt ends in zeros to pin RFC3339Nano trimming.
	contractUpdatedAt = time.Date(2026, 3, 5, 6, 7, 8, 120000000, time.UTC)
	// contractICT is a fixed +07:00 zone to pin that the offset is kept as-is.
	contractICT = time.FixedZone("ICT", 7*60*60)
)

const (
	contractPasswordHashMarker = "$2a$10$contract-secret-hash-must-never-leak"
	contractClearedExpires     = "Expires=Thu, 01 Jan 1970 00:00:01 GMT"
)

var (
	contractAccessCookieRe  = regexp.MustCompile(`^access_token=test-jwt-token; Path=/; Expires=[A-Z][a-z]{2}, \d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2} GMT; Max-Age=900; HttpOnly; SameSite=Lax$`)
	contractRefreshCookieRe = regexp.MustCompile(`^refresh_token=[A-Za-z0-9_-]{43}; Path=/api/v1/auth; Expires=[A-Z][a-z]{2}, \d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2} GMT; Max-Age=604800; HttpOnly; SameSite=Lax$`)

	contractClearedCookies = []string{
		"access_token=; Path=/; " + contractClearedExpires + "; Max-Age=0; HttpOnly; SameSite=Lax",
		"refresh_token=; Path=/api/v1/auth; " + contractClearedExpires + "; Max-Age=0; HttpOnly; SameSite=Lax",
	}
)

// contractRegisterRepo behaves like the PostgreSQL repository on Create: it
// assigns the ID and the timestamps. Other methods come from mockUserRepo.
type contractRegisterRepo struct {
	mockUserRepo
	id        int64
	createdAt time.Time
	createErr error
}

func (r *contractRegisterRepo) Create(ctx context.Context, u *domain.User) error {
	if r.createErr != nil {
		return r.createErr
	}
	u.ID = r.id
	u.CreatedAt = r.createdAt
	u.UpdatedAt = r.createdAt
	return nil
}

func contractJSONRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// contractAssertResponse pins status, Content-Type and the exact body bytes.
func contractAssertResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantBody string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Errorf("status = %d, want %d", rec.Code, wantStatus)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if got := rec.Body.String(); got != wantBody+"\n" {
		t.Errorf("body mismatch\n got: %s\nwant: %s", strings.TrimSuffix(got, "\n"), wantBody)
	}
}

// contractAssertNoSensitiveData fails if a success body exposes a password or
// token field, anything named like a hash, or any of the given secret values.
func contractAssertNoSensitiveData(t *testing.T, body string, secrets ...string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, key := range []string{"password", "passwordhash", "password_hash", "token", "access_token", "refresh_token"} {
		if strings.Contains(lower, `"`+key+`":`) {
			t.Errorf("response body must not contain field %q: %s", key, body)
		}
	}
	if strings.Contains(lower, "hash") {
		t.Errorf("response body must not mention a hash: %s", body)
	}
	for _, secret := range append(secrets, contractPasswordHashMarker) {
		if secret != "" && strings.Contains(body, secret) {
			t.Errorf("response body leaks secret value %q: %s", secret, body)
		}
	}
}

func contractAssertNoCookies(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("Set-Cookie = %q, want none", got)
	}
}

func contractAssertClearedCookies(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	got := rec.Header().Values("Set-Cookie")
	if len(got) != len(contractClearedCookies) {
		t.Fatalf("Set-Cookie = %q, want %q", got, contractClearedCookies)
	}
	for i := range contractClearedCookies {
		if got[i] != contractClearedCookies[i] {
			t.Errorf("Set-Cookie[%d] = %q, want %q", i, got[i], contractClearedCookies[i])
		}
	}
}

// contractAssertIssuedCookies pins the cookies set by Login and Refresh. Token
// values and Expires are dynamic: the access token comes from mockTokenMgr, the
// refresh token is a random 32-byte base64url value, and Expires must equal
// request time + TTL (to the second).
func contractAssertIssuedCookies(t *testing.T, rec *httptest.ResponseRecorder, before, after time.Time) (refreshValue string) {
	t.Helper()
	raw := rec.Header().Values("Set-Cookie")
	if len(raw) != 2 {
		t.Fatalf("Set-Cookie = %q, want exactly access_token then refresh_token", raw)
	}
	if !contractAccessCookieRe.MatchString(raw[0]) {
		t.Errorf("access cookie = %q, want match %s", raw[0], contractAccessCookieRe)
	}
	if !contractRefreshCookieRe.MatchString(raw[1]) {
		t.Errorf("refresh cookie = %q, want match %s", raw[1], contractRefreshCookieRe)
	}

	cookies := rec.Result().Cookies()
	ttls := map[string]time.Duration{
		AccessTokenCookieName:  15 * time.Minute,
		RefreshTokenCookieName: 7 * 24 * time.Hour,
	}
	for _, c := range cookies {
		ttl, ok := ttls[c.Name]
		if !ok {
			t.Errorf("unexpected cookie %q", c.Name)
			continue
		}
		lo := before.Add(ttl).Truncate(time.Second)
		hi := after.Add(ttl).Add(time.Second)
		if c.Expires.Before(lo) || c.Expires.After(hi) {
			t.Errorf("%s Expires = %v, want within [%v, %v]", c.Name, c.Expires, lo, hi)
		}
		if c.Name == RefreshTokenCookieName {
			refreshValue = c.Value
		}
	}
	return refreshValue
}

func contractBcryptHash(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return string(h)
}

// ---------------------------------------------------------------------------
// Auth: Register
// ---------------------------------------------------------------------------

func TestAuthContract_Register(t *testing.T) {
	tests := []struct {
		name       string
		repo       *contractRegisterRepo
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "created_at RFC3339 to the second, email normalized, role user",
			repo:       &contractRegisterRepo{id: 42, createdAt: contractCreatedAt},
			body:       `{"email":"  New.User@Example.COM ","password":"Password123"}`,
			wantStatus: http.StatusCreated,
			wantBody:   `{"success":true,"message":"User registered successfully","data":{"id":42,"email":"new.user@example.com","role":"user","created_at":"2026-03-04T05:06:07Z"}}`,
		},
		{
			name:       "created_at keeps the value's own offset",
			repo:       &contractRegisterRepo{id: 43, createdAt: contractCreatedAt.In(contractICT)},
			body:       `{"email":"offset@example.com","password":"Password123"}`,
			wantStatus: http.StatusCreated,
			wantBody:   `{"success":true,"message":"User registered successfully","data":{"id":43,"email":"offset@example.com","role":"user","created_at":"2026-03-04T12:06:07+07:00"}}`,
		},
		{
			name:       "zero created_at is an empty string",
			repo:       &contractRegisterRepo{id: 44},
			body:       `{"email":"zero@example.com","password":"Password123"}`,
			wantStatus: http.StatusCreated,
			wantBody:   `{"success":true,"message":"User registered successfully","data":{"id":44,"email":"zero@example.com","role":"user","created_at":""}}`,
		},
		{
			name:       "duplicate email",
			repo:       &contractRegisterRepo{createErr: repository.ErrEmailExists},
			body:       `{"email":"taken@example.com","password":"Password123"}`,
			wantStatus: http.StatusConflict,
			wantBody:   `{"success":false,"message":"email already exists","data":null}`,
		},
		{
			name:       "invalid email",
			repo:       &contractRegisterRepo{},
			body:       `{"email":"not-an-email","password":"Password123"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"success":false,"message":"invalid email","data":null}`,
		},
		{
			name:       "password too short",
			repo:       &contractRegisterRepo{},
			body:       `{"email":"short@example.com","password":"short"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"success":false,"message":"invalid password","data":null}`,
		},
		{
			name:       "unknown field",
			repo:       &contractRegisterRepo{},
			body:       `{"email":"a@example.com","password":"Password123","role":"admin"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"success":false,"message":"invalid JSON request","data":null}`,
		},
		{
			name:       "trailing JSON value",
			repo:       &contractRegisterRepo{},
			body:       `{"email":"a@example.com","password":"Password123"}{}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"success":false,"message":"request must contain exactly one JSON object","data":null}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := contractAuthHandler(tc.repo)

			rec := httptest.NewRecorder()
			handler.Register(rec, contractJSONRequest(http.MethodPost, "/api/v1/auth/register", tc.body))

			contractAssertResponse(t, rec, tc.wantStatus, tc.wantBody)
			contractAssertNoCookies(t, rec)
			if tc.wantStatus == http.StatusCreated {
				contractAssertNoSensitiveData(t, rec.Body.String(), "Password123")
			}
		})
	}

	t.Run("wrong method", func(t *testing.T) {
		handler := setupTestAuthHandler(&mockUserRepo{}, &mockRefreshRepo{})
		rec := httptest.NewRecorder()
		handler.Register(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/register", nil))

		contractAssertResponse(t, rec, http.StatusMethodNotAllowed, `{"success":false,"message":"method not allowed","data":null}`)
		if got := rec.Header().Get("Allow"); got != http.MethodPost {
			t.Errorf("Allow = %q, want POST", got)
		}
	})

	t.Run("missing content type", func(t *testing.T) {
		handler := setupTestAuthHandler(&mockUserRepo{}, &mockRefreshRepo{})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		handler.Register(rec, req)

		contractAssertResponse(t, rec, http.StatusUnsupportedMediaType, `{"success":false,"message":"Content-Type must be application/json","data":null}`)
	})
}

// contractAuthHandler builds an AuthHandler around any user repository, with
// the same token manager, TTLs and cookie settings as setupTestAuthHandler.
func contractAuthHandler(users repository.UserRepository) *AuthHandler {
	uc := usecase.NewAuthUseCase(users, &mockRefreshRepo{}, &mockTokenMgr{}, 15*time.Minute, 7*24*time.Hour, 15*time.Second)
	return NewAuthHandler(uc, NewCookieManager(false, http.SameSiteLaxMode, 15*time.Minute, 7*24*time.Hour))
}

// ---------------------------------------------------------------------------
// Auth: Login
// ---------------------------------------------------------------------------

func TestAuthContract_Login(t *testing.T) {
	hash := contractBcryptHash(t, "Password123")

	t.Run("success body, cookies and no secrets", func(t *testing.T) {
		uRepo := &mockUserRepo{user: &domain.User{
			ID: 7, Email: "admin@example.com", PasswordHash: hash, Role: "admin",
			CreatedAt: contractCreatedAt, UpdatedAt: contractUpdatedAt,
		}}
		handler := setupTestAuthHandler(uRepo, &mockRefreshRepo{})

		before := time.Now()
		rec := httptest.NewRecorder()
		handler.Login(rec, contractJSONRequest(http.MethodPost, "/api/v1/auth/login", `{"email":"  ADMIN@example.com ","password":"Password123"}`))
		after := time.Now()

		contractAssertResponse(t, rec, http.StatusOK,
			`{"success":true,"message":"Login successful","data":{"id":7,"email":"admin@example.com","role":"admin","created_at":"2026-03-04T05:06:07Z"}}`)
		refresh := contractAssertIssuedCookies(t, rec, before, after)
		contractAssertNoSensitiveData(t, rec.Body.String(), hash, "Password123", "test-jwt-token", refresh)
	})

	t.Run("created_at offset and zero rules", func(t *testing.T) {
		for _, tc := range []struct {
			createdAt time.Time
			want      string
		}{
			{contractCreatedAt.In(contractICT), `"created_at":"2026-03-04T12:06:07+07:00"`},
			{time.Time{}, `"created_at":""`},
		} {
			uRepo := &mockUserRepo{user: &domain.User{ID: 8, Email: "u@example.com", PasswordHash: hash, Role: "user", CreatedAt: tc.createdAt}}
			handler := setupTestAuthHandler(uRepo, &mockRefreshRepo{})
			rec := httptest.NewRecorder()
			handler.Login(rec, contractJSONRequest(http.MethodPost, "/api/v1/auth/login", `{"email":"u@example.com","password":"Password123"}`))

			contractAssertResponse(t, rec, http.StatusOK,
				`{"success":true,"message":"Login successful","data":{"id":8,"email":"u@example.com","role":"user",`+tc.want+`}}`)
		}
	})

	errorCases := []struct {
		name       string
		user       *domain.User
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "wrong password",
			user:       &domain.User{ID: 7, Email: "admin@example.com", PasswordHash: hash, Role: "admin"},
			body:       `{"email":"admin@example.com","password":"WrongPassword"}`,
			wantStatus: http.StatusUnauthorized,
			wantBody:   `{"success":false,"message":"invalid email or password","data":null}`,
		},
		{
			name:       "unknown email",
			body:       `{"email":"nobody@example.com","password":"Password123"}`,
			wantStatus: http.StatusUnauthorized,
			wantBody:   `{"success":false,"message":"invalid email or password","data":null}`,
		},
		{
			name:       "malformed email is reported as invalid credentials",
			body:       `{"email":"nobody","password":"Password123"}`,
			wantStatus: http.StatusUnauthorized,
			wantBody:   `{"success":false,"message":"invalid email or password","data":null}`,
		},
		{
			name:       "invalid JSON",
			body:       `{"email":`,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"success":false,"message":"invalid JSON request","data":null}`,
		},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := setupTestAuthHandler(&mockUserRepo{user: tc.user}, &mockRefreshRepo{})
			rec := httptest.NewRecorder()
			handler.Login(rec, contractJSONRequest(http.MethodPost, "/api/v1/auth/login", tc.body))

			contractAssertResponse(t, rec, tc.wantStatus, tc.wantBody)
			contractAssertNoCookies(t, rec)
		})
	}
}

// ---------------------------------------------------------------------------
// Auth: Refresh
// ---------------------------------------------------------------------------

func TestAuthContract_Refresh(t *testing.T) {
	refreshRequest := func(cookieValue string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		if cookieValue != "" {
			req.AddCookie(&http.Cookie{Name: RefreshTokenCookieName, Value: cookieValue})
		}
		return req
	}

	t.Run("success body, rotated cookies and no secrets", func(t *testing.T) {
		uRepo := &mockUserRepo{user: &domain.User{
			ID: 9, Email: "refresh@example.com", PasswordHash: contractPasswordHashMarker, Role: "user",
			CreatedAt: contractCreatedAt.In(contractICT), UpdatedAt: contractUpdatedAt,
		}}
		rRepo := &mockRefreshRepo{token: &domain.RefreshToken{ID: 1, UserID: 9, ExpiresAt: time.Now().Add(time.Hour)}}
		handler := setupTestAuthHandler(uRepo, rRepo)

		before := time.Now()
		rec := httptest.NewRecorder()
		handler.Refresh(rec, refreshRequest("old-refresh-token-value"))
		after := time.Now()

		contractAssertResponse(t, rec, http.StatusOK,
			`{"success":true,"message":"Token refreshed successfully","data":{"id":9,"email":"refresh@example.com","role":"user","created_at":"2026-03-04T12:06:07+07:00"}}`)
		refresh := contractAssertIssuedCookies(t, rec, before, after)
		if refresh == "old-refresh-token-value" {
			t.Error("refresh token cookie was not rotated")
		}
		contractAssertNoSensitiveData(t, rec.Body.String(), "test-jwt-token", refresh, "old-refresh-token-value")
	})

	t.Run("zero created_at is an empty string", func(t *testing.T) {
		uRepo := &mockUserRepo{user: &domain.User{ID: 9, Email: "refresh@example.com", Role: "user"}}
		rRepo := &mockRefreshRepo{token: &domain.RefreshToken{ID: 1, UserID: 9, ExpiresAt: time.Now().Add(time.Hour)}}
		handler := setupTestAuthHandler(uRepo, rRepo)
		rec := httptest.NewRecorder()
		handler.Refresh(rec, refreshRequest("old-refresh-token-value"))

		contractAssertResponse(t, rec, http.StatusOK,
			`{"success":true,"message":"Token refreshed successfully","data":{"id":9,"email":"refresh@example.com","role":"user","created_at":""}}`)
	})

	t.Run("missing cookie does not touch cookies", func(t *testing.T) {
		handler := setupTestAuthHandler(&mockUserRepo{}, &mockRefreshRepo{})
		rec := httptest.NewRecorder()
		handler.Refresh(rec, refreshRequest(""))

		contractAssertResponse(t, rec, http.StatusUnauthorized, `{"success":false,"message":"refresh token required","data":null}`)
		contractAssertNoCookies(t, rec)
	})

	clearing := []struct {
		name     string
		repo     *mockRefreshRepo
		wantBody string
	}{
		{
			name:     "unknown token",
			repo:     &mockRefreshRepo{},
			wantBody: `{"success":false,"message":"invalid or expired session","data":null}`,
		},
		{
			name:     "expired token",
			repo:     &mockRefreshRepo{token: &domain.RefreshToken{ID: 1, UserID: 9, ExpiresAt: time.Now().Add(-time.Hour)}},
			wantBody: `{"success":false,"message":"invalid or expired refresh token","data":null}`,
		},
	}
	for _, tc := range clearing {
		t.Run(tc.name+" clears cookies", func(t *testing.T) {
			handler := setupTestAuthHandler(&mockUserRepo{}, tc.repo)
			rec := httptest.NewRecorder()
			handler.Refresh(rec, refreshRequest("some-refresh-token"))

			contractAssertResponse(t, rec, http.StatusUnauthorized, tc.wantBody)
			contractAssertClearedCookies(t, rec)
		})
	}

	t.Run("infrastructure error keeps cookies", func(t *testing.T) {
		handler := setupTestAuthHandler(&mockUserRepo{}, &mockRefreshRepo{getErr: errors.New("db down")})
		rec := httptest.NewRecorder()
		handler.Refresh(rec, refreshRequest("some-refresh-token"))

		contractAssertResponse(t, rec, http.StatusInternalServerError, `{"success":false,"message":"Internal server error","data":null}`)
		contractAssertNoCookies(t, rec)
	})
}

// ---------------------------------------------------------------------------
// Auth: Me
// ---------------------------------------------------------------------------

func TestAuthContract_Me(t *testing.T) {
	meRequest := func(user *domain.User) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		if user != nil {
			req = req.WithContext(context.WithValue(req.Context(), authenticatedUserKey, user))
		}
		return req
	}

	tests := []struct {
		name     string
		user     *domain.User
		wantBody string
	}{
		{
			name: "created_at RFC3339 to the second in the value's offset",
			user: &domain.User{
				ID: 5, Email: "me@example.com", PasswordHash: contractPasswordHashMarker, Role: "admin",
				CreatedAt: contractCreatedAt.In(contractICT), UpdatedAt: contractUpdatedAt,
			},
			wantBody: `{"success":true,"message":"User profile retrieved successfully","data":{"id":5,"email":"me@example.com","role":"admin","created_at":"2026-03-04T12:06:07+07:00"}}`,
		},
		{
			name:     "zero created_at is an empty string",
			user:     &domain.User{ID: 6, Email: "zero@example.com", Role: "user"},
			wantBody: `{"success":true,"message":"User profile retrieved successfully","data":{"id":6,"email":"zero@example.com","role":"user","created_at":""}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := setupTestAuthHandler(&mockUserRepo{}, &mockRefreshRepo{})
			rec := httptest.NewRecorder()
			handler.Me(rec, meRequest(tc.user))

			contractAssertResponse(t, rec, http.StatusOK, tc.wantBody)
			contractAssertNoSensitiveData(t, rec.Body.String())
			contractAssertNoCookies(t, rec)
		})
	}

	t.Run("no user in context", func(t *testing.T) {
		handler := setupTestAuthHandler(&mockUserRepo{}, &mockRefreshRepo{})
		rec := httptest.NewRecorder()
		handler.Me(rec, meRequest(nil))

		contractAssertResponse(t, rec, http.StatusUnauthorized, `{"success":false,"message":"authentication required","data":null}`)
	})

	// Through the real middleware the user is rebuilt from the JWT claims only,
	// so created_at is always "" on this endpoint today.
	t.Run("through AuthMiddleware", func(t *testing.T) {
		handler := setupTestAuthHandler(&mockUserRepo{}, &mockRefreshRepo{})
		protected := AuthMiddleware(handler.useCase)(http.HandlerFunc(handler.Me))

		for _, tc := range []struct {
			name       string
			header     string
			wantStatus int
			wantBody   string
		}{
			{"valid token", "Bearer valid-token", http.StatusOK,
				`{"success":true,"message":"User profile retrieved successfully","data":{"id":1,"email":"test@example.com","role":"user","created_at":""}}`},
			{"no token", "", http.StatusUnauthorized, `{"success":false,"message":"authentication required","data":null}`},
			{"invalid token", "Bearer nope", http.StatusUnauthorized, `{"success":false,"message":"invalid or expired token","data":null}`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
				if tc.header != "" {
					req.Header.Set("Authorization", tc.header)
				}
				rec := httptest.NewRecorder()
				protected.ServeHTTP(rec, req)

				contractAssertResponse(t, rec, tc.wantStatus, tc.wantBody)
			})
		}
	})
}

// ---------------------------------------------------------------------------
// Auth: Logout
// ---------------------------------------------------------------------------

func TestAuthContract_Logout(t *testing.T) {
	const wantOK = `{"success":true,"message":"logged out","data":{"message":"logged out"}}`

	tests := []struct {
		name       string
		repo       *mockRefreshRepo
		cookie     string
		wantStatus int
		wantBody   string
	}{
		{"with refresh cookie", &mockRefreshRepo{}, "active-token", http.StatusOK, wantOK},
		{"without cookie", &mockRefreshRepo{}, "", http.StatusOK, wantOK},
		{"already revoked token", &mockRefreshRepo{revokeErr: repository.ErrRefreshTokenRevoked}, "revoked-token", http.StatusOK, wantOK},
		{"infrastructure error", &mockRefreshRepo{revokeErr: errors.New("db down")}, "active-token", http.StatusInternalServerError,
			`{"success":false,"message":"Internal server error","data":null}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := setupTestAuthHandler(&mockUserRepo{}, tc.repo)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: RefreshTokenCookieName, Value: tc.cookie})
			}
			rec := httptest.NewRecorder()
			handler.Logout(rec, req)

			contractAssertResponse(t, rec, tc.wantStatus, tc.wantBody)
			contractAssertClearedCookies(t, rec)
		})
	}

	t.Run("wrong method", func(t *testing.T) {
		handler := setupTestAuthHandler(&mockUserRepo{}, &mockRefreshRepo{})
		rec := httptest.NewRecorder()
		handler.Logout(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/logout", nil))

		contractAssertResponse(t, rec, http.StatusMethodNotAllowed, `{"success":false,"message":"method not allowed","data":null}`)
		contractAssertNoCookies(t, rec)
	})
}

// ---------------------------------------------------------------------------
// Users: List
// ---------------------------------------------------------------------------

func TestUserContract_List(t *testing.T) {
	listing := []domain.User{
		{
			ID: 1, Email: "admin@example.com", PasswordHash: contractPasswordHashMarker, Role: "admin",
			CreatedAt: contractCreatedAt, UpdatedAt: contractUpdatedAt,
		},
		{
			ID: 2, Email: "offset@example.com", PasswordHash: contractPasswordHashMarker, Role: "user",
			CreatedAt: contractCreatedAt.In(contractICT), UpdatedAt: contractUpdatedAt.In(contractICT),
		},
		{ID: 3, Email: "zero@example.com", PasswordHash: contractPasswordHashMarker, Role: "user"},
	}

	tests := []struct {
		name       string
		users      []domain.User
		total      int64
		path       string
		wantQuery  string
		wantPage   int
		wantLimit  int
		wantStatus int
		wantBody   string
	}{
		{
			name:  "items with RFC3339Nano timestamps and pagination",
			users: listing, total: 5,
			path:      "/api/v1/users?page=2&limit=3&q=%20example%20",
			wantQuery: "example", wantPage: 2, wantLimit: 3,
			wantStatus: http.StatusOK,
			wantBody: `{"success":true,"message":"Users retrieved successfully","data":{"items":[` +
				`{"id":1,"email":"admin@example.com","role":"admin","created_at":"2026-03-04T05:06:07.891234567Z","updated_at":"2026-03-05T06:07:08.12Z"},` +
				`{"id":2,"email":"offset@example.com","role":"user","created_at":"2026-03-04T12:06:07.891234567+07:00","updated_at":"2026-03-05T13:07:08.12+07:00"},` +
				`{"id":3,"email":"zero@example.com","role":"user","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}` +
				`],"pagination":{"page":2,"limit":3,"total":5,"total_pages":2}}}`,
		},
		{
			name:  "empty slice uses defaults and returns []",
			users: []domain.User{}, total: 0,
			path:      "/api/v1/users",
			wantQuery: "", wantPage: 1, wantLimit: 10,
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"Users retrieved successfully","data":{"items":[],"pagination":{"page":1,"limit":10,"total":0,"total_pages":0}}}`,
		},
		{
			name:  "nil slice still returns []",
			users: nil, total: 0,
			path:      "/api/v1/users?page=1&limit=100",
			wantQuery: "", wantPage: 1, wantLimit: 100,
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"Users retrieved successfully","data":{"items":[],"pagination":{"page":1,"limit":100,"total":0,"total_pages":0}}}`,
		},
		{name: "non-numeric page", path: "/api/v1/users?page=abc", wantStatus: http.StatusBadRequest,
			wantBody: `{"success":false,"message":"invalid page","data":null}`},
		{name: "page below 1", path: "/api/v1/users?page=0", wantStatus: http.StatusBadRequest,
			wantBody: `{"success":false,"message":"invalid page","data":null}`},
		{name: "non-numeric limit", path: "/api/v1/users?limit=abc", wantStatus: http.StatusBadRequest,
			wantBody: `{"success":false,"message":"invalid limit","data":null}`},
		{name: "limit above 100", path: "/api/v1/users?limit=101", wantStatus: http.StatusBadRequest,
			wantBody: `{"success":false,"message":"invalid limit","data":null}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			repo := &mockUserRepoForHandler{
				listFn: func(ctx context.Context, query string, page, limit int) ([]domain.User, int64, error) {
					called = true
					if query != tc.wantQuery || page != tc.wantPage || limit != tc.wantLimit {
						t.Errorf("repo.List(%q, %d, %d), want (%q, %d, %d)", query, page, limit, tc.wantQuery, tc.wantPage, tc.wantLimit)
					}
					return tc.users, tc.total, nil
				},
			}
			rec := httptest.NewRecorder()
			setupUserHandler(repo).List(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			contractAssertResponse(t, rec, tc.wantStatus, tc.wantBody)
			if tc.wantStatus == http.StatusOK {
				if !called {
					t.Error("repository List was not called")
				}
				contractAssertNoSensitiveData(t, rec.Body.String())
			}
		})
	}

	t.Run("repository failure", func(t *testing.T) {
		repo := &mockUserRepoForHandler{
			listFn: func(ctx context.Context, query string, page, limit int) ([]domain.User, int64, error) {
				return nil, 0, errors.New("db down")
			},
		}
		rec := httptest.NewRecorder()
		setupUserHandler(repo).List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))

		contractAssertResponse(t, rec, http.StatusInternalServerError, `{"success":false,"message":"Internal server error","data":null}`)
	})
}

// ---------------------------------------------------------------------------
// Users: GetByID
// ---------------------------------------------------------------------------

func TestUserContract_GetByID(t *testing.T) {
	tests := []struct {
		name       string
		user       *domain.User
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name: "RFC3339Nano timestamps",
			user: &domain.User{
				ID: 42, Email: "user@example.com", PasswordHash: contractPasswordHashMarker, Role: "user",
				CreatedAt: contractCreatedAt, UpdatedAt: contractUpdatedAt.In(contractICT),
			},
			path:       "/api/v1/users/42",
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"User retrieved successfully","data":{"id":42,"email":"user@example.com","role":"user","created_at":"2026-03-04T05:06:07.891234567Z","updated_at":"2026-03-05T13:07:08.12+07:00"}}`,
		},
		{
			name:       "zero timestamps are 0001-01-01T00:00:00Z, not empty or null",
			user:       &domain.User{ID: 43, Email: "zero@example.com", Role: "admin"},
			path:       "/api/v1/users/43",
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"User retrieved successfully","data":{"id":43,"email":"zero@example.com","role":"admin","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}}`,
		},
		{name: "not found", path: "/api/v1/users/999", wantStatus: http.StatusNotFound,
			wantBody: `{"success":false,"message":"user not found","data":null}`},
		{name: "non-numeric id", path: "/api/v1/users/abc", wantStatus: http.StatusBadRequest,
			wantBody: `{"success":false,"message":"invalid user ID","data":null}`},
		{name: "zero id", path: "/api/v1/users/0", wantStatus: http.StatusBadRequest,
			wantBody: `{"success":false,"message":"invalid user ID","data":null}`},
		{name: "nested path", path: "/api/v1/users/1/extra", wantStatus: http.StatusBadRequest,
			wantBody: `{"success":false,"message":"invalid user ID","data":null}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockUserRepoForHandler{
				getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
					if tc.user == nil {
						return nil, repository.ErrUserNotFound
					}
					u := *tc.user
					return &u, nil
				},
			}
			rec := httptest.NewRecorder()
			setupUserHandler(repo).GetByID(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			contractAssertResponse(t, rec, tc.wantStatus, tc.wantBody)
			if tc.wantStatus == http.StatusOK {
				contractAssertNoSensitiveData(t, rec.Body.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Users: Update
// ---------------------------------------------------------------------------

func TestUserContract_Update(t *testing.T) {
	existing := domain.User{
		ID: 10, Email: "existing@example.com", PasswordHash: contractPasswordHashMarker, Role: "user",
		CreatedAt: contractCreatedAt.In(contractICT), UpdatedAt: contractCreatedAt,
	}

	tests := []struct {
		name       string
		body       string
		updateErr  error
		missing    bool
		wantStatus int
		wantBody   string
	}{
		{
			name:       "email change returns normalized email and repo updated_at",
			body:       `{"email":"  Renamed@Example.COM "}`,
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"User updated successfully","data":{"id":10,"email":"renamed@example.com","role":"user","created_at":"2026-03-04T12:06:07.891234567+07:00","updated_at":"2026-03-05T06:07:08.12Z"}}`,
		},
		{
			name:       "role change",
			body:       `{"role":"admin"}`,
			wantStatus: http.StatusOK,
			wantBody:   `{"success":true,"message":"User updated successfully","data":{"id":10,"email":"existing@example.com","role":"admin","created_at":"2026-03-04T12:06:07.891234567+07:00","updated_at":"2026-03-05T06:07:08.12Z"}}`,
		},
		{name: "empty payload", body: `{}`, wantStatus: http.StatusBadRequest,
			wantBody: `{"success":false,"message":"at least one field must be provided for update","data":null}`},
		{name: "unknown field", body: `{"password":"x"}`, wantStatus: http.StatusBadRequest,
			wantBody: `{"success":false,"message":"invalid request body","data":null}`},
		{name: "invalid email", body: `{"email":"bad"}`, wantStatus: http.StatusBadRequest,
			wantBody: `{"success":false,"message":"invalid email","data":null}`},
		{name: "invalid role", body: `{"role":"superadmin"}`, wantStatus: http.StatusBadRequest,
			wantBody: `{"success":false,"message":"invalid user role","data":null}`},
		{name: "duplicate email", body: `{"email":"taken@example.com"}`, updateErr: repository.ErrEmailExists,
			wantStatus: http.StatusConflict, wantBody: `{"success":false,"message":"email already exists","data":null}`},
		{name: "missing user", body: `{"role":"admin"}`, missing: true,
			wantStatus: http.StatusNotFound, wantBody: `{"success":false,"message":"user not found","data":null}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockUserRepoForHandler{
				getByIDFn: func(ctx context.Context, id int64) (*domain.User, error) {
					if tc.missing {
						return nil, repository.ErrUserNotFound
					}
					u := existing
					return &u, nil
				},
				updateFn: func(ctx context.Context, u *domain.User) error {
					if tc.updateErr != nil {
						return tc.updateErr
					}
					u.UpdatedAt = contractUpdatedAt
					return nil
				},
			}
			rec := httptest.NewRecorder()
			setupUserHandler(repo).Update(rec, contractJSONRequest(http.MethodPatch, "/api/v1/users/10", tc.body))

			contractAssertResponse(t, rec, tc.wantStatus, tc.wantBody)
			if tc.wantStatus == http.StatusOK {
				contractAssertNoSensitiveData(t, rec.Body.String())
			}
		})
	}

	t.Run("invalid id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		setupUserHandler(&mockUserRepoForHandler{}).Update(rec, contractJSONRequest(http.MethodPatch, "/api/v1/users/abc", `{"role":"admin"}`))

		contractAssertResponse(t, rec, http.StatusBadRequest, `{"success":false,"message":"invalid user ID","data":null}`)
	})

	t.Run("missing content type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/10", strings.NewReader(`{"role":"admin"}`))
		rec := httptest.NewRecorder()
		setupUserHandler(&mockUserRepoForHandler{}).Update(rec, req)

		contractAssertResponse(t, rec, http.StatusUnsupportedMediaType, `{"success":false,"message":"Content-Type must be application/json","data":null}`)
	})
}
