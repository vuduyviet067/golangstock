import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';
import {
  RequestAbortedError,
  buildStockMovementQuery,
  getDashboardSummary,
  getStockMovementChart,
  getValidationFields,
  isAbortError,
} from '../src/services/dashboardApi';
import { ApiError, type ApiResponse } from '../src/types/api';
import type {
  DashboardSummary,
  InventoryStatusCount,
  StockMovementChart,
  StockMovementChartQuery,
  StockMovementGranularityParam,
} from '../src/types/dashboard';
import { presetRange, validateCustomRange } from '../src/utils/dashboardDates';

// Fixtures: the exact bodies asserted by
// backend/internal/handle/http/dashboard_handler_test.go.
const SUMMARY_JSON =
  '{"success":true,"message":"Dashboard summary retrieved successfully","data":{' +
  '"total_products":50,"total_stock":12840,"low_stock_products":6,"out_of_stock_products":4,' +
  '"status_breakdown":[{"status":"normal","count":40},{"status":"low_stock","count":6},{"status":"out_of_stock","count":4}]}}';

const DAILY_CHART_JSON =
  '{"success":true,"message":"Stock movement chart retrieved successfully","data":{' +
  '"from":"2026-11-01","to":"2026-11-03","granularity":"day","timezone":"Asia/Ho_Chi_Minh",' +
  '"points":[{"period":"2026-11-01","stock_in":0,"stock_out":0},{"period":"2026-11-02","stock_in":120,"stock_out":45},{"period":"2026-11-03","stock_in":0,"stock_out":0}],' +
  '"totals":{"stock_in":120,"stock_out":45}}}';

const MONTHLY_CHART_JSON =
  '{"success":true,"message":"Stock movement chart retrieved successfully","data":{' +
  '"from":"2026-01-15","to":"2026-03-20","granularity":"month","timezone":"UTC",' +
  '"points":[{"period":"2026-01-01","stock_in":0,"stock_out":0},{"period":"2026-02-01","stock_in":0,"stock_out":0},{"period":"2026-03-01","stock_in":0,"stock_out":0}],' +
  '"totals":{"stock_in":0,"stock_out":0}}}';

const RANGE_TOO_LONG_JSON =
  '{"success":false,"message":"invalid query parameters","data":{"fields":{"to":"range must be at most 92 days by day or 731 days by month"}}}';
const RANGE_REVERSED_JSON =
  '{"success":false,"message":"invalid query parameters","data":{"fields":{"from":"must be on or before to"}}}';
const BAD_TZ_JSON =
  '{"success":false,"message":"invalid query parameters","data":{"fields":{"tz":"must be an IANA time zone name"}}}';
const UNAUTHENTICATED_JSON = '{"success":false,"message":"authentication required","data":null}';
const FORBIDDEN_JSON = '{"success":false,"message":"forbidden: admin role required","data":null}';
const INTERNAL_JSON = '{"success":false,"message":"Internal server error","data":null}';

// The typed fixtures must equal the backend bodies byte for byte, so the
// types are checked against the real contract.
const summaryFixture = {
  success: true,
  message: 'Dashboard summary retrieved successfully',
  data: {
    total_products: 50,
    total_stock: 12840,
    low_stock_products: 6,
    out_of_stock_products: 4,
    status_breakdown: [
      { status: 'normal', count: 40 },
      { status: 'low_stock', count: 6 },
      { status: 'out_of_stock', count: 4 },
    ],
  },
} satisfies ApiResponse<DashboardSummary>;

const dailyChartFixture = {
  success: true,
  message: 'Stock movement chart retrieved successfully',
  data: {
    from: '2026-11-01',
    to: '2026-11-03',
    granularity: 'day',
    timezone: 'Asia/Ho_Chi_Minh',
    points: [
      { period: '2026-11-01', stock_in: 0, stock_out: 0 },
      { period: '2026-11-02', stock_in: 120, stock_out: 45 },
      { period: '2026-11-03', stock_in: 0, stock_out: 0 },
    ],
    totals: { stock_in: 120, stock_out: 45 },
  },
} satisfies ApiResponse<StockMovementChart>;

