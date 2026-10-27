package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// handler
func healthHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	response := HealthResponse{
		Status:   "ok",
		App:      "GoStock",
		Database: "connected",
	}

	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

func productsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listProductsHandler(db)(w, r)
		case http.MethodPost:
			createProductHandler(db)(w, r)
		case http.MethodPut:
			updateProductHandler(db)(w, r)
		case http.MethodDelete:
			deleteProductHandler(db)(w, r)
		default:
			w.Header().Set("Allow", "GET, POST, PUT, DELETE")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func updateProductHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateProductRequest

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, "Request must contain only one JSON object", http.StatusBadRequest)
			return
		}

		req.SKU = strings.TrimSpace(req.SKU)
		req.Name = strings.TrimSpace(req.Name)

		if req.SKU == "" || len(req.SKU) > 50 {
			http.Error(w, "SKU is required and must be at most 50 characters", http.StatusBadRequest)
			return
		}
		if req.Name == "" || len(req.Name) > 200 {
			http.Error(w, "Name is required and must be at most 200 characters", http.StatusBadRequest)
			return
		}
		if req.Price < 0 || req.Quantity < 0 {
			http.Error(w, "Price and quantity must be non-negative", http.StatusBadRequest)
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
		if id == "" || strings.Contains(id, "/") {
			http.Error(w, "Invalid product ID", http.StatusBadRequest)
			return
		}

		var product Product
		err := db.QueryRowContext(
			r.Context(),
			`UPDATE products
			 SET sku = $1, name = $2, price = $3,
			     quantity = $4, updated_at = NOW()
			 WHERE id = $5
			 RETURNING id, sku, name, price, quantity`,
			req.SKU, req.Name, req.Price, req.Quantity, id,
		).Scan(
			&product.ID, &product.SKU, &product.Name,
			&product.Price, &product.Quantity,
		)

		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Product not found", http.StatusNotFound)
			return
		}
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				http.Error(w, "SKU already exists", http.StatusConflict)
				return
			}
			log.Printf("update product: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(product)
	}
}

func deleteProductHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")

		if id == "" || strings.Contains(id, "/") {
			http.Error(w, "Invalid product ID", http.StatusBadRequest)
			return
		}

		result, err := db.ExecContext(
			r.Context(),
			`DELETE FROM products WHERE id = $1`,
			id,
		)
		if err != nil {
			log.Printf("delete product: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			log.Printf("check deleted product: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if rowsAffected == 0 {
			http.Error(w, "Product not found", http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func listProductsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		q := strings.TrimSpace(r.URL.Query().Get("q"))

		page := 1
		limit := 10

		if value := r.URL.Query().Get("page"); value != "" {
			parsedPage, err := strconv.Atoi(value)
			if err != nil || parsedPage < 1 {
				http.Error(w, "page must be a positive integer", http.StatusBadRequest)
				return
			}
			page = parsedPage
		}

		if value := r.URL.Query().Get("limit"); value != "" {
			parsedLimit, err := strconv.Atoi(value)
			if err != nil || parsedLimit < 1 || parsedLimit > 100 {
				http.Error(w, "limit must be between 1 and 100", http.StatusBadRequest)
				return
			}
			limit = parsedLimit
		}

		sortBy := r.URL.Query().Get("sortBy")
		if sortBy == "" {
			sortBy = "id"
		}

		if sortBy != "id" && sortBy != "name" && sortBy != "price" {
			http.Error(w, "sortBy must be id, name, or price", http.StatusBadRequest)
			return
		}

		order := strings.ToLower(r.URL.Query().Get("order"))
		if order == "" {
			order = "asc"
		}

		if order != "asc" && order != "desc" {
			http.Error(w, "order must be asc or desc", http.StatusBadRequest)
			return
		}

		query := `
			SELECT id, sku, name, price, quantity
			FROM products
			WHERE ($1 = '' OR name ILIKE '%' || $1 || '%' OR sku ILIKE '%' || $1 || '%')
			ORDER BY
				CASE WHEN $2 = 'id' AND $3 = 'asc' THEN id END ASC,
				CASE WHEN $2 = 'id' AND $3 = 'desc' THEN id END DESC,
				CASE WHEN $2 = 'name' AND $3 = 'asc' THEN name END ASC,
				CASE WHEN $2 = 'name' AND $3 = 'desc' THEN name END DESC,
				CASE WHEN $2 = 'price' AND $3 = 'asc' THEN price END ASC,
				CASE WHEN $2 = 'price' AND $3 = 'desc' THEN price END DESC,
				id ASC
			LIMIT $4 OFFSET $5
		`

		rows, err := db.QueryContext(
			r.Context(),
			query,
			q,
			sortBy,
			order,
			limit,
			(page-1)*limit,
		)

		if err != nil {
			log.Printf("failed to query products: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return

			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		products := make([]Product, 0)

		for rows.Next() {
			var p Product

			if err := rows.Scan(
				&p.ID,
				&p.SKU,
				&p.Name,
				&p.Price,
				&p.Quantity,
			); err != nil {
				log.Printf("failed to scan product: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			products = append(products, p)
		}

		if err := rows.Err(); err != nil {
			log.Printf("failed to iterate products: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(products); err != nil {
			log.Printf("failed to encode products: %v", err)
		}
	}
}

func createProductHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Giới hạn request body tối đa 1 MB.
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		var req CreateProductRequest

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "Invalid JSON request", http.StatusBadRequest)
			return
		}

		// Không chấp nhận nhiều JSON object trong cùng request.
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			http.Error(w, "Request must contain one JSON object", http.StatusBadRequest)
			return
		}

		req.SKU = strings.TrimSpace(req.SKU)
		req.Name = strings.TrimSpace(req.Name)

		if req.SKU == "" || req.Name == "" {
			http.Error(w, "SKU and name are required", http.StatusBadRequest)
			return
		}

		if len(req.SKU) > 50 || len(req.Name) > 200 {
			http.Error(w, "SKU or name exceeds maximum length", http.StatusBadRequest)
			return
		}

		if req.Price < 0 || req.Quantity < 0 {
			http.Error(w, "Price and quantity must be non-negative", http.StatusBadRequest)
			return
		}

		var product Product

		err := db.QueryRowContext(
			r.Context(),
			`INSERT INTO products (sku, name, price, quantity)
             VALUES ($1, $2, $3, $4)
             RETURNING id, sku, name, price, quantity`,
			req.SKU,
			req.Name,
			req.Price,
			req.Quantity,
		).Scan(
			&product.ID,
			&product.SKU,
			&product.Name,
			&product.Price,
			&product.Quantity,
		)

		if err != nil {
			var pgErr *pgconn.PgError

			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				http.Error(w, "SKU already exists", http.StatusConflict)
				return
			}

			log.Printf("failed to create product: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(w).Encode(product); err != nil {
			log.Printf("failed to encode created product: %v", err)
		}
	}

}
func stockInHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req StockRequest

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		if req.Quantity <= 0 {
			http.Error(w, "quantity must be greater than 0", http.StatusBadRequest)
			return
		}

		idValue := strings.TrimPrefix(
			r.URL.Path,
			"/api/v1/products/",
		)
		idValue = strings.TrimSuffix(idValue, "/stock-in")

		id, err := strconv.ParseInt(idValue, 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "Invalid product ID", http.StatusBadRequest)
			return
		}

		var product Product

		err = db.QueryRowContext(
			r.Context(),
			`UPDATE products
			 SET quantity = quantity + $1,
			     updated_at = NOW()
			 WHERE id = $2
			 RETURNING id, sku, name, price, quantity`,
			req.Quantity,
			id,
		).Scan(
			&product.ID,
			&product.SKU,
			&product.Name,
			&product.Price,
			&product.Quantity,
		)

		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Product not found", http.StatusNotFound)
			return
		}

		if err != nil {
			log.Printf("stock-in failed: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(product)
	}
}

func stockOutHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req StockRequest

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		if req.Quantity <= 0 {
			http.Error(w, "quantity must be greater than 0", http.StatusBadRequest)
			return
		}

		idValue := strings.TrimPrefix(
			r.URL.Path,
			"/api/v1/products/",
		)
		idValue = strings.TrimSuffix(idValue, "/stock-out")

		id, err := strconv.ParseInt(idValue, 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "Invalid product ID", http.StatusBadRequest)
			return
		}

		var product Product

		err = db.QueryRowContext(
			r.Context(),
			`UPDATE products
			 SET quantity = quantity - $1,
			     updated_at = NOW()
			 WHERE id = $2
			   AND quantity >= $1
			 RETURNING id, sku, name, price, quantity`,
			req.Quantity,
			id,
		).Scan(
			&product.ID,
			&product.SKU,
			&product.Name,
			&product.Price,
			&product.Quantity,
		)

		if errors.Is(err, sql.ErrNoRows) {
			var exists bool

			checkErr := db.QueryRowContext(
				r.Context(),
				`SELECT EXISTS(SELECT 1 FROM products WHERE id = $1)`,
				id,
			).Scan(&exists)

			if checkErr != nil {
				log.Printf("stock-out existence check failed: %v", checkErr)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			if !exists {
				http.Error(w, "Product not found", http.StatusNotFound)
				return
			}

			http.Error(w, "Insufficient stock", http.StatusConflict)
			return
		}

		if err != nil {
			log.Printf("stock-out failed: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(product)
	}
}
