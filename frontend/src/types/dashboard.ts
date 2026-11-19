// Types for the admin-only dashboard API:
//   GET /api/v1/dashboard/summary
//   GET /api/v1/dashboard/stock-movements
// They mirror backend/internal/dto/dashboard.go. Counts and quantities are
// int64 on the backend; they stay far below Number.MAX_SAFE_INTEGER.

// Every product is in exactly one status: normal (quantity > minimum_stock),
// low_stock (0 < quantity <= minimum_stock) or out_of_stock (quantity = 0).
export type InventoryStatus = 'normal' | 'low_stock' | 'out_of_stock';

export interface InventoryStatusCount {
  status: InventoryStatus;
  count: number;
}

export interface DashboardSummary {
  total_products: number;
  total_stock: number;
  low_stock_products: number;
  out_of_stock_products: number;
  // Always the three statuses, in the order normal, low_stock, out_of_stock.
  status_breakdown: InventoryStatusCount[];
}

// Granularity of the chart returned by the backend.
export type StockMovementGranularity = 'day' | 'month';

// Granularity requested by the client. auto groups by day for ranges up to
// 62 days and by month beyond.
export type StockMovementGranularityParam = 'auto' | StockMovementGranularity;

export interface StockMovementChartPoint {
  // YYYY-MM-DD: the day, or the first day of the month.
  period: string;
  stock_in: number;
  // Positive quantity.
  stock_out: number;
}

export interface StockMovementChartTotals {
  stock_in: number;
  stock_out: number;
}

export interface StockMovementChart {
  // The requested range (YYYY-MM-DD, both days included), even when
  // granularity is month.
  from: string;
  to: string;
  granularity: StockMovementGranularity;
  timezone: string;
  // One point per day or month of the range, zero-filled.
  points: StockMovementChartPoint[];
  totals: StockMovementChartTotals;
}

export interface StockMovementChartQuery {
  // Calendar dates in YYYY-MM-DD format, both included.
  from: string;
  to: string;
  granularity: StockMovementGranularityParam;
  // IANA time zone name. The service sends Asia/Ho_Chi_Minh when omitted,
  // because the backend would otherwise use UTC.
  tz?: string;
}

export type StockMovementChartQueryField = 'from' | 'to' | 'granularity' | 'tz';

// data.fields of a 400 response: one message per invalid query parameter.
// A reversed range is reported on from, a range that is too long on to.
export type DashboardValidationFields = Partial<Record<StockMovementChartQueryField, string>>;
