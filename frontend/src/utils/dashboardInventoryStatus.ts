import type { DashboardSummary, InventoryStatus } from '../types/dashboard';

// Prepares DashboardSummary.status_breakdown for the inventory status pie
// chart, without React or Recharts. The backend owns the figures: each
// status keeps its count and its place in the backend order (normal,
// low_stock, out_of_stock); nothing is summed, divided or inferred here.

// empty: no product at all (total_products is 0). data: anything else.
export type InventoryStatusPieState = 'empty' | 'data';

export const INVENTORY_STATUS_LABELS: Readonly<Record<InventoryStatus, string>> = {
  normal: 'Bình thường',
  low_stock: 'Sắp hết hàng',
  out_of_stock: 'Hết hàng',
};

// Design tokens from src/index.css, the same ones as the stock badges of the
// product table (badge-success, badge-warning, badge-danger).
export const INVENTORY_STATUS_COLOR_TOKENS = {
  normal: '--color-success',
  low_stock: '--color-warning',
  out_of_stock: '--color-danger',
} as const satisfies Readonly<Record<InventoryStatus, `--color-${string}`>>;

export type InventoryStatusColorToken = (typeof INVENTORY_STATUS_COLOR_TOKENS)[InventoryStatus];

export interface InventoryStatusSlice {
  readonly status: InventoryStatus;
  readonly label: string;
  // The backend count, unchanged.
  readonly count: number;
  readonly colorToken: InventoryStatusColorToken;
  // CSS value for a fill or a color, e.g. var(--color-success).
  readonly color: `var(${InventoryStatusColorToken})`;
}

export interface InventoryStatusPieModel {
  readonly state: InventoryStatusPieState;
  // The backend total, unchanged.
  readonly total_products: number;
  readonly slices: readonly InventoryStatusSlice[];
}

/** Decides what the pie shows, from the backend product total only. */
export function getInventoryStatusPieState(summary: Pick<DashboardSummary, 'total_products'>): InventoryStatusPieState {
  return summary.total_products === 0 ? 'empty' : 'data';
}

/**
 * Builds the display model of the status breakdown: one slice per backend
 * entry, in the backend order, with its label and colour. Zero counts are
 * kept (the chart decides how to draw them). The summary is left untouched.
 */
export function toInventoryStatusPieModel(
  summary: Pick<DashboardSummary, 'total_products' | 'status_breakdown'>
): InventoryStatusPieModel {
  return {
    state: getInventoryStatusPieState(summary),
    total_products: summary.total_products,
    slices: summary.status_breakdown.map(({ status, count }) => {
      const colorToken = INVENTORY_STATUS_COLOR_TOKENS[status];
      return {
        status,
        label: INVENTORY_STATUS_LABELS[status],
        count,
        colorToken,
        color: `var(${colorToken})` as const,
      };
    }),
  };
}
