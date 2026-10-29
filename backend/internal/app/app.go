package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"gostock/config"
	productHTTP "gostock/internal/handle/http"
	"gostock/internal/platform/database"
	"gostock/internal/repository/postgres"
	"gostock/internal/usecase"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func NewServer(cfg config.Config) (*http.Server, *sql.DB, error) {
	db, err := database.OpenPostgres(cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}

	productRepo := postgres.NewProductRepository(db)
	productUseCase := usecase.NewProductUseCase(productRepo)
	productHandler := productHTTP.NewProductHandler(productUseCase)

	userRepo := postgres.NewUserRepository(db)
	sessionRepo := postgres.NewSessionRepository(db)

	authUseCase := usecase.NewAuthUseCase(
		userRepo,
		sessionRepo,
		cfg.SessionTTL,
	)
	authHandler := productHTTP.NewAuthHandler(authUseCase)

	mux := NewRouter(db, productHandler, authHandler, authUseCase)

	server := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: mux,
	}

	return server, db, nil
}

func NewRouter(
	db *sql.DB,
	productHandler *productHTTP.ProductHandler,
	authHandler *productHTTP.AuthHandler,
	authUseCase productHTTP.Authenticator,
) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", productHTTP.HealthHandler)

	// Đăng ký Auth routes (public)
	mux.HandleFunc("/api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("/api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("/api/v1/auth/logout", authHandler.Logout)

	// Auth route yêu cầu xác thực
	authMiddleware := productHTTP.AuthMiddleware(authUseCase)
	mux.Handle(
		"/api/v1/auth/me",
		authMiddleware(
			http.HandlerFunc(authHandler.Me),
		),
	)

	if db != nil {
		readinessHandler := productHTTP.NewReadinessHandler(db)
		mux.Handle("/ready", readinessHandler)
	}

	createHandler := productHTTP.RequireAdmin(http.HandlerFunc(productHandler.Create))
	updateHandler := productHTTP.RequireAdmin(http.HandlerFunc(productHandler.Update))
	deleteHandler := productHTTP.RequireAdmin(http.HandlerFunc(productHandler.Delete))

	// Protected Product routes (/api/v1/products):
	// - GET: Authenticated (User & Admin) -> List
	// - POST: Admin Only -> Create
	mux.Handle("/api/v1/products", authMiddleware(http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		switch r.Method {
		case http.MethodGet:
			productHandler.List(w, r)
		case http.MethodPost:
			createHandler.ServeHTTP(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(
				w,
				"Method not allowed",
				http.StatusMethodNotAllowed,
			)
		}
	})))

	// Protected Product routes (/api/v1/products/):
	// - POST /stock-in: Authenticated (User & Admin) -> StockIn
	// - POST /stock-out: Authenticated (User & Admin) -> StockOut
	// - GET /{id}: Authenticated (User & Admin) -> GetByID
	// - PUT /{id}: Admin Only -> Update
	// - DELETE /{id}: Admin Only -> Delete
	mux.Handle("/api/v1/products/", authMiddleware(http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		path := r.URL.Path

		if strings.HasSuffix(path, "/stock-in") {
			productHandler.StockIn(w, r)
			return
		}

		if strings.HasSuffix(path, "/stock-out") {
			productHandler.StockOut(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			productHandler.GetByID(w, r)
		case http.MethodPut:
			updateHandler.ServeHTTP(w, r)
		case http.MethodDelete:
			deleteHandler.ServeHTTP(w, r)
		default:
			w.Header().Set("Allow", "GET, PUT, DELETE")
			http.Error(
				w,
				"Method not allowed",
				http.StatusMethodNotAllowed,
			)
		}
	})))

	return mux
}

func Run(cfg config.Config) error {
	server, db, err := NewServer(cfg)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	errCh := make(chan error, 1)

	go func() {
		fmt.Printf("GoStock is running at http://localhost%s\n", server.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("run HTTP server: %w", err)

	case <-ctx.Done():
		fmt.Println("Shutdown signal received")

		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}

		// Wait for ListenAndServe to return.
		err := <-errCh
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("stop HTTP server: %w", err)
		}

		fmt.Println("GoStock stopped gracefully")
		return nil
	}
}
