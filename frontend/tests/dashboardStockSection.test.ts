import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { StockMovementSection, TODAY_REFRESH_MS, loadStockMovements, stockMovementView } from '../src/components/StockMovementSection';
import DashboardView, { DashboardErrorNotice as NoticeFromView } from '../src/components/DashboardView';
import { DashboardErrorNotice } from '../src/components/DashboardErrorNotice';
import {
  dashboardRequestReducer,
  initialDashboardRequestState,
  type DashboardRequestAction,
  type DashboardRequestState,
} from '../src/hooks/useDashboardRequest';
import { RequestAbortedError } from '../src/services/dashboardApi';
import { ApiError } from '../src/types/api';
import type { StockMovementChart } from '../src/types/dashboard';
import { DASHBOARD_TIMEZONE, presetRange, todayIn } from '../src/utils/dashboardDates';
import { GENERIC_RANGE_ERROR_MESSAGE } from '../src/utils/dashboardRangeSelection';

// Server rendering gives the first pass only (no effect, so no request);
// the browser test covers loading, switching ranges and errors end to end.

// The daily body asserted by backend/internal/handle/http/dashboard_handler_test.go.
const DAILY_CHART_JSON =
  '{"success":true,"message":"Stock movement chart retrieved successfully","data":{' +
  '"from":"2026-11-01","to":"2026-11-03","granularity":"day","timezone":"Asia/Ho_Chi_Minh",' +
  '"points":[{"period":"2026-11-01","stock_in":0,"stock_out":0},{"period":"2026-11-02","stock_in":120,"stock_out":45},{"period":"2026-11-03","stock_in":0,"stock_out":0}],' +
  '"totals":{"stock_in":120,"stock_out":45}}}';
const RANGE_TOO_LONG_JSON =
  '{"success":false,"message":"invalid query parameters","data":{"fields":{"to":"range must be at most 92 days by day or 731 days by month"}}}';
const CHART: StockMovementChart = JSON.parse(DAILY_CHART_JSON).data;
const RANGE = { from: '2026-11-01', to: '2026-11-03', granularity: 'day' } as const;

const badRequest = (fields: unknown) =>
  new ApiError('invalid query parameters', 400, { success: false, message: 'invalid query parameters', data: { fields } });

type Result = Awaited<ReturnType<typeof loadStockMovements>>;
const run = (...actions: DashboardRequestAction<Result>[]): DashboardRequestState<Result> =>
  actions.reduce(dashboardRequestReducer<Result>, initialDashboardRequestState<Result>());
const chartResult: Result = { chart: CHART, rejected: null };
const rejectedResult: Result = { chart: null, rejected: [{ field: 'to', message: 'x' }] };

const noop = () => {};
const globalWithFetch = globalThis as { fetch: typeof fetch };
const originalFetch = globalWithFetch.fetch;
let fetchUrls: string[] = [];
let fetchResponse: () => Response = () => {
  throw new Error('no request is expected');
};
beforeEach(() => {
  fetchUrls = [];
  fetchResponse = () => {
    throw new Error('no request is expected');
  };
  globalWithFetch.fetch = async (input: RequestInfo | URL) => {
    fetchUrls.push(String(input));
    return fetchResponse();
  };
});
afterEach(() => {
  globalWithFetch.fetch = originalFetch;
});

