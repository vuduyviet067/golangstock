package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"gostock/internal/domain"
	"gostock/internal/dto"
	productHTTP "gostock/internal/handle/http"
	"gostock/internal/platform/database"
	"gostock/internal/repository"
	"gostock/internal/testdb"
	"gostock/internal/usecase"
)

type fakeProductRepo struct{}

func (f *fakeProductRepo) GetByID(ctx context.Context, id int64) (*domain.Product, error) {
	return &domain.Product{ID: id, SKU: "SKU-001", Name: "Test Product", Price: 50, Quantity: 20}, nil
}

func (f *fakeProductRepo) Create(ctx context.Context, p *domain.Product, actorID int64) error {
	p.ID = 1
	return nil
}

func (f *fakeProductRepo) Update(ctx context.Context, p *domain.Product, minimumStock *int) error {
	return nil
}

func (f *fakeProductRepo) Delete(ctx context.Context, id int64) error {
	return nil
}

func (f *fakeProductRepo) List(ctx context.Context, query string, page, limit int, sortBy, order string) ([]domain.Product, int64, error) {
	return []domain.Product{
		{ID: 1, SKU: "SKU-001", Name: "Test Product", Price: 50, Quantity: 20},
	}, 1, nil
}

func (f *fakeProductRepo) StockIn(ctx context.Context, id int64, quantity int, actorID int64) (*domain.Product, error) {
	return &domain.Product{ID: id, SKU: "SKU-001", Name: "Test Product", Price: 50, Quantity: 20 + quantity}, nil
}

func (f *fakeProductRepo) StockOut(ctx context.Context, id int64, quantity int, actorID int64) (*domain.Product, error) {
	return &domain.Product{ID: id, SKU: "SKU-001", Name: "Test Product", Price: 50, Quantity: 20 - quantity}, nil
}

type fakeAuth struct{}

func (f *fakeAuth) Authenticate(ctx context.Context, token string) (*domain.User, error) {
	switch token {
	case "admin-token":
		return &domain.User{ID: 1, Email: "admin@example.com", Role: "admin"}, nil
	case "user-token":
		return &domain.User{ID: 2, Email: "user@example.com", Role: "user"}, nil
	default:
		return nil, errors.New("invalid or expired session")
	}
}

type fakeUserRepo struct{}

func (f *fakeUserRepo) Create(ctx context.Context, user *domain.User) error { return nil }
func (f *fakeUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return nil, repository.ErrUserNotFound
}
func (f *fakeUserRepo) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	if id == 1 {
		return &domain.User{ID: 1, Email: "target@example.com", Role: "user"}, nil
	}
	return nil, repository.ErrUserNotFound
}
func (f *fakeUserRepo) List(ctx context.Context, query string, page, limit int) ([]domain.User, int64, error) {
	return []domain.User{}, 0, nil
}
func (f *fakeUserRepo) Update(ctx context.Context, user *domain.User) error {
	return nil
}
func (f *fakeUserRepo) UpdateAndRevokeTokens(ctx context.Context, user *domain.User, revokedAt time.Time) error {
	return nil
}

var _ repository.UserRepository = (*fakeUserRepo)(nil)
var _ repository.UserManagementRepository = (*fakeUserRepo)(nil)

type fakeRefreshTokenRepo struct{}

func (f *fakeRefreshTokenRepo) Create(ctx context.Context, token *domain.RefreshToken) error {
	return nil
}
func (f *fakeRefreshTokenRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	return nil, repository.ErrRefreshTokenNotFound
}
func (f *fakeRefreshTokenRepo) Rotate(ctx context.Context, oldTokenHash string, newToken *domain.RefreshToken) error {
	return nil
}
func (f *fakeRefreshTokenRepo) Revoke(ctx context.Context, tokenHash string, revokedAt time.Time) error {
	return nil
}
func (f *fakeRefreshTokenRepo) RevokeAllForUser(ctx context.Context, userID int64, revokedAt time.Time) error {
	return nil
}

type fakeTokenManager struct{}

func (f *fakeTokenManager) GenerateAccessToken(userID int64, email, role string) (string, time.Time, error) {
	return "test-token", time.Now().Add(time.Hour), nil
}
func (f *fakeTokenManager) ValidateAccessToken(tokenStr string) (*dto.TokenClaims, error) {
	return &dto.TokenClaims{UserID: 1, Email: "test@example.com", Role: "user"}, nil
}

