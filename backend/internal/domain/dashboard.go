package domain

type InventorySummary struct {
	TotalProducts int64

	TotalStock int64
	Normal     int64
	LowStock   int64
	OutOfStock int64
}

type StockMovementBucket struct {
	Period string

	StockIn int64

	StockOut int64
}

type StockMovementChart struct {
	From        string
	To          string
	Granularity string
	Timezone    string

	Points        []StockMovementBucket
	TotalStockIn  int64
	TotalStockOut int64
}
