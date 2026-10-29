package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":   "not_ready",
			"database": "disconnected",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":   "ready",
		"database": "connected",
	})
}