describe('loadStockMovements', () => {
  it('requests the range (from, to, granularity) with the signal and returns the chart as it is', async () => {
    const calls: unknown[][] = [];
    const signal = new AbortController().signal;
    const result = await loadStockMovements(RANGE, signal, async (...args: unknown[]) => {
      calls.push(args);
      return CHART;
    });
    assert.deepEqual(calls, [[{ from: '2026-11-01', to: '2026-11-03', granularity: 'day' }, { signal }]]);
    assert.equal(calls[0][1] && (calls[0][1] as { signal: AbortSignal }).signal, signal);
    assert.equal(result.chart, CHART);
    assert.equal(result.rejected, null);
  });

  it('a 400 resolves to the Vietnamese messages per field', async () => {
    const result = await loadStockMovements(RANGE, new AbortController().signal, async () => {
      throw badRequest({ from: 'must be on or before to', to: 'range must be at most 92 days by day or 731 days by month' });
    });
    assert.equal(result.chart, null);
    assert.deepEqual(result.rejected, [
      { field: 'from', message: 'Ngày bắt đầu phải trước hoặc bằng ngày kết thúc.' },
      { field: 'to', message: 'Khoảng thời gian quá dài: tối đa 92 ngày khi xem theo ngày hoặc 731 ngày khi xem theo tháng.' },
    ]);
    const generic = await loadStockMovements(RANGE, new AbortController().signal, async () => {
      throw new ApiError('bad', 400);
    });
    assert.deepEqual(generic.rejected, [{ field: null, message: GENERIC_RANGE_ERROR_MESSAGE }]);
  });

  it('rethrows every other error unchanged for the request hook (5xx, network, 401, 403, abort, unexpected)', async () => {
    const errors = [
      new ApiError('Internal server error', 500),
      new ApiError('network', 0),
      new ApiError('authentication required', 401),
      new ApiError('forbidden', 403),
      new RequestAbortedError(),
      new DOMException('aborted', 'AbortError'),
      new TypeError('boom'),
    ];
    for (const error of errors) {
      await assert.rejects(
        loadStockMovements(RANGE, new AbortController().signal, async () => {
          throw error;
        }),
        (thrown: unknown) => thrown === error
      );
    }
  });

  it('by default calls the stock movement API with tz Asia/Ho_Chi_Minh', async () => {
    fetchResponse = () => new Response(DAILY_CHART_JSON, { status: 200, headers: { 'Content-Type': 'application/json' } });
    const result = await loadStockMovements(RANGE, new AbortController().signal);
    assert.deepEqual(fetchUrls, ['/api/v1/dashboard/stock-movements?from=2026-11-01&to=2026-11-03&granularity=day&tz=Asia%2FHo_Chi_Minh']);
    assert.deepEqual(result, { chart: CHART, rejected: null });

    fetchUrls = [];
    fetchResponse = () => new Response(RANGE_TOO_LONG_JSON, { status: 400, headers: { 'Content-Type': 'application/json' } });
    const rejected = await loadStockMovements({ from: '2024-01-01', to: '2026-11-03', granularity: 'auto' }, new AbortController().signal);
    assert.deepEqual(fetchUrls, ['/api/v1/dashboard/stock-movements?from=2024-01-01&to=2026-11-03&granularity=auto&tz=Asia%2FHo_Chi_Minh']);
    assert.deepEqual(rejected.rejected, [
      { field: 'to', message: 'Khoảng thời gian quá dài: tối đa 92 ngày khi xem theo ngày hoặc 731 ngày khi xem theo tháng.' },
    ]);
  });
});

