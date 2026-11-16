import type { StockMovementGranularityParam } from '../types/dashboard';

// Calendar date helpers for the dashboard. Dates are YYYY-MM-DD strings.
// Arithmetic runs on UTC midnights (Date.UTC and getUTC* only), so it never
// depends on the time zone of the machine or on daylight saving time. Only
// todayIn looks at a time zone, to find the current local date.

// The dashboard time zone. Always send this constant: Intl may report the
// same zone under another name (Node 22 resolves it to Asia/Saigon).
export const DASHBOARD_TIMEZONE = 'Asia/Ho_Chi_Minh';

// Backend limits (backend/internal/usecase/dashboard.go). The backend stays
// the authority; these let the UI reject a range before sending it.
export const MAX_AUTO_DAY_DAYS = 62;
export const MAX_DAY_DAYS = 92;
export const MAX_MONTH_DAYS = 731;

const MIN_YEAR = 1000;
const MAX_YEAR = 9999;
const MS_PER_DAY = 86_400_000;
const DATE_PATTERN = /^(\d{4})-(\d{2})-(\d{2})$/;

export interface CalendarDateParts {
  year: number;
  // 1 to 12.
  month: number;
  day: number;
}

function daysInMonth(year: number, month: number): number {
  return new Date(Date.UTC(year, month, 0)).getUTCDate();
}

function pad(value: number, length: number): string {
  return String(value).padStart(length, '0');
}

function formatParts({ year, month, day }: CalendarDateParts): string {
  if (!(year >= MIN_YEAR && year <= MAX_YEAR)) {
    throw new RangeError(`date out of range: year ${year}`);
  }
  return `${pad(year, 4)}-${pad(month, 2)}-${pad(day, 2)}`;
}

/**
 * Parses a YYYY-MM-DD calendar date. Returns null unless the string is
 * exactly that format and names a real day of a year from 1000 to 9999
 * (Date.UTC reads years 0-99 as 1900-1999).
 */
export function parseCalendarDate(value: string): CalendarDateParts | null {
  if (typeof value !== 'string') {
    return null;
  }
  const match = DATE_PATTERN.exec(value);
  if (!match) {
    return null;
  }
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (year < MIN_YEAR || year > MAX_YEAR || month < 1 || month > 12) {
    return null;
  }
  if (day < 1 || day > daysInMonth(year, month)) {
    return null;
  }
  return { year, month, day };
}

export function isCalendarDate(value: string): boolean {
  return parseCalendarDate(value) !== null;
}

function requireDate(value: string): CalendarDateParts {
  const parts = parseCalendarDate(value);
  if (!parts) {
    throw new RangeError(`invalid calendar date: ${JSON.stringify(value)}`);
  }
  return parts;
}

function requireInteger(value: number, name: string): void {
  if (!Number.isInteger(value)) {
    throw new RangeError(`${name} must be an integer: ${value}`);
  }
}

function toEpochDay(parts: CalendarDateParts): number {
  return Date.UTC(parts.year, parts.month - 1, parts.day) / MS_PER_DAY;
}

function fromEpochDay(epochDay: number): string {
  const date = new Date(epochDay * MS_PER_DAY);
  return formatParts({
    year: date.getUTCFullYear(),
    month: date.getUTCMonth() + 1,
    day: date.getUTCDate(),
  });
}

/**
 * Returns the current date in the given time zone as YYYY-MM-DD. It is
 * computed on every call, so it follows midnight.
 */
export function todayIn(timeZone: string = DASHBOARD_TIMEZONE, now: Date = new Date()): string {
  const formatter = new Intl.DateTimeFormat('en-US-u-ca-gregory-nu-latn', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  });
  const values: Partial<Record<'year' | 'month' | 'day', number>> = {};
  for (const part of formatter.formatToParts(now)) {
    if (part.type === 'year' || part.type === 'month' || part.type === 'day') {
      values[part.type] = Number(part.value);
    }
  }
  const result = formatParts({
    year: values.year ?? NaN,
    month: values.month ?? NaN,
    day: values.day ?? NaN,
  });
  requireDate(result);
  return result;
}

/** Adds a number of days (negative to subtract). */
export function addDays(date: string, days: number): string {
  requireInteger(days, 'days');
  return fromEpochDay(toEpochDay(requireDate(date)) + days);
}