func setupTestRouter() *http.ServeMux {
	productRepo := &fakeProductRepo{}
	productUC := usecase.NewProductUseCase(productRepo)
	productHandler := productHTTP.NewProductHandler(productUC)

	userRepo := &fakeUserRepo{}
	refreshRepo := &fakeRefreshTokenRepo{}
	tokenManager := &fakeTokenManager{}
	authUC := usecase.NewAuthUseCase(userRepo, refreshRepo, tokenManager, 15*time.Minute, 7*24*time.Hour, 15*time.Second)
	authHandler := productHTTP.NewAuthHandler(authUC, nil)

	userUC := usecase.NewUserUseCase(userRepo)
	userHandler := productHTTP.NewUserHandler(userUC)

	authenticator := &fakeAuth{}

	stockMovementHandler := productHTTP.NewStockMovementHandler(
		usecase.NewStockMovementUseCase(productRepo, &fakeStockMovementRepo{}),
	)

	dashboardHandler := productHTTP.NewDashboardHandler(usecase.NewDashboardUseCase(&fakeDashboardRepo{}))

	return NewRouter(nil, productHandler, authHandler, authenticator, userHandler, stockMovementHandler, dashboardHandler)
}

func TestProductRoutes_AuthorizationMatrix(t *testing.T) {
	router := setupTestRouter()

	validCreateBody := `{"sku":"NEW-SKU","name":"New Product","price":99,"quantity":10}`
	validUpdateBody := `{"sku":"UPD-SKU","name":"Updated Product","price":120}`
	validStockBody := `{"quantity":5}`

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string // empty for anonymous, "user-token", or "admin-token"
		wantStatus int
	}{
		// 1. Anonymous requests (Must return 401 Unauthorized for all product endpoints)
		{
			name:       "Anonymous - GET /products",
			method:     http.MethodGet,
			path:       "/api/v1/products",
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Anonymous - POST /products",
			method:     http.MethodPost,
			path:       "/api/v1/products",
			body:       validCreateBody,
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Anonymous - GET /products/1",
			method:     http.MethodGet,
			path:       "/api/v1/products/1",
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Anonymous - PUT /products/1",
			method:     http.MethodPut,
			path:       "/api/v1/products/1",
			body:       validUpdateBody,
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Anonymous - DELETE /products/1",
			method:     http.MethodDelete,
			path:       "/api/v1/products/1",
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Anonymous - POST /products/1/stock-in",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       validStockBody,
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Anonymous - POST /products/1/stock-out",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-out",
			body:       validStockBody,
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},

		// 2. Regular User (role: "user")
		// Allowed: GET /products, GET /products/1, stock-in, stock-out
		// Forbidden: POST /products, PUT /products/1, DELETE /products/1
		{
			name:       "User - GET /products (allowed)",
			method:     http.MethodGet,
			path:       "/api/v1/products",
			token:      "user-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "User - GET /products/1 (allowed)",
			method:     http.MethodGet,
			path:       "/api/v1/products/1",
			token:      "user-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "User - POST /products/1/stock-in (allowed)",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       validStockBody,
			token:      "user-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "User - POST /products/1/stock-out (allowed)",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-out",
			body:       validStockBody,
			token:      "user-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "User - POST /products (forbidden)",
			method:     http.MethodPost,
			path:       "/api/v1/products",
			body:       validCreateBody,
			token:      "user-token",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "User - PUT /products/1 (forbidden)",
			method:     http.MethodPut,
			path:       "/api/v1/products/1",
			body:       validUpdateBody,
			token:      "user-token",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "User - DELETE /products/1 (forbidden)",
			method:     http.MethodDelete,
			path:       "/api/v1/products/1",
			token:      "user-token",
			wantStatus: http.StatusForbidden,
		},

		// 3. Admin (role: "admin")
		// Allowed for all operations
		{
			name:       "Admin - GET /products (allowed)",
			method:     http.MethodGet,
			path:       "/api/v1/products",
			token:      "admin-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Admin - GET /products/1 (allowed)",
			method:     http.MethodGet,
			path:       "/api/v1/products/1",
			token:      "admin-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Admin - POST /products (allowed)",
			method:     http.MethodPost,
			path:       "/api/v1/products",
			body:       validCreateBody,
			token:      "admin-token",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "Admin - PUT /products/1 (allowed)",
			method:     http.MethodPut,
			path:       "/api/v1/products/1",
			body:       validUpdateBody,
			token:      "admin-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Admin - DELETE /products/1 (allowed)",
			method:     http.MethodDelete,
			path:       "/api/v1/products/1",
			token:      "admin-token",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "Admin - POST /products/1/stock-in (allowed)",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-in",
			body:       validStockBody,
			token:      "admin-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Admin - POST /products/1/stock-out (allowed)",
			method:     http.MethodPost,
			path:       "/api/v1/products/1/stock-out",
			body:       validStockBody,
			token:      "admin-token",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader *strings.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			} else {
				bodyReader = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if tt.token != "" {
				req.AddCookie(&http.Cookie{Name: productHTTP.AccessTokenCookieName, Value: tt.token})
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d (%s), want %d",
					tt.method, tt.path, rec.Code, rec.Body.String(), tt.wantStatus)
			}
		})
	}
}

