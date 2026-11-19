package mapper

import (
	"gostock/internal/domain"
	"gostock/internal/dto"
)

// ToDashboardSummaryResponse maps the inventory summary. The status breakdown
// always lists normal, low_stock and out_of_stock, in that order, even when a
// count is 0.
func ToDashboardSummaryResponse(s domain.InventorySummary) dto.DashboardSummaryResponse {
	return dto.DashboardSummaryResponse{
		TotalProducts:      s.TotalProducts,
		TotalStock:         s.TotalStock,
		LowStockProducts:   s.LowStock,
		OutOfStockProducts: s.OutOfStock,
		StatusBreakdown: []dto.InventoryStatusCount{
			{Status: domain.InventoryStatusNormal, Count: s.Normal},
			{Status: domain.InventoryStatusLowStock, Count: s.LowStock},
			{Status: domain.InventoryStatusOutOfStock, Count: s.OutOfStock},
		},
	}
}

// ToStockMovementChartResponse maps the stock movement chart, keeping the
// point order. Points is never nil, so a chart without points is serialized
// as [].
func ToStockMovementChartResponse(c domain.StockMovementChart) dto.StockMovementChartResponse {
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
