package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/handle/http/response"
	"gostock/internal/usecase"

	"golang.org/x/crypto/bcrypt"
)

type mockUserRepo struct {
	user *domain.User
}

func (m *mockUserRepo) Create(ctx context.Context, u *domain.User) error {
	u.ID = 1
	return nil
}
func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if m.user == nil {
		return nil, domain.ErrUserNotFound
	}
	copy := *m.user
	return &copy, nil
}
func (m *mockUserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	if m.user == nil {
		return nil, domain.ErrUserNotFound
	}
	copy := *m.user
	return &copy, nil
}
func (m *mockUserRepo) List(ctx context.Context, query string, page, limit int) ([]domain.User, int64, error) {
	if m.user != nil {
		return []domain.User{*m.user}, 1, nil
	}
	return []domain.User{}, 0, nil
}
func (m *mockUserRepo) Update(ctx context.Context, u *domain.User) error {
	m.user = u
	return nil
}
func (m *mockUserRepo) UpdateAndRevokeTokens(ctx context.Context, u *domain.User, revokedAt time.Time) error {
	m.user = u
	return nil
}

type mockRefreshRepo struct {
	token         *domain.RefreshToken
	tokensByHash  map[string]*domain.RefreshToken
	getErr        error
	rotateErr     error
	revokeErr     error
	revokeCall    int
	revokeAllCall int
}

func (m *mockRefreshRepo) Create(ctx context.Context, t *domain.RefreshToken) error {
	t.ID = 1
	if m.tokensByHash != nil {
		copy := *t
		m.tokensByHash[t.TokenHash] = &copy
	}
	return nil
}
func (m *mockRefreshRepo) GetByTokenHash(ctx context.Context, h string) (*domain.RefreshToken, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.tokensByHash != nil {
		t, ok := m.tokensByHash[h]
		if !ok || t == nil {
			return nil, domain.ErrRefreshTokenNotFound
		}
		copy := *t
		return &copy, nil
	}
	if m.token == nil {
		return nil, domain.ErrRefreshTokenNotFound
	}
	copy := *m.token
	return &copy, nil
}
func (m *mockRefreshRepo) Rotate(ctx context.Context, old string, newT *domain.RefreshToken) error {
	if m.rotateErr != nil {
		return m.rotateErr
	}
	newT.ID = 2
	if m.tokensByHash != nil {
		oldT, ok := m.tokensByHash[old]
		if ok && oldT != nil {
			now := time.Now().UTC()
			oldT.RevokedAt = &now
			oldT.ReplacedByHash = &newT.TokenHash
		}
		copy := *newT
		m.tokensByHash[newT.TokenHash] = &copy
	}
	return nil
}
func (m *mockRefreshRepo) Revoke(ctx context.Context, h string, at time.Time) error {
	m.revokeCall++
	if m.tokensByHash != nil {
		if t, ok := m.tokensByHash[h]; ok && t != nil {
			t.RevokedAt = &at
		}
	}
	return m.revokeErr
}
func (m *mockRefreshRepo) RevokeAllForUser(ctx context.Context, u int64, at time.Time) error {
	m.revokeAllCall++
	if m.tokensByHash != nil {
		for _, t := range m.tokensByHash {
			if t.UserID == u {
				t.RevokedAt = &at
			}
		}
	}
	return nil
}

type mockTokenMgr struct {
	token string
}

func (m *mockTokenMgr) GenerateAccessToken(userID int64, email, role string) (string, time.Time, error) {
	t := m.token
	if t == "" {
		t = "test-jwt-token"
	}
	return t, time.Now().Add(15 * time.Minute), nil
}
func (m *mockTokenMgr) ValidateAccessToken(tokenStr string) (*usecase.TokenClaims, error) {
	if tokenStr == "valid-token" || tokenStr == "test-jwt-token" {
		return &usecase.TokenClaims{UserID: 1, Email: "test@example.com", Role: "user"}, nil
	}
	return nil, errors.New("invalid token")
}

func setupTestAuthHandler(uRepo *mockUserRepo, rRepo *mockRefreshRepo) *AuthHandler {
	tm := &mockTokenMgr{}
	uc := usecase.NewAuthUseCase(uRepo, rRepo, tm, 15*time.Minute, 7*24*time.Hour, 15*time.Second)
	cm := NewCookieManager(false, http.SameSiteLaxMode, 15*time.Minute, 7*24*time.Hour)
	return NewAuthHandler(uc, cm)
}

