import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { SummaryCards } from '../src/components/SummaryCards';
import type { DashboardSummary } from '../src/types/dashboard';

const summary = (
  total_products: number,
  total_stock: number,
  low_stock_products: number,
  out_of_stock_products: number
): DashboardSummary => ({
  total_products,
  total_stock,
  low_stock_products,
  out_of_stock_products,
  status_breakdown: [
    { status: 'normal', count: total_products - low_stock_products - out_of_stock_products },
    { status: 'low_stock', count: low_stock_products },
    { status: 'out_of_stock', count: out_of_stock_products },
  ],
});

interface Card {
  tone: string;
  field: string;
  label: string;
  value: string;
}

function renderCards(data: DashboardSummary): { html: string; cards: Card[] } {
  const html = renderToStaticMarkup(createElement(SummaryCards, { summary: data }));
  const cards = [...html.matchAll(
    /<div class="summary-card summary-card--(\w+)" data-field="(\w+)"><dt class="summary-card-label">([^<]*)<\/dt><dd class="summary-card-value">([^<]*)<\/dd><\/div>/g
  )].map(([, tone, field, label, value]) => ({ tone, field, label, value }));
  return { html, cards };
}

const globalWithFetch = globalThis as { fetch: typeof fetch };
const originalFetch = globalWithFetch.fetch;
let fetchCalls = 0;
beforeEach(() => {
  fetchCalls = 0;
  globalWithFetch.fetch = async () => {
    fetchCalls += 1;
    throw new Error('SummaryCards must not request anything');
  };
});
afterEach(() => {
  globalWithFetch.fetch = originalFetch;
});

describe('SummaryCards', () => {
  it('shows the four cards in order, with Vietnamese labels and their accent tone', () => {
    const { html, cards } = renderCards(summary(50, 12840, 6, 4));
    assert.match(html, /^<dl class="summary-cards">/);
    assert.deepEqual(cards, [
      { tone: 'primary', field: 'total_products', label: 'Tổng sản phẩm', value: '50' },
      { tone: 'accent', field: 'total_stock', label: 'Tổng tồn kho', value: '12.840' },
      { tone: 'warning', field: 'low_stock_products', label: 'Sản phẩm sắp hết hàng', value: '6' },
      { tone: 'danger', field: 'out_of_stock_products', label: 'Sản phẩm hết hàng', value: '4' },
    ]);
  });

  it('formats large numbers for vi-VN', () => {
    const { cards } = renderCards(summary(1234, 98765432, 56, 7));
    assert.deepEqual(cards.map((c) => c.value), ['1.234', '98.765.432', '56', '7']);
    const { cards: huge } = renderCards(summary(1000000, 2147483647, 999999, 100000));
    assert.deepEqual(huge.map((c) => c.value), ['1.000.000', '2.147.483.647', '999.999', '100.000']);
  });

  it('shows zeros as 0', () => {
    const { cards } = renderCards(summary(0, 0, 0, 0));
    assert.deepEqual(cards.map((c) => c.value), ['0', '0', '0', '0']);
  });

  it('shows the backend figures as given, never recomputed', () => {
    // Figures that do not add up are still shown verbatim.
    const inconsistent: DashboardSummary = {
      total_products: 5,
      total_stock: 1,
      low_stock_products: 9,
      out_of_stock_products: 8,
      status_breakdown: [
        { status: 'normal', count: 100 },
        { status: 'low_stock', count: 200 },
        { status: 'out_of_stock', count: 300 },
      ],
    };
    assert.deepEqual(renderCards(inconsistent).cards.map((c) => c.value), ['5', '1', '9', '8']);
  });

  it('is presentational: renders from props without requesting anything', () => {
    renderCards(summary(3, 53, 1, 1));
    assert.equal(fetchCalls, 0);
  });
});
