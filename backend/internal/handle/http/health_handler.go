package http

import (
	"net/http"

	"gostock/internal/handle/http/response"
)

// HealthHandler is the liveness probe (GET /health). It reports only that the
// HTTP process answers; it does not touch PostgreSQL and says nothing about
// it. Database availability is the readiness probe's job (GET /ready).
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	response.Success(w, http.StatusOK, "health check passed", map[string]string{
		"status": "ok",
		"app":    "GoStock",
	})
}