func hasClearCookies(cookies []*http.Cookie) (accessCleared, refreshCleared bool) {
	for _, c := range cookies {
		if c.Name == AccessTokenCookieName && c.MaxAge == -1 && c.Path == "/" {
			accessCleared = true
		}
		if c.Name == RefreshTokenCookieName && c.MaxAge == -1 && c.Path == "/api/v1/auth" {
			refreshCleared = true
		}
	}
	return accessCleared, refreshCleared
}

func TestAuthHandler_LoginSuccessAndCookies(t *testing.T) {
	hashedPass, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	uRepo := &mockUserRepo{
		user: &domain.User{ID: 1, Email: "user@example.com", PasswordHash: string(hashedPass), Role: "user"},
	}
	rRepo := &mockRefreshRepo{}
	handler := setupTestAuthHandler(uRepo, rRepo)

	reqBody := `{"email":"user@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader([]byte(reqBody)))
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Verify Cookies
	cookies := rec.Result().Cookies()
	var accessCookie, refreshCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == AccessTokenCookieName {
			accessCookie = c
		}
		if c.Name == RefreshTokenCookieName {
			refreshCookie = c
		}
	}

	if accessCookie == nil {
		t.Fatal("missing access_token cookie")
	}
	if accessCookie.Path != "/" {
		t.Errorf("access_token Path = %q, want '/'", accessCookie.Path)
	}
	if !accessCookie.HttpOnly {
		t.Error("access_token must be HttpOnly")
	}

	if refreshCookie == nil {
		t.Fatal("missing refresh_token cookie")
	}
	if refreshCookie.Path != "/api/v1/auth" {
		t.Errorf("refresh_token Path = %q, want '/api/v1/auth'", refreshCookie.Path)
	}
	if !refreshCookie.HttpOnly {
		t.Error("refresh_token must be HttpOnly")
	}

	// Verify response body envelope (must NOT contain tokens)
	var resp response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if !resp.Success {
		t.Error("expected resp.Success = true")
	}
	dataMap, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatal("expected data to be object")
	}
	if _, hasToken := dataMap["token"]; hasToken {
		t.Error("response JSON body must NOT expose token")
	}
	if _, hasAccessToken := dataMap["access_token"]; hasAccessToken {
		t.Error("response JSON body must NOT expose access_token")
	}
}

func TestAuthHandler_LoginWrongCredentials(t *testing.T) {
	hashedPass, _ := bcrypt.GenerateFromPassword([]byte("correctpass"), bcrypt.DefaultCost)
	uRepo := &mockUserRepo{
		user: &domain.User{ID: 1, Email: "user@example.com", PasswordHash: string(hashedPass), Role: "user"},
	}
	handler := setupTestAuthHandler(uRepo, &mockRefreshRepo{})

	reqBody := `{"email":"user@example.com","password":"wrongpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader([]byte(reqBody)))
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthHandler_RefreshSuccess(t *testing.T) {
	uRepo := &mockUserRepo{
		user: &domain.User{ID: 1, Email: "user@example.com", Role: "user"},
	}
	rRepo := &mockRefreshRepo{
		token: &domain.RefreshToken{ID: 1, UserID: 1, ExpiresAt: time.Now().Add(24 * time.Hour)},
	}
	handler := setupTestAuthHandler(uRepo, rRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{
		Name:  RefreshTokenCookieName,
		Value: "valid-refresh-token",
		Path:  "/api/v1/auth",
	})
	rec := httptest.NewRecorder()

	handler.Refresh(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cookies := rec.Result().Cookies()
	var accessCookie, refreshCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == AccessTokenCookieName {
			accessCookie = c
		}
		if c.Name == RefreshTokenCookieName {
			refreshCookie = c
		}
	}
	if accessCookie == nil || refreshCookie == nil {
		t.Fatal("expected both updated cookies after refresh")
	}
}

func TestAuthHandler_RefreshMissingCookie(t *testing.T) {
	handler := setupTestAuthHandler(&mockUserRepo{}, &mockRefreshRepo{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	rec := httptest.NewRecorder()

	handler.Refresh(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// Verify no clear cookies were sent since there was no cookie
	accClr, refClr := hasClearCookies(rec.Result().Cookies())
	if accClr || refClr {
		t.Error("missing cookie should not set clear cookie headers")
	}
}

func TestAuthHandler_RefreshExpired(t *testing.T) {
	now := time.Now()
	rRepo := &mockRefreshRepo{
		token: &domain.RefreshToken{ID: 1, UserID: 1, ExpiresAt: now.Add(-time.Hour)},
	}
	handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{
		Name:  RefreshTokenCookieName,
		Value: "expired-refresh-token",
	})
	rec := httptest.NewRecorder()

	handler.Refresh(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// Verify authentication failure clears cookies
	accClr, refClr := hasClearCookies(rec.Result().Cookies())
	if !accClr || !refClr {
		t.Error("expired token must clear both access_token and refresh_token cookies")
	}
}

func TestAuthHandler_RefreshRevoked(t *testing.T) {
	now := time.Now()
	rRepo := &mockRefreshRepo{
		token: &domain.RefreshToken{ID: 1, UserID: 1, ExpiresAt: now.Add(time.Hour), RevokedAt: &now},
	}
	handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{
		Name:  RefreshTokenCookieName,
		Value: "revoked-refresh-token",
	})
	rec := httptest.NewRecorder()

	handler.Refresh(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// Verify authentication failure clears cookies
	accClr, refClr := hasClearCookies(rec.Result().Cookies())
	if !accClr || !refClr {
		t.Error("revoked token must clear both access_token and refresh_token cookies")
	}
}

func TestAuthHandler_RefreshReused(t *testing.T) {
	replaced := "replaced-hash"
	rRepo := &mockRefreshRepo{
		token: &domain.RefreshToken{ID: 1, UserID: 1, ExpiresAt: time.Now().Add(time.Hour), ReplacedByHash: &replaced},
	}
	handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{
		Name:  RefreshTokenCookieName,
		Value: "reused-refresh-token",
	})
	rec := httptest.NewRecorder()

	handler.Refresh(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// Verify authentication failure clears cookies
	accClr, refClr := hasClearCookies(rec.Result().Cookies())
	if !accClr || !refClr {
		t.Error("reused token must clear both access_token and refresh_token cookies")
	}
}

func TestAuthHandler_RefreshInvalidRefreshTokenCredential(t *testing.T) {
	rRepo := &mockRefreshRepo{
		token:     &domain.RefreshToken{ID: 1, UserID: 1, ExpiresAt: time.Now().Add(time.Hour)},
		rotateErr: domain.ErrInvalidRefreshToken,
	}
	handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{
		Name:  RefreshTokenCookieName,
		Value: "malformed-refresh-token",
	})
	rec := httptest.NewRecorder()

	handler.Refresh(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// Verify invalid refresh credential failure clears cookies
	accClr, refClr := hasClearCookies(rec.Result().Cookies())
	if !accClr || !refClr {
		t.Error("invalid refresh token credential must clear both access_token and refresh_token cookies")
	}
}

func TestAuthHandler_RefreshConcurrent_WithinGracePeriod(t *testing.T) {
	replaced := "replaced-hash"
	rotatedAt := time.Now().Add(-2 * time.Second)
	rRepo := &mockRefreshRepo{
		token: &domain.RefreshToken{
			ID:             1,
			UserID:         1,
			ExpiresAt:      time.Now().Add(time.Hour),
			ReplacedByHash: &replaced,
			RevokedAt:      &rotatedAt,
		},
	}
	handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{
		Name:  RefreshTokenCookieName,
		Value: "concurrent-refresh-token",
	})
	rec := httptest.NewRecorder()

	handler.Refresh(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}

	// Verify concurrent refresh MUST NOT clear cookies so fresh cookies on client are preserved
	accClr, refClr := hasClearCookies(rec.Result().Cookies())
	if accClr || refClr {
		t.Error("concurrent refresh within grace period must NOT clear cookies")
	}
}

func TestAuthHandler_RefreshDatabaseError(t *testing.T) {
	rRepo := &mockRefreshRepo{
		getErr: errors.New("database connection refused"),
	}
	handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{
		Name:  RefreshTokenCookieName,
		Value: "valid-refresh-token",
	})
	rec := httptest.NewRecorder()

	handler.Refresh(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (database error should return 500)", rec.Code, http.StatusInternalServerError)
	}

	// CR-01: Critical check - Database or infrastructure error MUST NOT clear client cookies!
	accClr, refClr := hasClearCookies(rec.Result().Cookies())
	if accClr || refClr {
		t.Fatal("database/internal error in Refresh must NOT clear authentication cookies")
	}

	// Envelope must be unified error response without leaking internal sql/db messages
	var resp response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode JSON response error: %v", err)
	}
	if resp.Success != false {
		t.Errorf("resp.Success = %v, want false", resp.Success)
	}
	if !strings.EqualFold(resp.Message, "internal server error") {
		t.Errorf("resp.Message = %q, want 'internal server error' (must not leak internal error)", resp.Message)
	}
}

func TestAuthHandler_LogoutSuccess(t *testing.T) {
	rRepo := &mockRefreshRepo{}
	handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{
		Name:  RefreshTokenCookieName,
		Value: "active-token",
	})
	rec := httptest.NewRecorder()

	handler.Logout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rRepo.revokeCall != 1 {
		t.Errorf("revokeCall = %d, want 1", rRepo.revokeCall)
	}

	accClr, refClr := hasClearCookies(rec.Result().Cookies())
	if !accClr || !refClr {
		t.Error("logout must clear both access_token and refresh_token cookies")
	}
}

func TestAuthHandler_LogoutNoCookie(t *testing.T) {
	rRepo := &mockRefreshRepo{}
	handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rec := httptest.NewRecorder()

	handler.Logout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (logout without cookie must be idempotent 200 OK)", rec.Code, http.StatusOK)
	}
	if rRepo.revokeCall != 0 {
		t.Errorf("revokeCall = %d, want 0 when no cookie provided", rRepo.revokeCall)
	}

	accClr, refClr := hasClearCookies(rec.Result().Cookies())
	if !accClr || !refClr {
		t.Error("logout must clear cookies even if no cookie was sent")
	}
}

