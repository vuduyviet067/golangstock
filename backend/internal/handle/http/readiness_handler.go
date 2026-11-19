package http

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"gostock/pkg/response"
)

// readinessPingTimeout bounds the database ping of one readiness check.
const readinessPingTimeout = 2 * time.Second

type ReadinessHandler struct {
	db *sql.DB
}

func NewReadinessHandler(db *sql.DB) *ReadinessHandler {
	return &ReadinessHandler{db: db}
}

// ServeHTTP is the readiness probe (GET /ready): it answers 200 when
// PostgreSQL responds to a ping within readinessPingTimeout and 503 otherwise.
// Only GET is served; other methods get 405 before the database is touched.
// A failed ping is logged on the server; the client only sees the generic
// 503 body.
func (h *ReadinessHandler) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.Error(w, http.StatusMethodNotAllowed, response.CodeMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), readinessPingTimeout)
	defer cancel()

	if err := h.db.PingContext(ctx); err != nil {
		logPingFailure(r, ctx, err)
		response.Error(w, http.StatusServiceUnavailable, response.CodeServiceUnavailable, "database disconnected")
		return
	}

	response.Success(w, http.StatusOK, "service is ready", map[string]string{
		"status":   "ready",
		"database": "connected",
	})
}

// logPingFailure logs a failed readiness ping with the request ID (when the
// client sent a valid X-Request-ID), a reason and, for a PostgreSQL server
// error, its SQLSTATE. The driver's error text is never logged: pgconn's
// connection errors name the user, database and host. This follows
// internal/platform/database preflightQueryError.
func logPingFailure(r *http.Request, ctx context.Context, err error) {
	var attrs []any
	if reqID := extractRequestID(r); reqID != "" {
		attrs = append(attrs, slog.String("request_id", reqID))
	}
	attrs = append(attrs, slog.String("reason", pingFailureReason(ctx, err)))

	// pgconn.PgError exposes its SQLSTATE through this method; matching the
	// method keeps the HTTP layer free of a driver import.
	var withState interface{ SQLState() string }
	if errors.As(err, &withState) && withState.SQLState() != "" {
		attrs = append(attrs, slog.String("sqlstate", withState.SQLState()))
	}

	slog.ErrorContext(r.Context(), "readiness check failed: database ping", attrs...)
}

// pingFailureReason classifies a failed ping: the timeout expired, the
// request was cancelled, or the database is otherwise unavailable.
func pingFailureReason(ctx context.Context, err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
		return "canceled"
	default:
		return "unavailable"
	}
}
