package app

import (
	"database/sql"
	"net/http"
	"strings"

	productHTTP "gostock/internal/handle/http"
	"gostock/internal/handle/http/response"
)

// NewRouter constructs and configures the HTTP router with all application endpoints,
// public and protected routes, CORS, authentication and authorization middlewares.
func NewRouter(
	db *sql.DB,
	productHandler *productHTTP.ProductHandler,
	authHandler *productHTTP.AuthHandler,
	authUseCase productHTTP.Authenticator,
	userHandler *productHTTP.UserHandler,
) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", productHTTP.HealthHandler)

	// Public Auth routes
	mux.HandleFunc("/api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("/api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("/api/v1/auth/refresh", authHandler.Refresh)
	mux.HandleFunc("/api/v1/auth/logout", authHandler.Logout)

	// Protected Auth routes
	authMiddleware := productHTTP.AuthMiddleware(authUseCase)
	mux.Handle(
		"/api/v1/auth/me",
		authMiddleware(
			http.HandlerFunc(authHandler.Me),
		),
	)

	// Readiness probe (pings PostgreSQL when available)
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
			response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
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
			response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})))

	// Protected User Management routes (Admin Only):
	// - GET   /api/v1/users     -> List
	// - GET   /api/v1/users/{id} -> GetByID
	// - PATCH /api/v1/users/{id} -> Update
	if userHandler != nil {
		mux.Handle("/api/v1/users", authMiddleware(productHTTP.RequireAdmin(http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			if r.Method == http.MethodGet {
				userHandler.List(w, r)
				return
			}
			w.Header().Set("Allow", "GET")
			response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		}))))

		mux.Handle("/api/v1/users/", authMiddleware(productHTTP.RequireAdmin(http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			switch r.Method {
			case http.MethodGet:
				userHandler.GetByID(w, r)
			case http.MethodPatch:
				userHandler.Update(w, r)
			default:
				w.Header().Set("Allow", "GET, PATCH")
				response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}))))
	}

	return mux
}