describe('stockMovementView', () => {
  it('first load: the placeholder, busy', () => {
    assert.deepEqual(stockMovementView(run({ type: 'start', requestId: 1 })), { view: 'loading', busy: true });
  });

  it('a chart: shown; while the next range loads it stays, busy (dimmed)', () => {
    const loaded = run({ type: 'start', requestId: 1 }, { type: 'success', requestId: 1, data: chartResult });
    assert.deepEqual(stockMovementView(loaded), { view: 'chart', busy: false });
    const reloading = dashboardRequestReducer(loaded, { type: 'start', requestId: 2 });
    assert.deepEqual(stockMovementView(reloading), { view: 'chart', busy: true });
    assert.equal(reloading.data, chartResult);
  });

  it('a 400: the messages; the next load shows the placeholder, not the messages', () => {
    const rejected = run({ type: 'start', requestId: 1 }, { type: 'success', requestId: 1, data: rejectedResult });
    assert.deepEqual(stockMovementView(rejected), { view: 'rejected', busy: false });
    assert.deepEqual(stockMovementView(dashboardRequestReducer(rejected, { type: 'start', requestId: 2 })), { view: 'loading', busy: true });
  });

  it('an error: the notice only, even when an older chart is kept', () => {
    const failed = run(
      { type: 'start', requestId: 1 },
      { type: 'success', requestId: 1, data: chartResult },
      { type: 'start', requestId: 2 },
      { type: 'failure', requestId: 2, error: new ApiError('x', 503) }
    );
    assert.equal(failed.data, chartResult);
    assert.deepEqual(stockMovementView(failed), { view: 'error', busy: false });
    // Thử lại: loading again, with the older chart dimmed.
    assert.deepEqual(stockMovementView(dashboardRequestReducer(failed, { type: 'start', requestId: 3 })), { view: 'chart', busy: true });
  });

  it('an abort changes nothing', () => {
    const loaded = run({ type: 'start', requestId: 1 }, { type: 'success', requestId: 1, data: chartResult }, { type: 'start', requestId: 2 });
    const aborted = dashboardRequestReducer(loaded, { type: 'failure', requestId: 2, error: new RequestAbortedError() });
    assert.deepEqual(stockMovementView(aborted), { view: 'chart', busy: true });
  });
});

describe('StockMovementSection', () => {
  it('reads today again every minute', () => {
    assert.equal(TODAY_REFRESH_MS, 60_000);
  });

  it('first render: busy, the filter on the last 30 days of Asia/Ho_Chi_Minh, the placeholder, no request', () => {
    const before = todayIn(DASHBOARD_TIMEZONE);
    const html = renderToStaticMarkup(createElement(StockMovementSection, { onReauthenticate: noop }));
    const after = todayIn(DASHBOARD_TIMEZONE);
    assert.match(html, /^<section class="stock-movement" aria-label="Nhập và xuất kho" aria-busy="true"><div class="stock-movement-filter"><div class="range-filter">/);
    assert.match(html, /aria-pressed="true">30 ngày</);
    const attr = (tag: string, name: string) => new RegExp(` ${name}="([^"]*)"`).exec(tag)?.[1];
    const values = (html.match(/<input [^>]*>/g) ?? []).map((tag) => [attr(tag, 'value'), attr(tag, 'max')]);
    const expected = (today: string) => {
      const range = presetRange('30d', today);
      return [
        [range.from, today],
        [range.to, today],
      ];
    };
    assert.ok(JSON.stringify(values) === JSON.stringify(expected(before)) || JSON.stringify(values) === JSON.stringify(expected(after)), JSON.stringify(values));
    assert.match(html, /<div class="stock-movement-placeholder" role="status"><span class="spinner" aria-hidden="true"><\/span><span>Đang tải biểu đồ nhập xuất kho\.\.\.<\/span><\/div><\/section>$/);
    for (const absent of ['role="alert"', 'stock-chart', 'stock-movement-chart']) {
      assert.equal(html.includes(absent), false, absent);
    }
    assert.deepEqual(fetchUrls, []);
  });
});

describe('DashboardView with the stock movement section', () => {
  it('still exports DashboardErrorNotice, now from its own file', () => {
    assert.equal(NoticeFromView, DashboardErrorNotice);
  });

  it('renders the section after the summary, both loading, without requesting anything', () => {
    const html = renderToStaticMarkup(createElement(DashboardView, { onReauthenticate: noop }));
    const summary = html.indexOf('Đang tải số liệu');
    const section = html.indexOf('<section class="stock-movement"');
    assert.ok(summary > 0 && section > summary, `${summary} ${section}`);
    assert.match(html, /<\/section><\/section>$/);
    assert.match(html, /Đang tải biểu đồ nhập xuất kho/);
    assert.deepEqual(fetchUrls, []);
  });
});
