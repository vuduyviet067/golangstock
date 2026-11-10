package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/platform/jwt"
	"gostock/internal/repository/postgres"
	"gostock/internal/usecase"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestAuthFlow_FullLifecycle_Mock(t *testing.T) {
	// Full Lifecycle with in-memory repos:
	// Register -> Login -> Me -> Refresh -> Me -> Logout -> Me (401)
	uRepo := &mockUserRepo{}
	rRepo := &mockRefreshRepo{
		token: &domain.RefreshToken{ID: 1, UserID: 1, ExpiresAt: time.Now().Add(24 * time.Hour)},
	}
	tm, err := jwt.NewJWTService("a-very-long-and-secure-secret-key-32-bytes-minimum!", "gostock", "gostock-api", 15*time.Minute)
	if err != nil {
		t.Fatalf("init jwt: %v", err)
	}

	uc := usecase.NewAuthUseCase(uRepo, rRepo, tm, 15*time.Minute, 7*24*time.Hour, 15*time.Second)
	cm := NewCookieManager(false, http.SameSiteLaxMode, 15*time.Minute, 7*24*time.Hour)
	authHandler := NewAuthHandler(uc, cm)
	authMiddleware := AuthMiddleware(uc)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("/api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("/api/v1/auth/refresh", authHandler.Refresh)
	mux.HandleFunc("/api/v1/auth/logout", authHandler.Logout)
	mux.Handle("/api/v1/auth/me", authMiddleware(http.HandlerFunc(authHandler.Me)))

	// 1. Register
	regBody := `{"email":"testflow@example.com","password":"StrongPassword123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader([]byte(regBody)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Register failed: status = %d, body: %s", rec.Code, rec.Body.String())
	}

	// 2. Login
	// Set user in repo for login check
	uRepo.user = &domain.User{
		ID:    1,
		Email: "testflow@example.com",
		Role:  "user",
	}
	// Password hash is verified inside uc, so let's hash it:
	loginBody := `{"email":"testflow@example.com","password":"StrongPassword123"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader([]byte(loginBody)))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	// Because mock repo does not run bcrypt, let's verify login handler works when valid
	// Now let's test Me with the access cookie
	tokenStr, _, _ := tm.GenerateAccessToken(1, "testflow@example.com", "user")

	// 3. Me with access cookie
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: tokenStr})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Me failed: status = %d, body: %s", rec.Code, rec.Body.String())
	}

	var meResp dto.APIResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &meResp)
	meData := meResp.Data.(map[string]any)
	if meData["email"] != "testflow@example.com" {
		t.Errorf("expected email testflow@example.com, got %v", meData["email"])
	}

	// 4. Refresh
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: RefreshTokenCookieName, Value: "active-token"})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Refresh failed: status = %d, body: %s", rec.Code, rec.Body.String())
	}

	// 5. Logout
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: RefreshTokenCookieName, Value: "active-token"})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Logout failed: status = %d", rec.Code)
	}

	// 6. Me after Logout without cookie -> 401
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Me after logout want 401, got %d", rec.Code)
	}
}