func TestUserRoutes_AuthorizationMatrix(t *testing.T) {
	router := setupTestRouter()

	validPatchBody := `{"role":"admin"}`

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string
		wantStatus int
	}{
		// 1. Unauthenticated (no token) -> 401
		{
			name:       "No token - GET /users (401)",
			method:     http.MethodGet,
			path:       "/api/v1/users",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "No token - GET /users/1 (401)",
			method:     http.MethodGet,
			path:       "/api/v1/users/1",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "No token - PATCH /users/1 (401)",
			method:     http.MethodPatch,
			path:       "/api/v1/users/1",
			body:       validPatchBody,
			wantStatus: http.StatusUnauthorized,
		},

		// 2. Invalid token -> 401
		{
			name:       "Invalid token - GET /users (401)",
			method:     http.MethodGet,
			path:       "/api/v1/users",
			token:      "invalid-token",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Invalid token - GET /users/1 (401)",
			method:     http.MethodGet,
			path:       "/api/v1/users/1",
			token:      "invalid-token",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Invalid token - PATCH /users/1 (401)",
			method:     http.MethodPatch,
			path:       "/api/v1/users/1",
			body:       validPatchBody,
			token:      "invalid-token",
			wantStatus: http.StatusUnauthorized,
		},

		// 3. User (role: "user") -> 403 Forbidden
		{
			name:       "User role - GET /users (403)",
			method:     http.MethodGet,
			path:       "/api/v1/users",
			token:      "user-token",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "User role - GET /users/1 (403)",
			method:     http.MethodGet,
			path:       "/api/v1/users/1",
			token:      "user-token",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "User role - PATCH /users/1 (403)",
			method:     http.MethodPatch,
			path:       "/api/v1/users/1",
			body:       validPatchBody,
			token:      "user-token",
			wantStatus: http.StatusForbidden,
		},

		// 4. Admin (role: "admin") -> Allowed (200 OK)
		{
			name:       "Admin role - GET /users (200)",
			method:     http.MethodGet,
			path:       "/api/v1/users",
			token:      "admin-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Admin role - GET /users/1 (200)",
			method:     http.MethodGet,
			path:       "/api/v1/users/1",
			token:      "admin-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Admin role - PATCH /users/1 (200)",
			method:     http.MethodPatch,
			path:       "/api/v1/users/1",
			body:       validPatchBody,
			token:      "admin-token",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader *strings.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			} else {
				bodyReader = strings.NewReader("")
			}

			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d (%s), want %d",
					tt.method, tt.path, rec.Code, rec.Body.String(), tt.wantStatus)
			}
		})
	}
}

