package app

import (
	"database/sql"
	"net/http"
	"strings"

	productHTTP "gostock/internal/handle/http"
	"gostock/internal/handle/http/response"
)

func NewRouter(
	db *sql.DB,
	productHandler *productHTTP.ProductHandler,
	authHandler *productHTTP.AuthHandler,
	authUseCase productHTTP.Authenticator,
	userHandler *productHTTP.UserHandler,
	stockMovementHandler *productHTTP.StockMovementHandler,
	dashboardHandler *productHTTP.DashboardHandler,
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

	mux.Handle("/api/v1/products/", authMiddleware(http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		path := r.URL.Path

		// GET /api/v1/products/{id}/movements: User & Admin.
		if stockMovementHandler != nil && strings.HasSuffix(path, "/movements") {
			stockMovementHandler.ListByProduct(w, r)
			return
		}

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

	// GET /api/v1/stock-movements: Admin only. Includes movements of deleted
	// products.
	if stockMovementHandler != nil {
		mux.Handle("/api/v1/stock-movements", authMiddleware(productHTTP.RequireAdmin(
			http.HandlerFunc(stockMovementHandler.List),
		)))
	}

	// GET /api/v1/dashboard/summary and /api/v1/dashboard/stock-movements:
	// User & Admin.
	if dashboardHandler != nil {
		mux.Handle("/api/v1/dashboard/summary", authMiddleware(http.HandlerFunc(dashboardHandler.Summary)))
		mux.Handle("/api/v1/dashboard/stock-movements", authMiddleware(http.HandlerFunc(dashboardHandler.StockMovementChart)))
	}

	return mux
}
