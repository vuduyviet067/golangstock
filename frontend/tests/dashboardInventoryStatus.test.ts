import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import {
  INVENTORY_STATUS_COLOR_TOKENS,
  INVENTORY_STATUS_LABELS,
  getInventoryStatusPieState,
  toInventoryStatusPieModel,
  type InventoryStatusPieModel,
  type InventoryStatusPieState,
} from '../src/utils/dashboardInventoryStatus';
import type { DashboardSummary } from '../src/types/dashboard';

// data of the summary bodies asserted by
// backend/internal/handle/http/dashboard_handler_test.go.
const SUMMARY_JSON =
  '{"total_products":50,"total_stock":12840,"low_stock_products":6,"out_of_stock_products":4,' +
  '"status_breakdown":[{"status":"normal","count":40},{"status":"low_stock","count":6},{"status":"out_of_stock","count":4}]}';
const EMPTY_SUMMARY_JSON =
  '{"total_products":0,"total_stock":0,"low_stock_products":0,"out_of_stock_products":0,' +
  '"status_breakdown":[{"status":"normal","count":0},{"status":"low_stock","count":0},{"status":"out_of_stock","count":0}]}';

const parse = (json: string): DashboardSummary => JSON.parse(json);

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

function deepFreeze<T>(value: T): T {
  if (value && typeof value === 'object') {
    for (const child of Object.values(value)) deepFreeze(child);
    Object.freeze(value);
  }
  return value;
}

describe('labels and colours', () => {
  it('labels the three statuses in Vietnamese', () => {
    assert.deepEqual(INVENTORY_STATUS_LABELS, {
      normal: 'Bình thường',
      low_stock: 'Sắp hết hàng',
      out_of_stock: 'Hết hàng',
    });
  });

  it('uses the design tokens of the product table stock badges', () => {
    assert.deepEqual(INVENTORY_STATUS_COLOR_TOKENS, {
      normal: '--color-success',
      low_stock: '--color-warning',
      out_of_stock: '--color-danger',
    });
  });
});

describe('getInventoryStatusPieState', () => {
  it('data when there are products', () => {
    assert.equal(getInventoryStatusPieState(parse(SUMMARY_JSON)), 'data');
    assert.equal(getInventoryStatusPieState({ total_products: 1 }), 'data');
  });

  it('empty when total_products is 0', () => {
    assert.equal(getInventoryStatusPieState(parse(EMPTY_SUMMARY_JSON)), 'empty');
  });

  it('follows total_products only, never the counts', () => {
    // Counts that contradict the total do not change the result.
    assert.equal(getInventoryStatusPieState(summary(0, 3, 0, 0)), 'empty');
    assert.equal(getInventoryStatusPieState(summary(5, 0, 0, 0)), 'data');
  });
});

