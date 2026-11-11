package http

import (
	"net/http"

	"gostock/internal/domain"
	"gostock/internal/dto"
	"gostock/internal/handle/http/response"
	"gostock/internal/usecase"
)

// DashboardHandler serves the read-only dashboard figures.
type DashboardHandler struct {
	useCase *usecase.DashboardUseCase
}

func NewDashboardHandler(uc *usecase.DashboardUseCase) *DashboardHandler {
	return &DashboardHandler{
		useCase: uc,
	}
}

func toDashboardSummaryResponse(s domain.InventorySummary) dto.DashboardSummaryResponse {
	return dto.DashboardSummaryResponse{
		TotalProducts:      s.TotalProducts,
		TotalStock:         s.TotalStock,
		LowStockProducts:   s.LowStock,
		OutOfStockProducts: s.OutOfStock,
		StatusBreakdown: []dto.InventoryStatusCount{
			{Status: usecase.InventoryStatusNormal, Count: s.Normal},
			{Status: usecase.InventoryStatusLowStock, Count: s.LowStock},
			{Status: usecase.InventoryStatusOutOfStock, Count: s.OutOfStock},
		},
	}
}

func toStockMovementChartResponse(c domain.StockMovementChart) dto.StockMovementChartResponse {
	points := make([]dto.StockMovementChartPoint, len(c.Points))
	for i, p := range c.Points {
		points[i] = dto.StockMovementChartPoint{
			Period:   p.Period,
			StockIn:  p.StockIn,
			StockOut: p.StockOut,
		}
	}
	return dto.StockMovementChartResponse{
		From:        c.From,
		To:          c.To,
		Granularity: c.Granularity,
		Timezone:    c.Timezone,
		Points:      points,
		Totals: dto.StockMovementChartTotals{
			StockIn:  c.TotalStockIn,
			StockOut: c.TotalStockOut,
		},
	}
}

// Summary serves GET /api/v1/dashboard/summary.
func (h *DashboardHandler) Summary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	summary, err := h.useCase.Summary(r.Context())
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "Dashboard summary retrieved successfully", toDashboardSummaryResponse(summary))
}

// StockMovementChart serves GET /api/v1/dashboard/stock-movements with the
// optional query parameters from, to, granularity and tz.
func (h *DashboardHandler) StockMovementChart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := r.URL.Query()
	chart, err := h.useCase.StockMovementChart(r.Context(), dto.StockMovementChartQuery{
		From:        query.Get("from"),
		To:          query.Get("to"),
		Granularity: query.Get("granularity"),
		Timezone:    query.Get("tz"),
	})
	if err != nil {
		WriteError(w, r, err)
		return
	}

	response.Success(w, http.StatusOK, "Stock movement chart retrieved successfully", toStockMovementChartResponse(chart))
}