func TestRouter_RouteRegistration(t *testing.T) {
	t.Run("health check route registered", func(t *testing.T) {
		router := setupTestRouter()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for /health, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "health check passed") {
			t.Errorf("expected response to contain 'health check passed', got %s", rec.Body.String())
		}
	})

	t.Run("readiness check registered with database", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()
		mock.ExpectPing()

		productRepo := &fakeProductRepo{}
		productUC := usecase.NewProductUseCase(productRepo)
		productHandler := productHTTP.NewProductHandler(productUC)

		userRepo := &fakeUserRepo{}
		refreshRepo := &fakeRefreshTokenRepo{}
		tokenManager := &fakeTokenManager{}
		authUC := usecase.NewAuthUseCase(userRepo, refreshRepo, tokenManager, 15*time.Minute, 7*24*time.Hour, 15*time.Second)
		authHandler := productHTTP.NewAuthHandler(authUC, nil)

		authenticator := &fakeAuth{}
		userUC := usecase.NewUserUseCase(userRepo)
		userHandler := productHTTP.NewUserHandler(userUC)
		router := NewRouter(db, productHandler, authHandler, authenticator, userHandler, nil, nil)

		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for /ready, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "ready") {
			t.Errorf("expected response to contain ready, got %s", rec.Body.String())
		}
	})

	t.Run("readiness check omitted when database is nil", func(t *testing.T) {
		router := setupTestRouter()
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for /ready when db is nil, got %d", rec.Code)
		}
	})

	t.Run("auth routes registered", func(t *testing.T) {
		router := setupTestRouter()

		routes := []struct {
			path       string
			method     string
			wantNot404 bool
		}{
			{"/api/v1/auth/register", http.MethodPost, true},
			{"/api/v1/auth/login", http.MethodPost, true},
			{"/api/v1/auth/refresh", http.MethodPost, true},
			{"/api/v1/auth/logout", http.MethodPost, true},
			{"/api/v1/auth/me", http.MethodGet, true},
		}

		for _, r := range routes {
			req := httptest.NewRequest(r.method, r.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotFound {
				t.Errorf("route %s %s was not registered (got 404)", r.method, r.path)
			}
		}
	})
}