func TestAuthHandler_LogoutTokenNotFound(t *testing.T) {
	rRepo := &mockRefreshRepo{
		revokeErr: domain.ErrRefreshTokenNotFound,
	}
	handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{
		Name:  RefreshTokenCookieName,
		Value: "non-existent-token",
	})
	rec := httptest.NewRecorder()

	handler.Logout(rec, req)

	// Idempotent: token not found is considered a successful logout
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (idempotent logout)", rec.Code, http.StatusOK)
	}

	accClr, refClr := hasClearCookies(rec.Result().Cookies())
	if !accClr || !refClr {
		t.Error("logout must clear cookies even if token was not found in DB")
	}
}

func TestAuthHandler_LogoutTokenAlreadyRevoked(t *testing.T) {
	rRepo := &mockRefreshRepo{
		revokeErr: domain.ErrRefreshTokenRevoked,
	}
	handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{
		Name:  RefreshTokenCookieName,
		Value: "already-revoked-token",
	})
	rec := httptest.NewRecorder()

	handler.Logout(rec, req)

	// Idempotent: already revoked is considered a successful logout
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	accClr, refClr := hasClearCookies(rec.Result().Cookies())
	if !accClr || !refClr {
		t.Error("logout must clear cookies even if token was already revoked")
	}
}