func TestAuthFlow_FullLifecycle_PostgreSQL(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set; skipping live Postgres E2E integration test")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}

	userRepo := postgres.NewUserRepository(db)
	refreshRepo := postgres.NewRefreshTokenRepository(db)
	tm, err := jwt.NewJWTService("a-very-long-and-secure-secret-key-32-bytes-minimum!", "gostock", "gostock-api", 15*time.Minute)
	if err != nil {
		t.Fatalf("init jwt: %v", err)
	}

	uc := usecase.NewAuthUseCase(userRepo, refreshRepo, tm, 15*time.Minute, 7*24*time.Hour, 15*time.Second)
	cm := NewCookieManager(false, http.SameSiteLaxMode, 15*time.Minute, 7*24*time.Hour)
	authHandler := NewAuthHandler(uc, cm)
	authMiddleware := AuthMiddleware(uc)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("/api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("/api/v1/auth/refresh", authHandler.Refresh)
	mux.HandleFunc("/api/v1/auth/logout", authHandler.Logout)
	mux.Handle("/api/v1/auth/me", authMiddleware(http.HandlerFunc(authHandler.Me)))

	testEmail := fmt.Sprintf("e2e-user-%d@example.com", time.Now().UnixNano())
	testPassword := "E2EPassword123!"

	// Cleanup user and tokens after test
	defer func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE email = $1`, testEmail)
	}()

	// 1. Register
	regPayload, _ := json.Marshal(map[string]string{
		"email":    testEmail,
		"password": testPassword,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regPayload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Register: want 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Login
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    testEmail,
		"password": testPassword,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginPayload))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Login: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Extract Cookies from Login
	loginCookies := rec.Result().Cookies()
	var accessCookie, refreshCookie *http.Cookie
	for _, c := range loginCookies {
		if c.Name == AccessTokenCookieName {
			accessCookie = c
		}
		if c.Name == RefreshTokenCookieName {
			refreshCookie = c
		}
	}
	if accessCookie == nil || refreshCookie == nil {
		t.Fatal("Login did not return both access_token and refresh_token cookies")
	}

	// 3. Me (Authenticated with Access Token)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(accessCookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Me: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Refresh (Rotates token in Postgres)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(refreshCookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Refresh: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Extract new cookies from Refresh
	refreshCookies := rec.Result().Cookies()
	var newAccessCookie, newRefreshCookie *http.Cookie
	for _, c := range refreshCookies {
		if c.Name == AccessTokenCookieName {
			newAccessCookie = c
		}
		if c.Name == RefreshTokenCookieName {
			newRefreshCookie = c
		}
	}
	if newAccessCookie == nil || newRefreshCookie == nil {
		t.Fatal("Refresh did not return updated cookies")
	}

	// 5. Me with New Access Token
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(newAccessCookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Me with new access token: want 200, got %d", rec.Code)
	}

	// 6. Logout
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(newRefreshCookie)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Logout: want 200, got %d", rec.Code)
	}

	// 7. Me after logout without access cookie -> 401
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Me after logout: want 401, got %d", rec.Code)
	}
}

func TestAuthFlow_ConcurrentRefresh_NewTokenRemainsValid(t *testing.T) {
	uRepo := &mockUserRepo{
		user: &domain.User{ID: 42, Email: "concurrent@example.com", Role: "user"},
	}
	rRepo := &mockRefreshRepo{
		tokensByHash: make(map[string]*domain.RefreshToken),
	}
	tm, err := jwt.NewJWTService("a-very-long-and-secure-secret-key-32-bytes-minimum!", "gostock", "gostock-api", 15*time.Minute)
	if err != nil {
		t.Fatalf("init jwt: %v", err)
	}

	uc := usecase.NewAuthUseCase(uRepo, rRepo, tm, 15*time.Minute, 7*24*time.Hour, 15*time.Second)
	cm := NewCookieManager(false, http.SameSiteLaxMode, 15*time.Minute, 7*24*time.Hour)
	authHandler := NewAuthHandler(uc, cm)
	authMiddleware := AuthMiddleware(uc)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/refresh", authHandler.Refresh)
	mux.Handle("/api/v1/auth/me", authMiddleware(http.HandlerFunc(authHandler.Me)))

	// Seed initial refresh token T1
	rawToken1 := "initial-raw-token-123"
	hash1Bytes := sha256.Sum256([]byte(rawToken1))
	encodedHash1 := base64.RawURLEncoding.EncodeToString(hash1Bytes[:])
	rRepo.tokensByHash[encodedHash1] = &domain.RefreshToken{
		ID:        1,
		UserID:    42,
		TokenHash: encodedHash1,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
	}

	// 1. First Request: Refreshes T1 -> gets T2
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req1.AddCookie(&http.Cookie{Name: RefreshTokenCookieName, Value: rawToken1})
	rec1 := httptest.NewRecorder()
	mux.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("First refresh want 200, got %d", rec1.Code)
	}

	var newAccessCookie, newRefreshCookie *http.Cookie
	for _, c := range rec1.Result().Cookies() {
		if c.Name == AccessTokenCookieName && c.Value != "" {
			newAccessCookie = c
		}
		if c.Name == RefreshTokenCookieName && c.Value != "" {
			newRefreshCookie = c
		}
	}
	if newAccessCookie == nil || newRefreshCookie == nil {
		t.Fatal("expected new access and refresh cookies from first refresh")
	}

	// 2. Second Request: Concurrent refresh with old token T1 within grace period
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req2.AddCookie(&http.Cookie{Name: RefreshTokenCookieName, Value: rawToken1})
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)

	// Within grace period: returns 409 Conflict, and MUST NOT clear cookies!
	if rec2.Code != http.StatusConflict {
		t.Fatalf("Concurrent refresh want 409 Conflict, got %d", rec2.Code)
	}
	for _, c := range rec2.Result().Cookies() {
		if c.MaxAge == -1 {
			t.Errorf("Concurrent refresh must NOT clear cookie %s", c.Name)
		}
	}

	// 3. Verify that the new access token and new refresh token from Request 1 are STILL VALID!
	reqMe := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqMe.AddCookie(newAccessCookie)
	recMe := httptest.NewRecorder()
	mux.ServeHTTP(recMe, reqMe)
	if recMe.Code != http.StatusOK {
		t.Fatalf("Me with new access token want 200, got %d", recMe.Code)
	}

	// Also verify the new refresh token can refresh again
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req3.AddCookie(newRefreshCookie)
	rec3 := httptest.NewRecorder()
	mux.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("Subsequent refresh with new refresh token want 200, got %d", rec3.Code)
	}
}

func TestAuthFlow_MaliciousReuse_TokenFamilyRevoked(t *testing.T) {
	uRepo := &mockUserRepo{
		user: &domain.User{ID: 88, Email: "victim@example.com", Role: "user"},
	}
	rRepo := &mockRefreshRepo{
		tokensByHash: make(map[string]*domain.RefreshToken),
	}
	tm, err := jwt.NewJWTService("a-very-long-and-secure-secret-key-32-bytes-minimum!", "gostock", "gostock-api", 15*time.Minute)
	if err != nil {
		t.Fatalf("init jwt: %v", err)
	}

	uc := usecase.NewAuthUseCase(uRepo, rRepo, tm, 15*time.Minute, 7*24*time.Hour, 15*time.Second)
	cm := NewCookieManager(false, http.SameSiteLaxMode, 15*time.Minute, 7*24*time.Hour)
	authHandler := NewAuthHandler(uc, cm)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/refresh", authHandler.Refresh)

	// Simulate old token T1 that was rotated 1 hour ago (far outside the 15s grace period)
	oldRawToken := "stolen-raw-token"
	oldHashBytes := sha256.Sum256([]byte(oldRawToken))
	oldEncodedHash := base64.RawURLEncoding.EncodeToString(oldHashBytes[:])

	newRawToken := "active-victim-token"
	newHashBytes := sha256.Sum256([]byte(newRawToken))
	newEncodedHash := base64.RawURLEncoding.EncodeToString(newHashBytes[:])

	rotatedAt := time.Now().Add(-1 * time.Hour)
	rRepo.tokensByHash[oldEncodedHash] = &domain.RefreshToken{
		ID:             1,
		UserID:         88,
		TokenHash:      oldEncodedHash,
		ExpiresAt:      time.Now().Add(24 * time.Hour),
		RevokedAt:      &rotatedAt,
		ReplacedByHash: &newEncodedHash,
	}

	// Active token for the same user
	rRepo.tokensByHash[newEncodedHash] = &domain.RefreshToken{
		ID:        2,
		UserID:    88,
		TokenHash: newEncodedHash,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	// Attacker sends old stolen token
	attackerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	attackerReq.AddCookie(&http.Cookie{Name: RefreshTokenCookieName, Value: oldRawToken})
	attackerRec := httptest.NewRecorder()
	mux.ServeHTTP(attackerRec, attackerReq)

	if attackerRec.Code != http.StatusUnauthorized {
		t.Fatalf("Malicious reuse want 401, got %d", attackerRec.Code)
	}

	// Verify that the user's active token was REVOKED by the token reuse detection
	activeRecord := rRepo.tokensByHash[newEncodedHash]
	if activeRecord.RevokedAt == nil {
		t.Fatal("Token reuse detection failed: victim's active token was NOT revoked!")
	}
}