func TestApp_Lifecycle(t *testing.T) {
	validCfg := dto.AppConfig{
		DatabaseURL:             "postgres://user:pass@localhost:5432/gostock?sslmode=disable",
		HTTPPort:                "8080",
		JWTSecret:               "this-is-a-valid-secret-with-at-least-32-bytes-long!",
		JWTIssuer:               "gostock",
		JWTAudience:             "gostock-api",
		AccessTokenTTL:          15 * time.Minute,
		RefreshTokenTTL:         7 * 24 * time.Hour,
		RefreshTokenGracePeriod: 15 * time.Second,
	}

	t.Run("construction failure on invalid JWT secret returns useful error", func(t *testing.T) {
		cfg := validCfg
		cfg.JWTSecret = "too-short"

		app, err := New(cfg)
		if err == nil {
			t.Fatal("expected error for short JWT secret, got nil")
		}
		if app != nil {
			t.Fatal("expected nil App on construction error")
		}
		if !strings.Contains(err.Error(), "initialize jwt service") {
			t.Errorf("expected error to mention jwt service, got: %v", err)
		}
	})

	t.Run("construction failure on invalid database URL returns useful error", func(t *testing.T) {
		cfg := validCfg
		cfg.DatabaseURL = ""

		app, err := New(cfg)
		if err == nil {
			t.Fatal("expected error for empty DatabaseURL, got nil")
		}
		if app != nil {
			t.Fatal("expected nil App on construction error")
		}
		if !strings.Contains(err.Error(), "connect to database") {
			t.Errorf("expected error to mention connect to database, got: %v", err)
		}
	})

	t.Run("partially initialized DB is closed when post-DB step fails", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		mock.ExpectClose()

		mockOpener := func(dsn string) (*sql.DB, error) {
			return db, nil
		}
		simulatedFailure := func(db *sql.DB) error {
			return errors.New("simulated post-db failure")
		}

		app, err := newApp(validCfg, mockOpener, simulatedFailure)
		if err == nil {
			t.Fatal("expected error from post-db failure, got nil")
		}
		if app != nil {
			t.Fatal("expected nil App on error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("expected db.Close() to be called on failure: %v", err)
		}
	})

	t.Run("Close handles nil DB safely", func(t *testing.T) {
		app := &App{db: nil}
		if err := app.Close(); err != nil {
			t.Errorf("expected Close() on nil db to succeed, got %v", err)
		}

		var nilApp *App
		if err := nilApp.Close(); err != nil {
			t.Errorf("expected Close() on nil App to succeed, got %v", err)
		}
	})

	t.Run("Close is idempotent and does not close DB twice", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		mock.ExpectClose() // only 1 Close call expected

		app := &App{db: db}
		if err := app.Close(); err != nil {
			t.Fatalf("first Close() failed: %v", err)
		}
		if err := app.Close(); err != nil {
			t.Fatalf("second Close() failed: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Close was unexpectedly called more than once: %v", err)
		}
	})

	t.Run("Run with already-canceled context returns error without starting server", func(t *testing.T) {
		server := &http.Server{
			Addr:    "127.0.0.1:0",
			Handler: http.NotFoundHandler(),
		}
		app := &App{server: server}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // canceled before calling Run

		err := app.Run(ctx)
		if err == nil {
			t.Fatal("expected error on canceled context, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected error wrapping context.Canceled, got: %v", err)
		}
	})

	t.Run("Run returns error if called a second time", func(t *testing.T) {
		server := &http.Server{
			Addr:    "127.0.0.1:0",
			Handler: http.NotFoundHandler(),
		}
		app := &App{server: server}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately
		_ = app.Run(ctx)

		// Second run attempt
		err := app.Run(context.Background())
		if err == nil {
			t.Fatal("expected error on second Run(), got nil")
		}
		if !strings.Contains(err.Error(), "already started") {
			t.Errorf("expected error mentioning 'already started', got: %v", err)
		}
	})

	t.Run("Run starts server and handles graceful shutdown deterministically", func(t *testing.T) {
		ready := make(chan struct{})
		server := &http.Server{
			Addr:    "127.0.0.1:0",
			Handler: http.NotFoundHandler(),
		}
		app := &App{
			server:  server,
			readyCh: ready,
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- app.Run(ctx)
		}()

		select {
		case <-ready:
			// Network listener has been bound; cancel context to trigger shutdown
			cancel()
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for readyCh")
		}

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("expected nil on normal shutdown, got: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for Run to exit")
		}
	})

	t.Run("shutdown failure triggers force close and returns error", func(t *testing.T) {
		ready := make(chan struct{})
		server := &http.Server{
			Addr:    "127.0.0.1:0",
			Handler: http.NotFoundHandler(),
		}
		simulatedErr := errors.New("simulated shutdown timeout")
		app := &App{
			server:  server,
			readyCh: ready,
			shutdownFunc: func(ctx context.Context) error {
				return simulatedErr
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- app.Run(ctx)
		}()

		select {
		case <-ready:
			cancel()
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for readyCh")
		}

		select {
		case err := <-errCh:
			if err == nil {
				t.Fatal("expected error on shutdown timeout/failure, got nil")
			}
			if !strings.Contains(err.Error(), "shutdown HTTP server") || !strings.Contains(err.Error(), "simulated shutdown timeout") {
				t.Errorf("expected error to mention shutdown HTTP server and simulated timeout, got: %v", err)
			}
			if !errors.Is(err, simulatedErr) {
				t.Errorf("expected errors.Is(err, simulatedErr) to be true, got false; err: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for force-close shutdown to return")
		}
	})

	t.Run("Run returns error when network listener fails to bind", func(t *testing.T) {
		server := &http.Server{
			Addr:    "invalid-host-name-cannot-bind:99999",
			Handler: http.NotFoundHandler(),
		}
		app := &App{server: server}

		err := app.Run(context.Background())
		if err == nil {
			t.Fatal("expected error when server fails to bind, got nil")
		}
		if !strings.Contains(err.Error(), "listen on") {
			t.Errorf("expected error to mention 'listen on', got: %v", err)
		}
	})

	t.Run("Run returns error on nil or uninitialized application", func(t *testing.T) {
		var nilApp *App
		if err := nilApp.Run(context.Background()); err == nil {
			t.Fatal("expected error on nil App, got nil")
		}

		app := &App{server: nil}
		if err := app.Run(context.Background()); err == nil {
			t.Fatal("expected error on nil server, got nil")
		}
	})
}

// startTestConfig returns a configuration that newApp accepts.
func startTestConfig() dto.AppConfig {
	return dto.AppConfig{
		DatabaseURL:             "postgres://user:pass@localhost:5432/gostock?sslmode=disable",
		HTTPPort:                "8080",
		JWTSecret:               "this-is-a-valid-secret-with-at-least-32-bytes-long!",
		JWTIssuer:               "gostock",
		JWTAudience:             "gostock-api",
		AccessTokenTTL:          15 * time.Minute,
		RefreshTokenTTL:         7 * 24 * time.Hour,
		RefreshTokenGracePeriod: 15 * time.Second,
	}
}

// countingOpener returns a dbOpener that hands out db and counts its calls.
func countingOpener(db *sql.DB) (dbOpener, *int) {
	calls := new(int)
	return func(string) (*sql.DB, error) {
		*calls++
		return db, nil
	}, calls
}

// runStartUntilReady runs start in a goroutine, cancels its context once the
// listener is bound, and returns start's result.
func runStartUntilReady(t *testing.T, cfg dto.AppConfig, opener dbOpener) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- start(ctx, cfg, opener, ready, "127.0.0.1:0")
	}()

	select {
	case <-ready:
		cancel()
	case err := <-errCh:
		t.Fatalf("start returned before the listener was ready: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the listener to be ready")
	}

	select {
	case err := <-errCh:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for start to return after cancellation")
		return nil
	}
}

