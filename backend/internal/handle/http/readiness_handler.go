package http

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"gostock/internal/handle/http/response"
)

type ReadinessHandler struct {
	db *sql.DB
}

func NewReadinessHandler(db *sql.DB) *ReadinessHandler {
	return &ReadinessHandler{db: db}
}

func (h *ReadinessHandler) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.PingContext(ctx); err != nil {
		response.Error(w, http.StatusServiceUnavailable, "database disconnected")
		return
	}

	response.Success(w, http.StatusOK, "service is ready", map[string]string{
		"status":   "ready",
		"database": "connected",
	})
}