/**
 * Adds a number of months (negative to subtract). A day that does not exist
 * in the target month becomes its last day: 2026-01-31 + 1 month is
 * 2026-02-28.
 */
export function addMonths(date: string, months: number): string {
  requireInteger(months, 'months');
  const { year, month, day } = requireDate(date);
  const index = year * 12 + (month - 1) + months;
  const targetYear = Math.floor(index / 12);
  const targetMonth = index - targetYear * 12 + 1;
  if (targetYear < MIN_YEAR || targetYear > MAX_YEAR) {
    throw new RangeError(`date out of range: year ${targetYear}`);
  }
  return formatParts({
    year: targetYear,
    month: targetMonth,
    day: Math.min(day, daysInMonth(targetYear, targetMonth)),
  });
}

/** Returns the first day of the date's month. */
export function startOfMonth(date: string): string {
  const { year, month } = requireDate(date);
  return formatParts({ year, month, day: 1 });
}

/**
 * Returns the number of days from from to to, both included: 1 for the same
 * day, 0 or less when from is after to.
 */
export function daysInclusive(from: string, to: string): number {
  return toEpochDay(requireDate(to)) - toEpochDay(requireDate(from)) + 1;
}

export interface DashboardDateRange {
  from: string;
  to: string;
  granularity: StockMovementGranularityParam;
}

export type DashboardPreset = '7d' | '30d' | '90d' | '12mo';

export const DASHBOARD_PRESETS: readonly DashboardPreset[] = ['7d', '30d', '90d', '12mo'];

/**
 * Returns the range of a preset ending today. The day presets send
 * granularity=day explicitly: with auto, the backend would group 90 days by
 * month. 12mo starts on the first day of the month eleven months ago.
 */
export function presetRange(preset: DashboardPreset, today: string): DashboardDateRange {
  requireDate(today);
  switch (preset) {
    case '7d':
      return { from: addDays(today, -6), to: today, granularity: 'day' };
    case '30d':
      return { from: addDays(today, -29), to: today, granularity: 'day' };
    case '90d':
      return { from: addDays(today, -89), to: today, granularity: 'day' };
    case '12mo':
      return { from: addMonths(startOfMonth(today), -11), to: today, granularity: 'month' };
    default: {
      const unknown: never = preset;
      throw new RangeError(`unknown dashboard preset: ${String(unknown)}`);
    }
  }
}

export type CustomRangeError = 'invalid_date' | 'reversed' | 'future' | 'too_long';

export type CustomRangeResult =
  | { ok: true; range: DashboardDateRange }
  | { ok: false; error: CustomRangeError };

/**
 * Checks a custom range picked by the user. Errors, in this order:
 * invalid_date (from or to is not a real YYYY-MM-DD date), reversed (from is
 * after to), future (to is after today), too_long (more than
 * MAX_MONTH_DAYS days, the backend limit for auto granularity). A valid
 * range uses granularity=auto.
 */
export function validateCustomRange(from: string, to: string, today: string): CustomRangeResult {
  requireDate(today);
  if (!isCalendarDate(from) || !isCalendarDate(to)) {
    return { ok: false, error: 'invalid_date' };
  }
  const days = daysInclusive(from, to);
  if (days < 1) {
    return { ok: false, error: 'reversed' };
  }
  const daysAfterToday = daysInclusive(today, to) - 1;
  if (daysAfterToday > 0) {
    return { ok: false, error: 'future' };
  }
  if (days > MAX_MONTH_DAYS) {
    return { ok: false, error: 'too_long' };
  }
  return { ok: true, range: { from, to, granularity: 'auto' } };
}

export const CUSTOM_RANGE_ERROR_MESSAGES: Readonly<Record<CustomRangeError, string>> = {
  invalid_date: 'Ngày không hợp lệ. Vui lòng nhập ngày theo định dạng YYYY-MM-DD.',
  reversed: 'Ngày bắt đầu phải trước hoặc bằng ngày kết thúc.',
  future: 'Ngày kết thúc không được sau ngày hôm nay.',
  too_long: `Khoảng thời gian tối đa là ${MAX_MONTH_DAYS} ngày.`,
};