const monthlyChartFixture = {
  success: true,
  message: 'Stock movement chart retrieved successfully',
  data: {
    from: '2026-01-15',
    to: '2026-03-20',
    granularity: 'month',
    timezone: 'UTC',
    points: [
      { period: '2026-01-01', stock_in: 0, stock_out: 0 },
      { period: '2026-02-01', stock_in: 0, stock_out: 0 },
      { period: '2026-03-01', stock_in: 0, stock_out: 0 },
    ],
    totals: { stock_in: 0, stock_out: 0 },
  },
} satisfies ApiResponse<StockMovementChart>;

// Values the types must reject (checked by tsc).
// @ts-expect-error 'low' is not an inventory status
const badStatus: InventoryStatusCount = { status: 'low', count: 1 };
// @ts-expect-error the backend never returns granularity auto
const badChartGranularity: StockMovementChart['granularity'] = 'auto';
// @ts-expect-error week is not a granularity
const badGranularityParam: StockMovementGranularityParam = 'week';
// @ts-expect-error from, to and granularity are required
const incompleteQuery: StockMovementChartQuery = { from: '2026-11-01' };

describe('types', () => {
  it('match the backend JSON byte for byte', () => {
    assert.equal(JSON.stringify(summaryFixture), SUMMARY_JSON);
    assert.equal(JSON.stringify(dailyChartFixture), DAILY_CHART_JSON);
    assert.equal(JSON.stringify(monthlyChartFixture), MONTHLY_CHART_JSON);
  });

  it('reject invalid values (see @ts-expect-error above)', () => {
    assert.equal([badStatus, badChartGranularity, badGranularityParam, incompleteQuery].length, 4);
  });
});

describe('buildStockMovementQuery', () => {
  it('sends from, to, granularity and tz in a fixed order, encoded', () => {
    assert.equal(
      buildStockMovementQuery({ from: '2026-11-01', to: '2026-11-30', granularity: 'day', tz: 'Asia/Ho_Chi_Minh' }),
      'from=2026-11-01&to=2026-11-30&granularity=day&tz=Asia%2FHo_Chi_Minh'
    );
  });

  it('always sends tz, Asia/Ho_Chi_Minh by default', () => {
    const expected = 'from=2026-11-01&to=2026-11-30&granularity=auto&tz=Asia%2FHo_Chi_Minh';
    assert.equal(buildStockMovementQuery({ from: '2026-11-01', to: '2026-11-30', granularity: 'auto' }), expected);
    assert.equal(buildStockMovementQuery({ from: '2026-11-01', to: '2026-11-30', granularity: 'auto', tz: '' }), expected);
    assert.equal(buildStockMovementQuery({ from: '2026-11-01', to: '2026-11-30', granularity: 'auto', tz: '  ' }), expected);
  });

  it('encodes "+" as %2B so the backend does not read a space', () => {
    const qs = buildStockMovementQuery({ from: '2026-11-01', to: '2026-11-01', granularity: 'day', tz: 'Etc/GMT+7' });
    assert.equal(qs, 'from=2026-11-01&to=2026-11-01&granularity=day&tz=Etc%2FGMT%2B7');
    assert.equal(new URLSearchParams(qs).get('tz'), 'Etc/GMT+7');
  });

  it('encodes characters that would change the query', () => {
    const qs = buildStockMovementQuery({ from: '2026-11-01&x=1', to: '2026-11-02', granularity: 'day' });
    assert.equal(new URLSearchParams(qs).get('from'), '2026-11-01&x=1');
    assert.equal(new URLSearchParams(qs).get('x'), null);
  });

  it('leaves out blank values', () => {
    assert.equal(
      buildStockMovementQuery({ from: '', to: ' ', granularity: 'auto' }),
      'granularity=auto&tz=Asia%2FHo_Chi_Minh'
    );
  });

  it('sends the presets and custom ranges as built by the date helpers', () => {
    assert.equal(
      buildStockMovementQuery(presetRange('90d', '2026-11-12')),
      'from=2026-08-15&to=2026-11-12&granularity=day&tz=Asia%2FHo_Chi_Minh'
    );
    assert.equal(
      buildStockMovementQuery(presetRange('12mo', '2026-11-12')),
      'from=2025-12-01&to=2026-11-12&granularity=month&tz=Asia%2FHo_Chi_Minh'
    );
    const custom = validateCustomRange('2026-01-01', '2026-11-12', '2026-11-12');
    assert.ok(custom.ok);
    assert.equal(
      buildStockMovementQuery(custom.range),
      'from=2026-01-01&to=2026-11-12&granularity=auto&tz=Asia%2FHo_Chi_Minh'
    );
  });
});

