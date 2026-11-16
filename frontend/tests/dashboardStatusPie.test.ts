import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import {
  InventoryStatusPie,
  formatProductCount,
  legendItemSorter,
  legendText,
  statusSummaryText,
  tooltipFormatter,
} from '../src/components/InventoryStatusPie';
import { toInventoryStatusPieModel, type InventoryStatusPieModel } from '../src/utils/dashboardInventoryStatus';
import type { DashboardSummary } from '../src/types/dashboard';

// Recharts draws nothing during server rendering (it measures in the
// browser), so these tests cover the component's own markup and text; the
// drawing is covered by the browser test.

const summary = (total: number, normal: number, low: number, out: number): DashboardSummary => ({
  total_products: total,
  total_stock: 0,
  low_stock_products: low,
  out_of_stock_products: out,
  status_breakdown: [
    { status: 'normal', count: normal },
    { status: 'low_stock', count: low },
    { status: 'out_of_stock', count: out },
  ],
});
const DATA = toInventoryStatusPieModel(summary(50, 40, 6, 4));
const LARGE = toInventoryStatusPieModel(summary(1234567, 1200000, 30000, 4567));
const EMPTY = toInventoryStatusPieModel(summary(0, 0, 0, 0));

const render = (model: InventoryStatusPieModel) => renderToStaticMarkup(createElement(InventoryStatusPie, { model }));

const globalWithFetch = globalThis as { fetch: typeof fetch };
const originalFetch = globalWithFetch.fetch;
let fetchCalls = 0;
beforeEach(() => {
  fetchCalls = 0;
  globalWithFetch.fetch = async () => {
    fetchCalls += 1;
    throw new Error('InventoryStatusPie must not request anything');
  };
});
afterEach(() => {
  globalWithFetch.fetch = originalFetch;
});

describe('pie text helpers', () => {
  it('formats product counts for vi-VN', () => {
    assert.equal(formatProductCount(0), '0 sản phẩm');
    assert.equal(formatProductCount(40), '40 sản phẩm');
    assert.equal(formatProductCount(1234567), '1.234.567 sản phẩm');
  });

  it('tooltip: Vietnamese label and the backend count', () => {
    assert.deepEqual(tooltipFormatter(40, 'Bình thường'), ['40 sản phẩm', 'Bình thường']);
    assert.deepEqual(tooltipFormatter(30000, 'Sắp hết hàng'), ['30.000 sản phẩm', 'Sắp hết hàng']);
  });

  it('legend: each label with its count, so colour is not the only cue', () => {
    assert.deepEqual(DATA.slices.map((s) => legendText(DATA, s.label)), ['Bình thường: 40', 'Sắp hết hàng: 6', 'Hết hàng: 4']);
    assert.equal(legendText(LARGE, 'Bình thường'), 'Bình thường: 1.200.000');
    assert.equal(legendText(DATA, 'Không có'), 'Không có');
  });

  it('legend order: the backend order, not the alphabetical default of Recharts', () => {
    const sorter = legendItemSorter(DATA);
    const items = ['Hết hàng', 'Bình thường', 'Sắp hết hàng'].map((value) => ({ value }));
    const sorted = [...items].sort((a, b) => sorter(a) - sorter(b)).map((i) => i.value);
    assert.deepEqual(sorted, ['Bình thường', 'Sắp hết hàng', 'Hết hàng']);
    assert.equal(sorter({ value: 'Không có' }), 3);
  });

  it('screen reader summary: the backend total and every status, zero counts included', () => {
    assert.equal(statusSummaryText(DATA), 'Tổng 50 sản phẩm. Bình thường: 40, Sắp hết hàng: 6, Hết hàng: 4.');
    const model = toInventoryStatusPieModel(summary(7, 7, 0, 0));
    assert.equal(statusSummaryText(model), 'Tổng 7 sản phẩm. Bình thường: 7, Sắp hết hàng: 0, Hết hàng: 0.');
  });

  it('uses the backend total, never the sum of the counts', () => {
    const model = toInventoryStatusPieModel(summary(9, 1, 1, 1));
    assert.match(statusSummaryText(model), /^Tổng 9 sản phẩm\./);
  });
});

describe('InventoryStatusPie', () => {
  it('data: title, backend total, screen reader summary and the chart box', () => {
    const html = render(DATA);
    assert.match(html, /<section class="status-pie" aria-labelledby="inventory-status-pie-title" data-state="data">/);
    assert.match(html, /<h3 id="inventory-status-pie-title" class="status-pie-title">Phân bố trạng thái tồn kho<\/h3>/);
    assert.match(html, /<p class="status-pie-total">Tổng: 50 sản phẩm<\/p>/);
    assert.match(html, /<p class="sr-only">Tổng 50 sản phẩm\. Bình thường: 40, Sắp hết hàng: 6, Hết hàng: 4\.<\/p>/);
    assert.match(html, /<div class="status-pie-chart">/);
    assert.equal(html.includes('status-pie-empty'), false);
  });

  it('formats large figures for vi-VN', () => {
    const html = render(LARGE);
    assert.match(html, /Tổng: 1\.234\.567 sản phẩm/);
    assert.match(html, /Bình thường: 1\.200\.000, Sắp hết hàng: 30\.000, Hết hàng: 4\.567/);
  });

  it('empty: a clear message and no chart', () => {
    const html = render(EMPTY);
    assert.match(html, /data-state="empty"/);
    assert.match(html, /<p class="status-pie-empty">Chưa có sản phẩm nào để thống kê trạng thái tồn kho\.<\/p>/);
    assert.match(html, /Tổng: 0 sản phẩm/);
    assert.equal(html.includes('status-pie-chart'), false);
    assert.equal(html.includes('recharts'), false);
    assert.equal(html.includes('sr-only'), false);
  });

  it('follows the model state: zero counts with products is still a chart, not the empty state', () => {
    const html = render(toInventoryStatusPieModel(summary(3, 0, 0, 0)));
    assert.match(html, /data-state="data"/);
    assert.match(html, /<div class="status-pie-chart">/);
    assert.equal(html.includes('status-pie-empty'), false);
  });

  it('is presentational: renders from the model without requesting anything', () => {
    render(DATA);
    render(EMPTY);
    assert.equal(fetchCalls, 0);
  });
});
