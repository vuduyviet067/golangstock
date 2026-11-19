import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import {
  ALL_ZERO_CHART_MESSAGE,
  EMPTY_CHART_MESSAGE,
  STOCK_CHART_SERIES,
  StockMovementBarChart,
  axisTickFormatter,
  formatQuantity,
  legendSeriesSorter,
  stockChartSummaryText,
  tooltipLabelFormatter,
  tooltipValueFormatter,
} from '../src/components/StockMovementBarChart';
import { toStockMovementChartModel, type StockMovementChartModel } from '../src/utils/dashboardChart';
import type { StockMovementChart } from '../src/types/dashboard';

// Recharts draws nothing during server rendering (it measures in the
// browser), so these tests cover the component's own markup and text; the
// bars, axes, tooltip and legend are covered by the browser test.

// The daily body asserted by backend/internal/handle/http/dashboard_handler_test.go.
const DAILY: StockMovementChart = JSON.parse(
  '{"from":"2026-11-01","to":"2026-11-03","granularity":"day","timezone":"Asia/Ho_Chi_Minh",' +
  '"points":[{"period":"2026-11-01","stock_in":0,"stock_out":0},{"period":"2026-11-02","stock_in":120,"stock_out":45},{"period":"2026-11-03","stock_in":0,"stock_out":0}],' +
  '"totals":{"stock_in":120,"stock_out":45}}'
);
const MONTHLY: StockMovementChart = {
  from: '2025-12-01',
  to: '2026-11-12',
  granularity: 'month',
  timezone: 'Asia/Ho_Chi_Minh',
  points: [
    { period: '2025-12-01', stock_in: 1500, stock_out: 1200 },
    { period: '2026-01-01', stock_in: 0, stock_out: 0 },
    { period: '2026-11-01', stock_in: 98765432, stock_out: 1234567 },
  ],
  totals: { stock_in: 98766932, stock_out: 1235767 },
};
const ALL_ZERO: StockMovementChart = {
  ...DAILY,
  points: DAILY.points.map((p) => ({ ...p, stock_in: 0, stock_out: 0 })),
  totals: { stock_in: 0, stock_out: 0 },
};
const EMPTY: StockMovementChart = { ...DAILY, points: [], totals: { stock_in: 0, stock_out: 0 } };

const model = (chart: StockMovementChart) => toStockMovementChartModel(chart);
const render = (m: StockMovementChartModel) => renderToStaticMarkup(createElement(StockMovementBarChart, { model: m }));

const globalWithFetch = globalThis as { fetch: typeof fetch };
const originalFetch = globalWithFetch.fetch;
let fetchCalls = 0;
beforeEach(() => {
  fetchCalls = 0;
  globalWithFetch.fetch = async () => {
    fetchCalls += 1;
    throw new Error('StockMovementBarChart must not request anything');
  };
});
afterEach(() => {
  globalWithFetch.fetch = originalFetch;
});

describe('stock chart helpers', () => {
  it('two series, stock in then stock out, with the agreed colour tokens', () => {
    assert.deepEqual(STOCK_CHART_SERIES, [
      { dataKey: 'stock_in', name: 'Nhập kho', color: 'var(--color-accent)' },
      { dataKey: 'stock_out', name: 'Xuất kho', color: 'var(--color-warning)' },
    ]);
  });

  it('legend order: stock in before stock out, not the alphabetical default', () => {
    const items = [{ value: 'Xuất kho' }, { value: 'Nhập kho' }];
    // "Nhập" < "Xuất" alphabetically too, so check the keys themselves.
    assert.equal(legendSeriesSorter({ value: 'Nhập kho' }), 0);
    assert.equal(legendSeriesSorter({ value: 'Xuất kho' }), 1);
    assert.equal(legendSeriesSorter({ value: 'Khác' }), 2);
    assert.deepEqual([...items].sort((a, b) => legendSeriesSorter(a) - legendSeriesSorter(b)).map((i) => i.value), ['Nhập kho', 'Xuất kho']);
  });

  it('formats quantities for vi-VN', () => {
    assert.equal(formatQuantity(0), '0');
    assert.equal(formatQuantity(1234567), '1.234.567');
  });

  it('axis ticks and tooltip titles follow the granularity, through dashboardChartLabels', () => {
    assert.equal(axisTickFormatter('day')('2026-11-02'), '02/11');
    assert.equal(axisTickFormatter('month')('2026-11-01'), '11/2026');
    assert.equal(tooltipLabelFormatter('day')('2026-11-02'), '02/11/2026');
    assert.equal(tooltipLabelFormatter('month')('2026-11-01'), 'Tháng 11/2026');
    assert.equal(tooltipLabelFormatter('day')('2028-02-29'), '29/02/2028');
  });

  it('tooltip values: the series name and the backend quantity', () => {
    assert.deepEqual(tooltipValueFormatter(120, 'Nhập kho'), ['120', 'Nhập kho']);
    assert.deepEqual(tooltipValueFormatter(1234567, 'Xuất kho'), ['1.234.567', 'Xuất kho']);
  });

  it('screen reader summary: range, granularity and the backend totals', () => {
    assert.equal(stockChartSummaryText(model(DAILY)), '01/11/2026 – 03/11/2026, theo ngày: tổng nhập kho 120, tổng xuất kho 45.');
    assert.equal(
      stockChartSummaryText(model(MONTHLY)),
      '01/12/2025 – 12/11/2026, theo tháng: tổng nhập kho 98.766.932, tổng xuất kho 1.235.767.'
    );
  });
});