func TestAuthHandler_LogoutDatabaseError(t *testing.T) {
	rRepo := &mockRefreshRepo{
		revokeErr: errors.New("database connection lost"),
	}
	handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{
		Name:  RefreshTokenCookieName,
		Value: "valid-token",
	})
	rec := httptest.NewRecorder()

	handler.Logout(rec, req)

	// CR-02: Infrastructure / database error during logout must return 500
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d on DB failure during revoke", rec.Code, http.StatusInternalServerError)
	}

	// CR-02: Must STILL clear cookies on client side so browser session ends
	accClr, refClr := hasClearCookies(rec.Result().Cookies())
	if !accClr || !refClr {
		t.Fatal("logout must clear client cookies even when database revoke fails with 500")
	}

	// Ensure error response envelope without internal details
	var resp response.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode JSON response error: %v", err)
	}
	if resp.Success != false {
		t.Errorf("resp.Success = %v, want false", resp.Success)
	}
	if !strings.EqualFold(resp.Message, "internal server error") {
		t.Errorf("resp.Message = %q, want 'internal server error'", resp.Message)
	}
}

func TestAuthHandler_Me(t *testing.T) {
	handler := setupTestAuthHandler(&mockUserRepo{}, &mockRefreshRepo{})

	t.Run("unauthenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		rec := httptest.NewRecorder()

		handler.Me(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("authenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		user := &domain.User{ID: 5, Email: "viet@example.com", Role: "admin"}
		ctx := context.WithValue(req.Context(), authenticatedUserKey, user)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		handler.Me(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var resp response.Response
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		data := resp.Data.(map[string]any)
		if data["email"] != "viet@example.com" {
			t.Errorf("email = %v, want viet@example.com", data["email"])
		}
		if data["role"] != "admin" {
			t.Errorf("role = %v, want admin", data["role"])
		}
	})
}

func TestAuthHandler_Logout(t *testing.T) {
	t.Run("method not allowed for GET", func(t *testing.T) {
		handler := setupTestAuthHandler(&mockUserRepo{}, &mockRefreshRepo{})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/logout", nil)
		rec := httptest.NewRecorder()

		handler.Logout(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("successful logout with valid cookie", func(t *testing.T) {
		rRepo := &mockRefreshRepo{}
		handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.AddCookie(&http.Cookie{
			Name:  RefreshTokenCookieName,
			Value: "valid-refresh-token",
		})
		rec := httptest.NewRecorder()

		handler.Logout(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if rRepo.revokeCall != 1 {
			t.Errorf("revokeCall = %d, want 1", rRepo.revokeCall)
		}
		accessCleared, refreshCleared := hasClearCookies(rec.Result().Cookies())
		if !accessCleared || !refreshCleared {
			t.Errorf("expected both cookies cleared, got access=%v, refresh=%v", accessCleared, refreshCleared)
		}
	})

	t.Run("logout without cookie is idempotent and clears cookies", func(t *testing.T) {
		rRepo := &mockRefreshRepo{}
		handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		rec := httptest.NewRecorder()

		handler.Logout(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if rRepo.revokeCall != 0 {
			t.Errorf("revokeCall = %d, want 0 when no cookie present", rRepo.revokeCall)
		}
		accessCleared, refreshCleared := hasClearCookies(rec.Result().Cookies())
		if !accessCleared || !refreshCleared {
			t.Errorf("expected cookies cleared, got access=%v, refresh=%v", accessCleared, refreshCleared)
		}
	})

	idempotentErrors := []struct {
		name string
		err  error
	}{
		{"InvalidSession", usecase.ErrInvalidSession},
		{"RefreshTokenNotFound", domain.ErrRefreshTokenNotFound},
		{"RefreshTokenRevoked", domain.ErrRefreshTokenRevoked},
		{"RefreshTokenExpired", domain.ErrRefreshTokenExpired},
		{"InvalidRefreshToken", domain.ErrInvalidRefreshToken},
		{"RefreshTokenReused", domain.ErrRefreshTokenReused},
	}

	for _, tc := range idempotentErrors {
		t.Run("idempotent logout on "+tc.name, func(t *testing.T) {
			rRepo := &mockRefreshRepo{revokeErr: tc.err}
			handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
			req.AddCookie(&http.Cookie{
				Name:  RefreshTokenCookieName,
				Value: "expired-or-invalid-token",
			})
			rec := httptest.NewRecorder()

			handler.Logout(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d for %s", rec.Code, http.StatusOK, tc.name)
			}
			var resp response.Response
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if !resp.Success {
				t.Errorf("expected success = true for %s", tc.name)
			}
			accessCleared, refreshCleared := hasClearCookies(rec.Result().Cookies())
			if !accessCleared || !refreshCleared {
				t.Errorf("expected cookies cleared for %s", tc.name)
			}
		})
	}

	t.Run("infrastructure failure returns 500 but still clears cookies", func(t *testing.T) {
		rRepo := &mockRefreshRepo{revokeErr: errors.New("pq: database connection failure")}
		handler := setupTestAuthHandler(&mockUserRepo{}, rRepo)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.AddCookie(&http.Cookie{
			Name:  RefreshTokenCookieName,
			Value: "valid-token",
		})
		rec := httptest.NewRecorder()

		handler.Logout(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
		var resp response.Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Success {
			t.Error("expected success = false")
		}
		if !strings.EqualFold(resp.Message, "internal server error") {
			t.Errorf("Message = %q, want 'Internal server error'", resp.Message)
		}
		accessCleared, refreshCleared := hasClearCookies(rec.Result().Cookies())
		if !accessCleared || !refreshCleared {
			t.Error("expected cookies cleared even on infrastructure failure")
		}
	})
}
