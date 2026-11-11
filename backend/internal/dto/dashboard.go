package dto

type DashboardSummaryResponse struct {
	TotalProducts      int64 `json:"total_products"`
	TotalStock         int64 `json:"total_stock"`
	LowStockProducts   int64 `json:"low_stock_products"`
	OutOfStockProducts int64 `json:"out_of_stock_products"`

	StatusBreakdown []InventoryStatusCount `json:"status_breakdown"`
}

type InventoryStatusCount struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

type StockMovementChartQuery struct {
	From        string
	To          string
	Granularity string
	Timezone    string
}

type StockMovementChartResponse struct {
	From        string                    `json:"from"`
	To          string                    `json:"to"`
	Granularity string                    `json:"granularity"`
	Timezone    string                    `json:"timezone"`
	Points      []StockMovementChartPoint `json:"points"`
	Totals      StockMovementChartTotals  `json:"totals"`
}

type StockMovementChartPoint struct {
	Period   string `json:"period"`
	StockIn  int64  `json:"stock_in"`
	StockOut int64  `json:"stock_out"`
}

type StockMovementChartTotals struct {
	StockIn  int64 `json:"stock_in"`
	StockOut int64 `json:"stock_out"`
}