describe('toInventoryStatusPieModel', () => {
  it('maps the three statuses with non-zero counts', () => {
    assert.deepEqual(toInventoryStatusPieModel(parse(SUMMARY_JSON)), {
      state: 'data',
      total_products: 50,
      slices: [
        { status: 'normal', label: 'Bình thường', count: 40, colorToken: '--color-success', color: 'var(--color-success)' },
        { status: 'low_stock', label: 'Sắp hết hàng', count: 6, colorToken: '--color-warning', color: 'var(--color-warning)' },
        { status: 'out_of_stock', label: 'Hết hàng', count: 4, colorToken: '--color-danger', color: 'var(--color-danger)' },
      ],
    } satisfies InventoryStatusPieModel);
  });

  it('keeps a status whose count is 0', () => {
    const model = toInventoryStatusPieModel(summary(7, 7, 0, 0));
    assert.equal(model.state, 'data');
    assert.deepEqual(model.slices.map((s) => [s.status, s.count]), [['normal', 7], ['low_stock', 0], ['out_of_stock', 0]]);
  });

  it('keeps several zero counts around a non-zero one', () => {
    const model = toInventoryStatusPieModel(summary(2, 0, 2, 0));
    assert.deepEqual(model.slices.map((s) => s.count), [0, 2, 0]);
    assert.equal(model.slices[1].label, 'Sắp hết hàng');
    assert.equal(model.slices[1].color, 'var(--color-warning)');
  });

  it('empty when total_products is 0, still with the three zero slices', () => {
    const model = toInventoryStatusPieModel(parse(EMPTY_SUMMARY_JSON));
    assert.equal(model.state, 'empty');
    assert.equal(model.total_products, 0);
    assert.deepEqual(model.slices.map((s) => [s.status, s.count]), [['normal', 0], ['low_stock', 0], ['out_of_stock', 0]]);
  });

  it('keeps the backend order and counts as sent', () => {
    const response = parse(SUMMARY_JSON);
    const model = toInventoryStatusPieModel(response);
    assert.deepEqual(model.slices.map((s) => s.status), ['normal', 'low_stock', 'out_of_stock']);
    assert.deepEqual(model.slices.map((s) => s.count), response.status_breakdown.map((e) => e.count));
    // An order other than the contract's is passed through, not re-sorted.
    const reordered: DashboardSummary = { ...response, status_breakdown: [...response.status_breakdown].reverse() };
    assert.deepEqual(toInventoryStatusPieModel(reordered).slices.map((s) => s.status), ['out_of_stock', 'low_stock', 'normal']);
  });

  it('copies total_products and never recomputes it from the counts', () => {
    const model = toInventoryStatusPieModel(summary(9, 1, 1, 1));
    assert.equal(model.total_products, 9);
  });

  it('computes no ratio and produces only finite numbers, even with no products', () => {
    for (const input of [parse(SUMMARY_JSON), parse(EMPTY_SUMMARY_JSON)]) {
      const model = toInventoryStatusPieModel(input);
      for (const slice of model.slices) {
        assert.deepEqual(Object.keys(slice).sort(), ['color', 'colorToken', 'count', 'label', 'status']);
      }
      const numbers: number[] = [model.total_products, ...model.slices.map((s) => s.count)];
      assert.ok(numbers.every(Number.isFinite), JSON.stringify(numbers));
    }
  });

  it('does not mutate the summary (frozen input) and shares no object with it', () => {
    const response = deepFreeze(parse(SUMMARY_JSON));
    const before = JSON.stringify(response);
    const model = toInventoryStatusPieModel(response);
    assert.equal(JSON.stringify(response), before);
    assert.notEqual(model.slices, response.status_breakdown);
    assert.ok(model.slices.every((slice, i) => slice !== response.status_breakdown[i]));
  });

  it('a later change to the model does not reach the summary', () => {
    const response = parse(SUMMARY_JSON);
    const model = toInventoryStatusPieModel(response);
    (model.slices[0] as { count: number }).count = 999;
    assert.equal(response.status_breakdown[0].count, 40);
  });

  it('only needs total_products and status_breakdown', () => {
    const model = toInventoryStatusPieModel({
      total_products: 3,
      status_breakdown: [
        { status: 'normal', count: 1 },
        { status: 'low_stock', count: 1 },
        { status: 'out_of_stock', count: 1 },
      ],
    });
    assert.equal(model.slices.length, 3);
  });

  it('is type-safe: read-only model, closed state and colour sets', () => {
    const model = toInventoryStatusPieModel(parse(SUMMARY_JSON));
    const typeChecks = () => {
      // @ts-expect-error slices are read-only
      model.slices.push(model.slices[0]);
      // @ts-expect-error a slice is read-only
      model.slices[0].count = 1;
      // @ts-expect-error the state is read-only
      model.state = 'empty';
      // @ts-expect-error unknown pie state
      const unknownState: InventoryStatusPieState = 'all_zero';
      // @ts-expect-error a colour must be one of the three tokens
      const unknownColor: InventoryStatusPieModel['slices'][number]['color'] = 'var(--color-primary)';
      // @ts-expect-error the labels are read-only
      INVENTORY_STATUS_LABELS.normal = 'Normal';
      return [unknownState, unknownColor];
    };
    assert.equal(typeof typeChecks, 'function'); // compiled by tsc, never run
  });
});