describe('StockMovementBarChart', () => {
  it('data: title, range, granularity, backend totals, summary and the chart box', () => {
    const html = render(model(DAILY));
    assert.match(html, /<section class="stock-chart" aria-labelledby="stock-movement-chart-title" data-state="data">/);
    assert.match(html, /<h3 id="stock-movement-chart-title" class="stock-chart-title">Nhập và xuất kho<\/h3>/);
    assert.match(html, /<p class="stock-chart-range">01\/11\/2026 – 03\/11\/2026 · Theo ngày<\/p>/);
    assert.match(html, /<div class="stock-chart-total stock-chart-total--in"><dt>Tổng nhập kho<\/dt><dd>120<\/dd><\/div>/);
    assert.match(html, /<div class="stock-chart-total stock-chart-total--out"><dt>Tổng xuất kho<\/dt><dd>45<\/dd><\/div>/);
    assert.match(html, /<p class="sr-only">01\/11\/2026 – 03\/11\/2026, theo ngày: tổng nhập kho 120, tổng xuất kho 45\.<\/p>/);
    assert.match(html, /<div class="stock-chart-canvas">/);
    assert.equal(html.includes('stock-chart-note'), false);
    assert.equal(html.includes('stock-chart-empty'), false);
  });

  it('monthly: Theo tháng and large totals formatted for vi-VN', () => {
    const html = render(model(MONTHLY));
    assert.match(html, /01\/12\/2025 – 12\/11\/2026 · Theo tháng/);
    assert.match(html, /<dt>Tổng nhập kho<\/dt><dd>98\.766\.932<\/dd>/);
    assert.match(html, /<dt>Tổng xuất kho<\/dt><dd>1\.235\.767<\/dd>/);
  });

  it('shows the backend totals, never a sum of the rows (cards and screen reader summary)', () => {
    const inconsistent = model({ ...DAILY, totals: { stock_in: 7, stock_out: 3 } });
    const html = render(inconsistent);
    assert.match(html, /<dt>Tổng nhập kho<\/dt><dd>7<\/dd>/);
    assert.match(html, /<dt>Tổng xuất kho<\/dt><dd>3<\/dd>/);
    assert.match(html, /<p class="sr-only">[^<]*tổng nhập kho 7, tổng xuất kho 3\.<\/p>/);
    assert.equal(stockChartSummaryText(inconsistent), '01/11/2026 – 03/11/2026, theo ngày: tổng nhập kho 7, tổng xuất kho 3.');
  });

  it('all_zero: the chart box stays (axes are drawn), with a no-movement note and zero totals', () => {
    const html = render(model(ALL_ZERO));
    assert.match(html, /data-state="all_zero"/);
    assert.match(html, new RegExp(`<p class="stock-chart-note">${ALL_ZERO_CHART_MESSAGE}</p>`));
    assert.match(html, /<div class="stock-chart-canvas">/);
    assert.match(html, /<dt>Tổng nhập kho<\/dt><dd>0<\/dd>/);
    assert.equal(html.includes('stock-chart-empty'), false);
  });

  it('empty: a clear message, no chart, no totals', () => {
    const html = render(model(EMPTY));
    assert.match(html, /data-state="empty"/);
    assert.match(html, new RegExp(`<p class="stock-chart-empty">${EMPTY_CHART_MESSAGE}</p>`));
    assert.match(html, /01\/11\/2026 – 03\/11\/2026 · Theo ngày/);
    for (const absent of ['stock-chart-canvas', 'stock-chart-totals', 'stock-chart-note', 'sr-only', 'recharts']) {
      assert.equal(html.includes(absent), false, absent);
    }
  });

  it('follows the model state, not the rows', () => {
    // Rows with movements but zero backend totals: all_zero, as the model says.
    const allZero = render(model({ ...DAILY, totals: { stock_in: 0, stock_out: 0 } }));
    assert.match(allZero, /data-state="all_zero"/);
    assert.match(allZero, /<p class="stock-chart-note">/);
    // Zero rows but non-zero backend totals: data, so no no-movement note.
    const data = render(model({ ...ALL_ZERO, totals: { stock_in: 5, stock_out: 0 } }));
    assert.match(data, /data-state="data"/);
    assert.equal(data.includes('stock-chart-note'), false);
  });

  it('does not change the model it is given', () => {
    const m = model(DAILY);
    const before = JSON.stringify(m);
    render(m);
    assert.equal(JSON.stringify(m), before);
  });

  it('is presentational: renders from the model without requesting anything', () => {
    render(model(DAILY));
    render(model(EMPTY));
    assert.equal(fetchCalls, 0);
  });
});