func TestStart_InvalidJWTConfigDoesNotOpenDatabase(t *testing.T) {
	cfg := startTestConfig()
	cfg.JWTSecret = "too-short"
	opener, calls := countingOpener(nil)

	err := start(context.Background(), cfg, opener, nil, "")

	if err == nil {
		t.Fatal("expected an error for an invalid JWT secret, got nil")
	}
	if !strings.Contains(err.Error(), "initialize application") || !strings.Contains(err.Error(), "initialize jwt service") {
		t.Errorf("error %q should mention initialize application and initialize jwt service", err)
	}
	if *calls != 0 {
		t.Errorf("database opener called %d times, want 0", *calls)
	}
}

func TestStart_DatabaseOpenFailure(t *testing.T) {
	dbErr := errors.New("database unreachable")
	opener := func(string) (*sql.DB, error) { return nil, dbErr }

	err := start(context.Background(), startTestConfig(), opener, nil, "")

	if !errors.Is(err, dbErr) {
		t.Fatalf("error = %v, want it to wrap the opener error", err)
	}
	if !strings.Contains(err.Error(), "initialize application") {
		t.Errorf("error %q should mention initialize application", err)
	}
}

func TestStart_CancelledContextDoesNotOpenDatabase(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opener, calls := countingOpener(nil)

	err := start(ctx, startTestConfig(), opener, nil, "")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want it to wrap context.Canceled", err)
	}
	if *calls != 0 {
		t.Errorf("database opener called %d times, want 0", *calls)
	}
}

