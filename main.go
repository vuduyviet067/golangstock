package main

import (
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")

	db, err := openDatabase(dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	log.Println("Connected to PostgreSQL successfully")

	mux := http.NewServeMux()

	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/api/v1/products", productsHandler(db))

	mux.HandleFunc("/api/v1/products/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stock-in") {
			stockInHandler(db)(w, r)
			return
		}

		if strings.HasSuffix(r.URL.Path, "/stock-out") {
			stockOutHandler(db)(w, r)
			return
		}

		productsHandler(db)(w, r)
	})

	log.Println("GoStock is running at http://localhost:8080")

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