// fetch mock -----------------------------------------------------------------

interface FetchCall {
  url: string;
  init: RequestInit | undefined;
}

type Responder = (call: FetchCall) => Promise<Response>;

const globalWithFetch = globalThis as { fetch: typeof fetch };
const originalFetch = globalWithFetch.fetch;
let calls: FetchCall[] = [];
let responders: Responder[] = [];

function respondWith(...next: Responder[]) {
  responders = next;
}

function json(status: number, body: string): Responder {
  return async () => new Response(body, { status, headers: { 'Content-Type': 'application/json' } });
}

// Never answers; rejects like fetch when the request's signal aborts.
const hang: Responder = (call) =>
  new Promise<Response>((_, reject) => {
    const signal = call.init?.signal;
    if (!signal) {
      reject(new Error('test request has no signal'));
      return;
    }
    const abort = () => reject(new DOMException('This operation was aborted', 'AbortError'));
    if (signal.aborted) {
      abort();
    } else {
      signal.addEventListener('abort', abort, { once: true });
    }
  });

async function caught(promise: Promise<unknown>): Promise<unknown> {
  try {
    await promise;
  } catch (err) {
    return err;
  }
  throw new Error('expected the request to fail');
}

beforeEach(() => {
  calls = [];
  responders = [];
  globalWithFetch.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const call = { url: String(input), init };
    calls.push(call);
    const responder = responders.shift();
    if (!responder) {
      throw new Error(`unexpected fetch: ${call.url}`);
    }
    return responder(call);
  };
});

afterEach(() => {
  globalWithFetch.fetch = originalFetch;
});

function assertDashboardGet(call: FetchCall, url: string) {
  assert.equal(call.url, url);
  assert.equal(call.init?.method, 'GET');
  assert.equal(call.init?.credentials, 'include');
  assert.equal(call.init?.body, undefined);
  assert.equal((call.init?.headers as Record<string, string> | undefined)?.Accept, 'application/json');
}

describe('getDashboardSummary', () => {
  it('GETs /api/v1/dashboard/summary with cookies and returns data', async () => {
    respondWith(json(200, SUMMARY_JSON));
    const summary = await getDashboardSummary();
    assert.deepEqual(summary, summaryFixture.data);
    assert.equal(calls.length, 1);
    assertDashboardGet(calls[0], '/api/v1/dashboard/summary');
  });

  it('passes the signal to fetch', async () => {
    respondWith(json(200, SUMMARY_JSON));
    const controller = new AbortController();
    await getDashboardSummary({ signal: controller.signal });
    assert.equal(calls[0].init?.signal, controller.signal);
  });
});