func TestStart_GracefulShutdownClosesDatabaseOnce(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	mock.ExpectClose() // exactly one Close is expected
	opener, calls := countingOpener(db)

	if err := runStartUntilReady(t, startTestConfig(), opener); err != nil {
		t.Fatalf("expected nil after graceful shutdown, got: %v", err)
	}
	if *calls != 1 {
		t.Errorf("database opener called %d times, want 1", *calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("database was not closed exactly once: %v", err)
	}
}

func TestStart_ListenFailureStillClosesDatabase(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	mock.ExpectClose()
	opener, _ := countingOpener(db)

	cfg := startTestConfig()
	cfg.HTTPPort = "99999" // out of range: net.Listen fails through the real config path

	err = start(context.Background(), cfg, opener, nil, "")

	if err == nil {
		t.Fatal("expected a listen error, got nil")
	}
	if !strings.Contains(err.Error(), "run application") || !strings.Contains(err.Error(), "listen on") {
		t.Errorf("error %q should mention run application and listen on", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("database was not closed exactly once after a listen failure: %v", err)
	}
}

// TestStart_RunAndCloseFailuresBothReported simulates a shutdown signal that
// arrives while the database is being opened: Run then fails with
// context.Canceled, and Close also fails. Both errors must stay discoverable.
func TestStart_RunAndCloseFailuresBothReported(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	closeErr := errors.New("simulated close failure")
	mock.ExpectClose().WillReturnError(closeErr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opener := func(string) (*sql.DB, error) {
		cancel() // the "signal" arrives during startup
		return db, nil
	}

	err = start(ctx, startTestConfig(), opener, nil, "127.0.0.1:0")

	if !errors.Is(err, context.Canceled) {
		t.Errorf("error %v should wrap context.Canceled (the Run failure)", err)
	}
	if !errors.Is(err, closeErr) {
		t.Errorf("error %v should wrap the Close failure", err)
	}
	if !strings.Contains(err.Error(), "run application") || !strings.Contains(err.Error(), "close database") {
		t.Errorf("error %q should mention both run application and close database", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("database Close was not attempted: %v", err)
	}
}

func TestStart_CloseFailureAfterSuccessfulRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	closeErr := errors.New("simulated close failure")
	mock.ExpectClose().WillReturnError(closeErr)
	opener, _ := countingOpener(db)

	err = runStartUntilReady(t, startTestConfig(), opener)

	if !errors.Is(err, closeErr) {
		t.Fatalf("error = %v, want the Close failure to be returned", err)
	}
	if strings.Contains(err.Error(), "run application") {
		t.Errorf("error %q must not report a Run failure after a clean shutdown", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("database Close was not attempted: %v", err)
	}
}

// preflightConfig returns a configuration for the integration test database,
// whose name testdb.Open has already checked to be exactly gostock_test.
func preflightConfig(t *testing.T, expected string) dto.AppConfig {
	t.Helper()
	testdb.Open(t)
	cfg := startTestConfig()
	cfg.DatabaseURL = os.Getenv(testdb.EnvVar)
	cfg.ExpectedDatabaseName = expected
	return cfg
}

// trackingOpener opens the real database and records the pool it returned.
func trackingOpener(opened **sql.DB) dbOpener {
	return func(dsn string) (*sql.DB, error) {
		db, err := database.OpenPostgres(dsn)
		*opened = db
		return db, err
	}
}

func assertPoolClosed(t *testing.T, db *sql.DB) {
	t.Helper()
	if db == nil {
		t.Fatal("the database was never opened")
	}
	if err := db.Ping(); err == nil || !strings.Contains(err.Error(), "database is closed") {
		t.Errorf("Ping after failed construction = %v, want the pool closed", err)
	}
}

func TestNewApp_DatabasePreflight(t *testing.T) {
	t.Run("connected to another database: refused and pool closed", func(t *testing.T) {
		// The UI database is expected, but DATABASE_URL points at gostock_test.
		cfg := preflightConfig(t, "gostock_ui_test")
		var opened *sql.DB
		hookRan := false

		app, err := newApp(cfg, trackingOpener(&opened), func(*sql.DB) error {
			hookRan = true
			return nil
		})
		if err == nil {
			_ = app.Close()
			t.Fatal("newApp succeeded on the wrong database")
		}
		if app != nil {
			t.Error("newApp returned an App with its error")
		}
		if want := `connected to "gostock_test", expected "gostock_ui_test"`; !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
		if strings.Contains(err.Error(), cfg.DatabaseURL) {
			t.Error("error contains the connection string")
		}
		if hookRan {
			t.Error("construction continued past the failed preflight")
		}
		assertPoolClosed(t, opened)
	})

	t.Run("connected to the expected database: constructed", func(t *testing.T) {
		cfg := preflightConfig(t, testdb.RequiredName)
		var opened *sql.DB

		app, err := newApp(cfg, trackingOpener(&opened), nil)
		if err != nil {
			t.Fatalf("newApp: %v", err)
		}
		if err := app.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	t.Run("unset: no query is run, as before", func(t *testing.T) {
		db, mock, err := sqlmock.New() // fails on any query it does not expect
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		mock.ExpectClose()

		cfg := startTestConfig()
		cfg.ExpectedDatabaseName = ""
		app, err := newApp(cfg, func(string) (*sql.DB, error) { return db, nil }, nil)
		if err != nil {
			t.Fatalf("newApp: %v", err)
		}
		if err := app.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unexpected database use: %v", err)
		}
	})
}

// TestStart_DatabasePreflightFailureDoesNotListen goes through the same path
// as the server (Start): on the wrong database it fails before binding.
func TestStart_DatabasePreflightFailureDoesNotListen(t *testing.T) {
	cfg := preflightConfig(t, "gostock_ui_test")
	var opened *sql.DB
	ready := make(chan struct{})

	// Without the preflight, start would serve until cancelled: the deadline
	// turns that regression into a failure instead of a hang.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := start(ctx, cfg, trackingOpener(&opened), ready, "127.0.0.1:0")
	select {
	case <-ready:
		t.Fatal("the HTTP listener was bound despite the wrong database")
	default:
	}
	if err == nil || !strings.Contains(err.Error(), `expected "gostock_ui_test"`) {
		t.Fatalf("start error = %v, want the preflight error", err)
	}
	assertPoolClosed(t, opened)
}
