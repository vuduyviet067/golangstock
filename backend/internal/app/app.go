package app

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"gostock/config"
	productHTTP "gostock/internal/handle/http"
	"gostock/internal/platform/database"
	"gostock/internal/repository/postgres"
	"gostock/internal/usecase"
)

func NewServer(cfg config.Config) (*http.Server, *sql.DB, error) {
	db, err := database.OpenPostgres(cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}

	productRepo := postgres.NewProductRepository(db)
	productUseCase := usecase.NewProductUseCase(productRepo)
	productHandler := productHTTP.NewProductHandler(productUseCase)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", productHTTP.HealthHandler)

	mux.HandleFunc("/api/v1/products", func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		switch r.Method {
		case http.MethodGet:
			productHandler.List(w, r)
		case http.MethodPost:
			productHandler.Create(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(
				w,
				"Method not allowed",
				http.StatusMethodNotAllowed,
			)
		}
	})

	mux.HandleFunc("/api/v1/products/", func(
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
			productHandler.Update(w, r)
		case http.MethodDelete:
			productHandler.Delete(w, r)
		default:
			w.Header().Set("Allow", "GET, PUT, DELETE")
			http.Error(
				w,
				"Method not allowed",
				http.StatusMethodNotAllowed,
			)
		}
	})

	server := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: mux,
	}

	return server, db, nil
}

func Run(cfg config.Config) error {
	server, db, err := NewServer(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	fmt.Printf("GoStock is running at http://localhost%s\n", server.Addr)

	return server.ListenAndServe()
}