describe('getStockMovementChart', () => {
  it('GETs /api/v1/dashboard/stock-movements with the encoded query and returns data', async () => {
    respondWith(json(200, DAILY_CHART_JSON));
    const chart = await getStockMovementChart({ from: '2026-11-01', to: '2026-11-03', granularity: 'day' });
    assert.deepEqual(chart, dailyChartFixture.data);
    assert.equal(calls.length, 1);
    assertDashboardGet(
      calls[0],
      '/api/v1/dashboard/stock-movements?from=2026-11-01&to=2026-11-03&granularity=day&tz=Asia%2FHo_Chi_Minh'
    );
  });

  it('returns a monthly chart as sent', async () => {
    respondWith(json(200, MONTHLY_CHART_JSON));
    const chart = await getStockMovementChart({ from: '2026-01-15', to: '2026-03-20', granularity: 'auto', tz: 'UTC' });
    assert.deepEqual(chart, monthlyChartFixture.data);
    assert.equal(calls[0].url, '/api/v1/dashboard/stock-movements?from=2026-01-15&to=2026-03-20&granularity=auto&tz=UTC');
  });
});

describe('errors', () => {
  const query: StockMovementChartQuery = { from: '2026-11-01', to: '2026-11-30', granularity: 'day' };

  it('400: keeps the status and exposes the field messages', async () => {
    respondWith(json(400, RANGE_TOO_LONG_JSON));
    const err = await caught(getStockMovementChart(query));
    assert.ok(err instanceof ApiError);
    assert.equal(err.status, 400);
    assert.equal(err.message, 'invalid query parameters');
    assert.deepEqual(getValidationFields(err), { to: 'range must be at most 92 days by day or 731 days by month' });
    assert.equal(isAbortError(err), false);
  });

  it('400: a reversed range is reported on from, a bad zone on tz', async () => {
    respondWith(json(400, RANGE_REVERSED_JSON), json(400, BAD_TZ_JSON));
    assert.deepEqual(getValidationFields(await caught(getStockMovementChart(query))), {
      from: 'must be on or before to',
    });
    assert.deepEqual(getValidationFields(await caught(getStockMovementChart(query))), {
      tz: 'must be an IANA time zone name',
    });
  });

  it('401: tries the silent refresh once, then keeps the status', async () => {
    respondWith(json(401, UNAUTHENTICATED_JSON), json(401, UNAUTHENTICATED_JSON));
    const err = await caught(getDashboardSummary());
    assert.ok(err instanceof ApiError);
    assert.equal(err.status, 401);
    assert.equal(err.message, 'authentication required');
    assert.equal(getValidationFields(err), null);
    assert.deepEqual(calls.map((c) => [c.init?.method, c.url]), [
      ['GET', '/api/v1/dashboard/summary'],
      ['POST', '/api/v1/auth/refresh'],
    ]);
  });

  it('401: after a successful refresh the request is retried with the same signal', async () => {
    respondWith(json(401, UNAUTHENTICATED_JSON), json(200, '{"success":true,"message":"ok","data":null}'), json(200, SUMMARY_JSON));
    const controller = new AbortController();
    const summary = await getDashboardSummary({ signal: controller.signal });
    assert.deepEqual(summary, summaryFixture.data);
    assert.deepEqual(calls.map((c) => c.url), [
      '/api/v1/dashboard/summary',
      '/api/v1/auth/refresh',
      '/api/v1/dashboard/summary',
    ]);
    assert.equal(calls[2].init?.signal, controller.signal);
  });

  it('403: a user keeps the status and message', async () => {
    respondWith(json(403, FORBIDDEN_JSON));
    const err = await caught(getStockMovementChart(query));
    assert.ok(err instanceof ApiError);
    assert.equal(err.status, 403);
    assert.equal(err.message, 'forbidden: admin role required');
    assert.equal(getValidationFields(err), null);
    assert.equal(calls.length, 1);
  });

  it('500: keeps the status', async () => {
    respondWith(json(500, INTERNAL_JSON));
    const err = await caught(getDashboardSummary());
    assert.ok(err instanceof ApiError);
    assert.equal(err.status, 500);
    assert.equal(err.message, 'Internal server error');
    assert.equal(getValidationFields(err), null);
  });
});

