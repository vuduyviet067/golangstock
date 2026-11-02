package http

import (
	"net/http"

	"gostock/internal/handle/http/response"
)

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	response.Success(w, http.StatusOK, "health check passed", map[string]string{
		"status":   "ok",
		"app":      "GoStock",
		"database": "connected",
	})
}
