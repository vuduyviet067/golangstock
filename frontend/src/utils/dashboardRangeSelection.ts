import { ApiError } from '../types/api';
import type { StockMovementChartQueryField } from '../types/dashboard';
import { getValidationFields } from '../services/dashboardApi';
import {
  CUSTOM_RANGE_ERROR_MESSAGES,
  MAX_DAY_DAYS,
  MAX_MONTH_DAYS,
  presetRange,
  validateCustomRange,
  type CustomRangeError,
  type DashboardDateRange,
  type DashboardPreset,
} from './dashboardDates';

// Which date range the stock movement chart shows. A selection holds the
// resolved range ({ from, to, granularity }), computed once when it is made:
// it does not move at midnight. "today" is always passed in (normally
// todayIn(DASHBOARD_TIMEZONE)), so these helpers never read the clock. The
// ranges themselves come from dashboardDates (presetRange,
// validateCustomRange); nothing is recomputed here.

export type RangeSelection =
  | { readonly kind: 'preset'; readonly preset: DashboardPreset; readonly range: Readonly<DashboardDateRange> }
  | { readonly kind: 'custom'; readonly range: Readonly<DashboardDateRange> };

export const DEFAULT_RANGE_PRESET: DashboardPreset = '30d';

/** The range of a preset ending today (90d by day, 12mo by month). */
export function selectPreset(preset: DashboardPreset, today: string): RangeSelection {
  return { kind: 'preset', preset, range: presetRange(preset, today) };
}

/** The initial selection: the last 30 days. */
export function defaultRangeSelection(today: string): RangeSelection {
  return selectPreset(DEFAULT_RANGE_PRESET, today);
}

export type CustomRangeSelectionResult =
  | { readonly ok: true; readonly selection: RangeSelection }
  | { readonly ok: false; readonly error: CustomRangeError; readonly message: string };

/**
 * Checks a custom range picked by the user (validateCustomRange) and, when
 * valid, selects it with granularity auto; otherwise returns the error code
 * and its Vietnamese message.
 */
export function applyCustomRange(from: string, to: string, today: string): CustomRangeSelectionResult {
  const result = validateCustomRange(from, to, today);
  if (!result.ok) {
    return { ok: false, error: result.error, message: CUSTOM_RANGE_ERROR_MESSAGES[result.error] };
  }
  return { ok: true, selection: { kind: 'custom', range: result.range } };
}

// Backend validation errors (400, data.fields) -----------------------------

// The field messages of backend/internal/handle/http/error_mapper.go, in
// Vietnamese. Only these confirmed (field, message) pairs are translated.
// Maps, not objects: a message such as "toString" must not hit a prototype.
const KNOWN_FIELD_MESSAGES: Readonly<Record<StockMovementChartQueryField, ReadonlyMap<string, string>>> = {
  from: new Map([
    ['must be a date in YYYY-MM-DD format', 'Ngày bắt đầu không hợp lệ.'],
    ['must be on or before to', CUSTOM_RANGE_ERROR_MESSAGES.reversed],
  ]),
  to: new Map([
    ['must be a date in YYYY-MM-DD format', 'Ngày kết thúc không hợp lệ.'],
    [
      'range must be at most 92 days by day or 731 days by month',
      `Khoảng thời gian quá dài: tối đa ${MAX_DAY_DAYS} ngày khi xem theo ngày hoặc ${MAX_MONTH_DAYS} ngày khi xem theo tháng.`,
    ],
  ]),
  granularity: new Map([['must be auto, day or month', 'Cách gộp dữ liệu không hợp lệ.']]),
  tz: new Map([['must be an IANA time zone name', 'Múi giờ không hợp lệ.']]),
};

// A known field with a message this version does not know.
const FIELD_FALLBACK_MESSAGES: Readonly<Record<StockMovementChartQueryField, string>> = {
  from: 'Ngày bắt đầu không được máy chủ chấp nhận.',
  to: 'Ngày kết thúc không được máy chủ chấp nhận.',
  granularity: 'Cách gộp dữ liệu không được máy chủ chấp nhận.',
  tz: 'Múi giờ không được máy chủ chấp nhận.',
};

// A 400 without any field this version knows.
export const GENERIC_RANGE_ERROR_MESSAGE = 'Máy chủ từ chối khoảng thời gian đã chọn.';

const FIELD_ORDER: readonly StockMovementChartQueryField[] = ['from', 'to', 'granularity', 'tz'];

export interface RangeErrorMessage {
  // The query parameter concerned, or null for the generic message.
  readonly field: StockMovementChartQueryField | null;
  readonly message: string;
}

/**
 * Vietnamese messages for a 400 response of the chart endpoint, one per
 * field in the order from, to, granularity, tz. Unknown messages fall back
 * to a message for their field, and a 400 without a known field to a
 * generic message; the backend's English text is never shown. Returns null
 * for any other error (handled by the dashboard error notice).
 */
export function describeRangeValidationError(err: unknown): readonly RangeErrorMessage[] | null {
  if (!(err instanceof ApiError) || err.status !== 400) {
    return null;
  }
  const fields = getValidationFields(err);
  const messages: RangeErrorMessage[] = [];
  for (const field of FIELD_ORDER) {
    const text = fields?.[field];
    if (text !== undefined) {
      messages.push({ field, message: KNOWN_FIELD_MESSAGES[field].get(text) ?? FIELD_FALLBACK_MESSAGES[field] });
    }
  }
  return messages.length > 0 ? messages : [{ field: null, message: GENERIC_RANGE_ERROR_MESSAGE }];
}
