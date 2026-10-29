package http

import (
	"encoding/json"
	"log"
	"net/http"
)

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(
			w,
			"Method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	response := struct {
		Status   string `json:"status"`
		App      string `json:"app"`
		Database string `json:"database"`
	}{
		Status:   "ok",
		App:      "GoStock",
		Database: "connected",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("failed to write health response: %v", err)
	}
}