describe('getValidationFields', () => {
  it('keeps only string messages of known fields', () => {
    const err = new ApiError('invalid query parameters', 400, {
      success: false,
      message: 'invalid query parameters',
      data: { fields: { from: 'bad', sku: 'unknown field', tz: 42 } },
    });
    assert.deepEqual(getValidationFields(err), { from: 'bad' });
  });

  it('returns null without field messages', () => {
    assert.equal(getValidationFields(new ApiError('bad', 400, { success: false, message: 'bad', data: null })), null);
    assert.equal(getValidationFields(new ApiError('bad', 400, { data: { fields: {} } })), null);
    assert.equal(getValidationFields(new ApiError('bad', 400, { data: { fields: ['to'] } })), null);
    assert.equal(getValidationFields(new ApiError('bad', 400, 'text body')), null);
    assert.equal(getValidationFields(new ApiError('bad', 400)), null);
  });

  it('returns null for errors other than ApiError 400', () => {
    const body = JSON.parse(RANGE_TOO_LONG_JSON);
    assert.equal(getValidationFields(new ApiError('x', 422, body)), null);
    assert.equal(getValidationFields(new Error('x')), null);
    assert.equal(getValidationFields(null), null);
    assert.equal(getValidationFields(undefined), null);
  });
});

describe('abort', () => {
  it('an abort during the request becomes RequestAbortedError', async () => {
    respondWith(hang);
    const controller = new AbortController();
    const pending = caught(getDashboardSummary({ signal: controller.signal }));
    controller.abort();
    const err = await pending;
    assert.ok(err instanceof RequestAbortedError);
    assert.equal(err instanceof ApiError, false);
    assert.equal(isAbortError(err), true);
    assert.equal((err as RequestAbortedError).name, 'AbortError');
  });

  it('keeps the abort reason', async () => {
    respondWith(hang);
    const controller = new AbortController();
    const pending = caught(getStockMovementChart(
      { from: '2026-11-01', to: '2026-11-30', granularity: 'day' },
      { signal: controller.signal }
    ));
    controller.abort('filters changed');
    const err = await pending;
    assert.ok(err instanceof RequestAbortedError);
    assert.equal(err.reason, 'filters changed');
  });

  it('a signal aborted before the call becomes RequestAbortedError', async () => {
    respondWith(hang);
    const controller = new AbortController();
    controller.abort();
    const err = await caught(getDashboardSummary({ signal: controller.signal }));
    assert.equal(isAbortError(err), true);
  });

  it('an abort after the response arrived drops the result', async () => {
    const controller = new AbortController();
    respondWith(async () => {
      controller.abort();
      return new Response(SUMMARY_JSON, { status: 200, headers: { 'Content-Type': 'application/json' } });
    });
    const err = await caught(getDashboardSummary({ signal: controller.signal }));
    assert.equal(isAbortError(err), true);
  });

  it('a network failure stays ApiError with status 0, even with a signal', async () => {
    respondWith(async () => {
      throw new TypeError('fetch failed');
    });
    const controller = new AbortController();
    const err = await caught(getDashboardSummary({ signal: controller.signal }));
    assert.ok(err instanceof ApiError);
    assert.equal(err.status, 0);
    assert.equal(isAbortError(err), false);
  });

  it('isAbortError tells aborts from other errors', () => {
    assert.equal(isAbortError(new RequestAbortedError()), true);
    assert.equal(isAbortError(new DOMException('aborted', 'AbortError')), true);
    assert.equal(isAbortError(new ApiError('Network error', 0)), false);
    assert.equal(isAbortError(new ApiError('aborted', 0, new DOMException('aborted', 'AbortError'))), false);
    assert.equal(isAbortError(new TypeError('fetch failed')), false);
    assert.equal(isAbortError(new Error('x')), false);
    assert.equal(isAbortError(null), false);
    assert.equal(isAbortError('AbortError'), false);
  });
});
