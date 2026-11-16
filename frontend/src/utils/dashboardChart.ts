import type {
  StockMovementChart,
  StockMovementChartPoint,
  StockMovementChartTotals,
  StockMovementGranularity,
} from '../types/dashboard';

// Prepares the stock movement chart response for display, without React or
// Recharts. The backend owns the figures: every bucket is passed through
// unchanged (same order, period, stock_in and stock_out; stock_out stays a
// positive quantity), and the totals are the backend's, never recomputed
// from the points. Labels are formatted by the chart, not here.

// empty: no bucket at all. all_zero: buckets, but nothing moved in or out
// (per the backend totals). data: anything else.
export type ChartDisplayState = 'empty' | 'all_zero' | 'data';

// One bucket, with the backend field names so the chart can read them as is.
export type StockMovementChartRow = Readonly<StockMovementChartPoint>;

export interface StockMovementChartModel {
  readonly state: ChartDisplayState;
  readonly granularity: StockMovementGranularity;
  // The requested range and time zone, as returned by the backend.
  readonly from: string;
  readonly to: string;
  readonly timezone: string;
  readonly rows: readonly StockMovementChartRow[];
  readonly totals: Readonly<StockMovementChartTotals>;
}

/**
 * Decides what the chart shows, from the number of buckets and the backend
 * totals only. Buckets whose values contradict the totals do not change the
 * result: the totals are the official figures.
 */
export function getChartDisplayState(
  chart: Pick<StockMovementChart, 'points' | 'totals'>
): ChartDisplayState {
  if (chart.points.length === 0) {
    return 'empty';
  }
  if (chart.totals.stock_in === 0 && chart.totals.stock_out === 0) {
    return 'all_zero';
  }
  return 'data';
}

/**
 * Builds the display model of a chart response. The result is a new object
 * that shares nothing with the response, which is left untouched.
 */
export function toStockMovementChartModel(chart: StockMovementChart): StockMovementChartModel {
  return {
    state: getChartDisplayState(chart),
    granularity: chart.granularity,
    from: chart.from,
    to: chart.to,
    timezone: chart.timezone,
    rows: chart.points.map((point) => ({
      period: point.period,
      stock_in: point.stock_in,
      stock_out: point.stock_out,
    })),
    totals: {
      stock_in: chart.totals.stock_in,
      stock_out: chart.totals.stock_out,
    },
  };
}
