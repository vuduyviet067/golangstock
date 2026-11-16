package http

import (
	"net/http"
	"strconv"
	"strings"

	"gostock/internal/handle/http/response"
	"gostock/internal/usecase"
)

// StockMovementHandler serves the read-only stock movement history.
type StockMovementHandler struct {
	useCase *usecase.StockMovementUseCase
}

func NewStockMovementHandler(uc *usecase.StockMovementUseCase) *StockMovementHandler {
	return &StockMovementHandler{
		useCase: uc,
	}
}

// parsePagination reads the page and limit query parameters with the same
// defaults (1 and 10) and messages as the product and user lists. Range
// checks are done by the use case. It writes 400 and returns false when a
// value is not an integer.
func parsePagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	query := r.URL.Query()
	page := 1
	limit := 10

	if rawPage := query.Get("page"); rawPage != "" {
		parsed, err := strconv.Atoi(rawPage)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid page")
			return 0, 0, false
		}
		page = parsed
	}

	if rawLimit := query.Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid limit")
			return 0, 0, false
		}
		limit = parsed
	}

	return page, limit, true
}

// ListByProduct serves GET /api/v1/products/{id}/movements.
func (h *StockMovementHandler) ListByProduct(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rawID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	rawID = strings.TrimSuffix(rawID, "/movements")
	if rawID == "" || strings.Contains(rawID, "/") {
		response.Error(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid product ID")
		return
	}

	page, limit, ok := parsePagination(w, r)
	if !ok {
		return
	}

	movements, total, err := h.useCase.ListByProduct(r.Context(), id, page, limit)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Paginated(w, "Stock movements retrieved successfully", movements, page, limit, total)
}

// List serves GET /api/v1/stock-movements. The router restricts it to admins.
func (h *StockMovementHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	page, limit, ok := parsePagination(w, r)
	if !ok {
		return
	}

	movements, total, err := h.useCase.List(r.Context(), page, limit)
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Paginated(w, "Stock movements retrieved successfully", movements, page, limit, total)
}
