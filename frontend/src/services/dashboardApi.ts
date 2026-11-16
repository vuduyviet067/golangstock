import { request } from './client';
import { ApiError } from '../types/api';
import type {
  DashboardSummary,
  DashboardValidationFields,
  StockMovementChart,
  StockMovementChartQuery,
  StockMovementChartQueryField,
} from '../types/dashboard';
import { DASHBOARD_TIMEZONE } from '../utils/dashboardDates';

// Admin-only dashboard API. A user gets ApiError 403, an expired session
// ApiError 401 (after the client's silent refresh failed).

const DASHBOARD_BASE_URL = '/api/v1/dashboard';

const QUERY_FIELDS: readonly StockMovementChartQueryField[] = ['from', 'to', 'granularity', 'tz'];

export interface DashboardRequestOptions {
  signal?: AbortSignal;
}

/**
 * Thrown instead of the client's error when the caller's signal aborted the
 * request. The client reports an abort as ApiError with status 0, exactly
 * like a network failure, so the signal is what tells them apart.
 */
export class RequestAbortedError extends Error {
  readonly reason: unknown;

  constructor(reason?: unknown) {
    super('Request was aborted');
    this.name = 'AbortError';
    this.reason = reason;
    Object.setPrototypeOf(this, RequestAbortedError.prototype);
  }
}

/**
 * Reports whether a request was cancelled on purpose, as opposed to failing.
 * An ApiError with status 0 is a network failure and is not an abort.
 */
export function isAbortError(err: unknown): boolean {
  if (err instanceof RequestAbortedError) {
    return true;
  }
  return (
    typeof err === 'object' &&
    err !== null &&
    !(err instanceof ApiError) &&
    (err as { name?: unknown }).name === 'AbortError'
  );
}

async function getDashboard<T>(url: string, signal: AbortSignal | undefined): Promise<T> {
  let data: T;
  try {
    data = await request<T>(url, { method: 'GET', signal });
  } catch (err) {
    if (signal?.aborted) {
      throw new RequestAbortedError(signal.reason);
    }
    throw err;
  }
  // An abort while the body is read makes the client return null instead of
  // failing; the caller asked to drop the result either way.
  if (signal?.aborted) {
    throw new RequestAbortedError(signal.reason);
  }
  return data;
}

/**
 * Builds the query string (without "?") of GET /api/v1/dashboard/stock-movements.
 * URLSearchParams encodes "/" and "+" (a bare "+" would be read as a space).
 * Blank values are left out; tz defaults to Asia/Ho_Chi_Minh because the
 * backend would otherwise use UTC.
 */
export function buildStockMovementQuery(query: StockMovementChartQuery): string {
  const values: Record<StockMovementChartQueryField, string | undefined> = {
    from: query.from,
    to: query.to,
    granularity: query.granularity,
    tz: query.tz,
  };
  if (typeof values.tz !== 'string' || values.tz.trim() === '') {
    values.tz = DASHBOARD_TIMEZONE;
  }

  const params = new URLSearchParams();
  for (const field of QUERY_FIELDS) {
    const value = values[field];
    if (typeof value === 'string' && value.trim() !== '') {
      params.set(field, value);
    }
  }
  return params.toString();
}

/** GET /api/v1/dashboard/summary */
export function getDashboardSummary(options: DashboardRequestOptions = {}): Promise<DashboardSummary> {
  return getDashboard<DashboardSummary>(`${DASHBOARD_BASE_URL}/summary`, options.signal);
}

/** GET /api/v1/dashboard/stock-movements */
export function getStockMovementChart(
  query: StockMovementChartQuery,
  options: DashboardRequestOptions = {}
): Promise<StockMovementChart> {
  return getDashboard<StockMovementChart>(
    `${DASHBOARD_BASE_URL}/stock-movements?${buildStockMovementQuery(query)}`,
    options.signal
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/**
 * Returns the field messages of a 400 response
 * ({ data: { fields: { from?, to?, granularity?, tz? } } }), or null for any
 * other error.
 */
export function getValidationFields(err: unknown): DashboardValidationFields | null {
  if (!(err instanceof ApiError) || err.status !== 400) {
    return null;
  }
  const body = err.data;
  if (!isRecord(body) || !isRecord(body.data) || !isRecord(body.data.fields)) {
    return null;
  }
  const fields: DashboardValidationFields = {};
  let found = false;
  for (const field of QUERY_FIELDS) {
    const message = body.data.fields[field];
    if (typeof message === 'string') {
      fields[field] = message;
      found = true;
    }
  }
  return found ? fields : null;
}
