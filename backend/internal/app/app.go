package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"gostock/config"
	productHTTP "gostock/internal/handle/http"
	"gostock/internal/platform/database"
	"gostock/internal/platform/jwt"
	"gostock/internal/repository/postgres"
	"gostock/internal/usecase"
)

const defaultShutdownTimeout = 10 * time.Second

// App encapsulates the application's runtime dependencies and manages its lifecycle.
type App struct {
	server          *http.Server
	db              *sql.DB
	shutdownTimeout time.Duration
	readyCh         chan struct{}                   // optional channel closed when network listener is bound
	shutdownFunc    func(ctx context.Context) error // optional test seam to simulate shutdown failure
	started         bool
	closed          bool
	mu              sync.Mutex
}

type dbOpener func(dsn string) (*sql.DB, error)

// New initializes the composition root, wiring together repositories, use cases,
// handlers, router, and configuring the HTTP server with timeouts.
// If initialization fails after opening the database, the database is closed before returning.
func New(cfg config.Config) (*App, error) {
	return newApp(cfg, database.OpenPostgres, nil)
}

// newApp is an internal helper allowing dependency injection for database opener and post-db hooks.
func newApp(cfg config.Config, opener dbOpener, postDBHook func(db *sql.DB) error) (*App, error) {
	// 1. Initialize JWT service first (fails fast before touching the database)
	jwtService, err := jwt.NewJWTService(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTAudience, cfg.AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("initialize jwt service: %w", err)
	}

	// 2. Open PostgreSQL connection pool
	db, err := opener(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	// Resource safety: If any subsequent initialization step fails, guarantee db cleanup
	var success bool
	defer func() {
		if !success && db != nil {
			_ = db.Close()
		}
	}()

	if postDBHook != nil {
		if err := postDBHook(db); err != nil {
			return nil, fmt.Errorf("post-database hook: %w", err)
		}
	}

	// 3. Construct repositories
	productRepo := postgres.NewProductRepository(db)
	userRepo := postgres.NewUserRepository(db)
	refreshRepo := postgres.NewRefreshTokenRepository(db)

	// 4. Construct use cases
	productUseCase := usecase.NewProductUseCase(productRepo)
	authUseCase := usecase.NewAuthUseCase(
		userRepo,
		refreshRepo,
		jwtService,
		cfg.AccessTokenTTL,
		cfg.RefreshTokenTTL,
		cfg.RefreshTokenGracePeriod,
	)
	userUseCase := usecase.NewUserUseCase(userRepo)

	// 5. Construct HTTP handlers
	cookieManager := productHTTP.NewCookieManager(cfg.CookieSecure, cfg.CookieSameSite, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	authHandler := productHTTP.NewAuthHandler(authUseCase, cookieManager)
	productHandler := productHTTP.NewProductHandler(productUseCase)
	userHandler := productHTTP.NewUserHandler(userUseCase)

	// 6. Construct router and middleware
	router := NewRouter(db, productHandler, authHandler, authUseCase, userHandler)
	handler := productHTTP.CORSMiddleware(cfg.AllowedOrigins)(router)

	// 7. Configure HTTP server with production-grade timeouts
	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	success = true
	return &App{
		server:          server,
		db:              db,
		shutdownTimeout: defaultShutdownTimeout,
	}, nil
}

// Run starts the HTTP server and coordinates graceful shutdown on context cancellation.
// It guarantees that:
// - A canceled context before startup does not start the server or leak goroutines.
// - The server is started exactly once.
// - Shutdown is properly coordinated with the serving goroutine.
// - http.ErrServerClosed is treated as normal shutdown.
// - Unexpected serving errors are returned to the caller.
func (a *App) Run(ctx context.Context) error {
	if a == nil || a.server == nil {
		return errors.New("cannot run uninitialized application")
	}

	a.mu.Lock()
	if a.started {
		a.mu.Unlock()
		return errors.New("application already started")
	}
	a.started = true
	a.mu.Unlock()

	// 1. If context is already canceled, do not start the server
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context canceled before server start: %w", err)
	}

	// 2. Bind the network listener synchronously so startup errors are immediate
	listener, err := net.Listen("tcp", a.server.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", a.server.Addr, err)
	}
	a.server.Addr = listener.Addr().String()

	// 3. Double-check context cancellation after binding before launching goroutine
	if err := ctx.Err(); err != nil {
		_ = listener.Close()
		return fmt.Errorf("context canceled before serving: %w", err)
	}

	if a.readyCh != nil {
		close(a.readyCh)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.server.Serve(listener)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP server: %w", err)

	case <-ctx.Done():
		timeout := a.shutdownTimeout
		if timeout <= 0 {
			timeout = defaultShutdownTimeout
		}

		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			timeout,
		)
		defer cancel()

		var shutdownErr error
		if a.shutdownFunc != nil {
			if err := a.shutdownFunc(shutdownCtx); err != nil {
				shutdownErr = fmt.Errorf("shutdown HTTP server: %w", err)
			}
		} else {
			if err := a.server.Shutdown(shutdownCtx); err != nil {
				shutdownErr = fmt.Errorf("shutdown HTTP server: %w", err)
			}
		}

		if shutdownErr != nil {
			_ = a.server.Close()
		}

		// Wait for the serving goroutine to exit
		serveErr := <-errCh
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = fmt.Errorf("serve HTTP server: %w", serveErr)
			if shutdownErr != nil {
				return errors.Join(shutdownErr, serveErr)
			}
			return serveErr
		}

		if shutdownErr != nil {
			return shutdownErr
		}

		return nil
	}
}

// Start builds the application from cfg, serves HTTP until ctx is cancelled,
// and always releases the resources it opened. It is the single entry point a
// process needs: callers own only the context (for example, one cancelled by
// OS signals) and the configuration, never the App or its database.
//
// The returned error wraps whichever step failed. If closing the database also
// fails, that error is joined with it, so both remain discoverable with
// errors.Is; a cleanup failure after a clean shutdown is returned on its own.
func Start(ctx context.Context, cfg config.Config) error {
	return start(ctx, cfg, database.OpenPostgres, startOptions{})
}

// startOptions are test seams for start. The zero value is production behavior.
type startOptions struct {
	// ready, if non-nil, is closed once the network listener is bound.
	ready chan struct{}
	// listenAddr, if non-empty, overrides the server address so tests can
	// bind to 127.0.0.1:0 instead of every interface.
	listenAddr string
}

func start(ctx context.Context, cfg config.Config, opener dbOpener, opts startOptions) (err error) {
	// Do not open resources for a process that is already shutting down.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("start application: %w", ctxErr)
	}

	// newApp closes the database itself if construction fails after opening it.
	application, err := newApp(cfg, opener, nil)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	application.readyCh = opts.ready
	if opts.listenAddr != "" {
		application.server.Addr = opts.listenAddr
	}

	if runErr := application.Run(ctx); runErr != nil {
		return fmt.Errorf("run application: %w", runErr)
	}
	return nil
}

// Close closes the underlying database connection pool safely.
// It is nil-safe, idempotent, and guarantees database resources are not closed multiple times.
func (a *App) Close() error {
	if a == nil {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}
	a.closed = true

	if a.db != nil {
		if err := a.db.Close(); err != nil {
			return fmt.Errorf("close database: %w", err)
		}
	}

	return nil
}
