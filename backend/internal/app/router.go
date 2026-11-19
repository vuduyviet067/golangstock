package app

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"

	productHTTP "gostock/internal/handle/http"
)

// NewRouter builds the API router.
//
// Routes are grouped by permission: public, authenticated (AuthMiddleware) and
// admin (AuthMiddleware then RequireAdmin). A protected group is a chi
// subrouter whose middleware runs before chi resolves the method, so a wrong
// method gets 401 or 403 before 405, as before the move to chi. Unknown paths
// get 404 NOT_FOUND before any authentication (RequireRoute); a wrong method
// gets 405 METHOD_NOT_ALLOWED with an Allow header. Both are written by the
// central handlers of internal/handle/http. CORS wraps the router in app.New.
func NewRouter(
	db *sql.DB,
	productHandler *productHTTP.ProductHandler,
	authHandler *productHTTP.AuthHandler,
	authUseCase productHTTP.Authenticator,
	userHandler *productHTTP.UserHandler,
	stockMovementHandler *productHTTP.StockMovementHandler,
	dashboardHandler *productHTTP.DashboardHandler,
	woodTypeHandler *productHTTP.WoodTypeHandler,
	supplierHandler *productHTTP.SupplierHandler,
) http.Handler {
	r := chi.NewRouter()
	routes := productHTTP.NewRouteTable(r)
	r.NotFound(productHTTP.NotFound)
	r.MethodNotAllowed(routes.MethodNotAllowed)
	r.Use(routes.RequireRoute)

	authenticated := productHTTP.AuthMiddleware(authUseCase)
	admin := productHTTP.RequireAdmin

	// Public.
	r.Get("/health", productHTTP.HealthHandler)
	if db != nil {
		// Readiness probe (pings PostgreSQL).
		r.Method(http.MethodGet, "/ready", productHTTP.NewReadinessHandler(db))
	}

	r.Route("/api/v1", func(r chi.Router) {
		// Public.
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)
		r.Post("/auth/refresh", authHandler.Refresh)
		r.Post("/auth/logout", authHandler.Logout)

		// Authenticated: any role. Writing products is admin only.
		r.Route("/auth/me", func(r chi.Router) {
			r.Use(authenticated)
			r.Get("/", authHandler.Me)
		})
		r.Route("/auth/change-password", func(r chi.Router) {
			r.Use(authenticated)
			r.Post("/", authHandler.ChangePassword)
		})
		r.Route("/products", func(r chi.Router) {
			r.Use(authenticated)
			r.Get("/", productHandler.List)
			r.With(admin).Post("/", productHandler.Create)
			r.Get("/{id}", productHandler.GetByID)
			r.With(admin).Put("/{id}", productHandler.Update)
			r.With(admin).Delete("/{id}", productHandler.Delete)
			if stockMovementHandler != nil {
				r.Get("/{id}/movements", stockMovementHandler.ListByProduct)
			}
			r.Post("/{id}/stock-in", productHandler.StockIn)
			r.Post("/{id}/stock-out", productHandler.StockOut)
		})

		// Admin. The role is checked before the method and the query.
		if userHandler != nil {
			r.Route("/users", func(r chi.Router) {
				r.Use(authenticated, admin)
				r.Get("/", userHandler.List)
				r.Get("/{id}", userHandler.GetByID)
				r.Patch("/{id}", userHandler.Update)
			})
		}
		if stockMovementHandler != nil {
			// Includes movements of deleted products.
			r.Route("/stock-movements", func(r chi.Router) {
				r.Use(authenticated, admin)
				r.Get("/", stockMovementHandler.List)
			})
		}
		if dashboardHandler != nil {
			r.Route("/dashboard", func(r chi.Router) {
				r.Use(authenticated, admin)
				r.Get("/summary", dashboardHandler.Summary)
				r.Get("/stock-movements", dashboardHandler.StockMovementChart)
			})
		}
		// Catalogs of the timber workshop (XG-05).
		if woodTypeHandler != nil {
			r.Route("/wood-types", func(r chi.Router) {
				r.Use(authenticated, admin)
				r.Get("/", woodTypeHandler.List)
				r.Post("/", woodTypeHandler.Create)
				r.Patch("/{id}", woodTypeHandler.Update)
				r.Delete("/{id}", woodTypeHandler.Delete)
			})
		}
		if supplierHandler != nil {
			r.Route("/suppliers", func(r chi.Router) {
				r.Use(authenticated, admin)
				r.Get("/", supplierHandler.List)
				r.Post("/", supplierHandler.Create)
				r.Patch("/{id}", supplierHandler.Update)
				r.Delete("/{id}", supplierHandler.Delete)
			})
		}
	})

	return r
}
