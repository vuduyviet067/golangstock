import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import {
  getChartDisplayState,
  toStockMovementChartModel,
  type ChartDisplayState,
  type StockMovementChartModel,
} from '../src/utils/dashboardChart';
import type { StockMovementChart } from '../src/types/dashboard';

// Bodies asserted by backend/internal/handle/http/dashboard_handler_test.go.
const DAILY_JSON =
  '{"from":"2026-11-01","to":"2026-11-03","granularity":"day","timezone":"Asia/Ho_Chi_Minh",' +
  '"points":[{"period":"2026-11-01","stock_in":0,"stock_out":0},{"period":"2026-11-02","stock_in":120,"stock_out":45},{"period":"2026-11-03","stock_in":0,"stock_out":0}],' +
  '"totals":{"stock_in":120,"stock_out":45}}';
const MONTHLY_ZERO_JSON =
  '{"from":"2026-01-15","to":"2026-03-20","granularity":"month","timezone":"UTC",' +
  '"points":[{"period":"2026-01-01","stock_in":0,"stock_out":0},{"period":"2026-02-01","stock_in":0,"stock_out":0},{"period":"2026-03-01","stock_in":0,"stock_out":0}],' +
  '"totals":{"stock_in":0,"stock_out":0}}';

const parse = (json: string): StockMovementChart => JSON.parse(json);

const chart = (overrides: Partial<StockMovementChart>): StockMovementChart => ({
  from: '2026-11-01',
  to: '2026-11-05',
  granularity: 'day',
  timezone: 'Asia/Ho_Chi_Minh',
  points: [],
  totals: { stock_in: 0, stock_out: 0 },
  ...overrides,
});

// Daily range with empty buckets between the ones with movements.
const INTERLEAVED = chart({
  points: [
    { period: '2026-11-01', stock_in: 0, stock_out: 0 },
    { period: '2026-11-02', stock_in: 3, stock_out: 0 },
    { period: '2026-11-03', stock_in: 0, stock_out: 0 },
    { period: '2026-11-04', stock_in: 0, stock_out: 4 },
    { period: '2026-11-05', stock_in: 0, stock_out: 0 },
  ],
  totals: { stock_in: 3, stock_out: 4 },
});

// Monthly range starting and ending inside a month: from/to stay the
// requested days, the buckets are the first days of the months.
const MONTHLY_DATA = chart({
  from: '2026-01-15',
  to: '2026-03-20',
  granularity: 'month',
  timezone: 'UTC',
  points: [
    { period: '2026-01-01', stock_in: 5, stock_out: 2 },
    { period: '2026-02-01', stock_in: 0, stock_out: 0 },
    { period: '2026-03-01', stock_in: 7, stock_out: 9 },
  ],
  totals: { stock_in: 12, stock_out: 11 },
});

function deepFreeze<T>(value: T): T {
  if (value && typeof value === 'object') {
    for (const child of Object.values(value)) deepFreeze(child);
    Object.freeze(value);
  }
  return value;
}

describe('getChartDisplayState', () => {
  it('data: buckets with stock in and stock out', () => {
    assert.equal(getChartDisplayState(parse(DAILY_JSON)), 'data');
    assert.equal(getChartDisplayState(MONTHLY_DATA), 'data');
  });

  it('all_zero: buckets, but both totals are 0', () => {
    assert.equal(getChartDisplayState(parse(MONTHLY_ZERO_JSON)), 'all_zero');
    assert.equal(
      getChartDisplayState(chart({ points: [{ period: '2026-11-01', stock_in: 0, stock_out: 0 }] })),
      'all_zero'
    );
  });

  it('data: only stock in moved', () => {
    assert.equal(
      getChartDisplayState(chart({
        points: [{ period: '2026-11-01', stock_in: 4, stock_out: 0 }],
        totals: { stock_in: 4, stock_out: 0 },
      })),
      'data'
    );
  });

  it('data: only stock out moved', () => {
    assert.equal(
      getChartDisplayState(chart({
        points: [{ period: '2026-11-01', stock_in: 0, stock_out: 6 }],
        totals: { stock_in: 0, stock_out: 6 },
      })),
      'data'
    );
  });

  it('empty: no bucket at all, whatever the totals say', () => {
    assert.equal(getChartDisplayState(chart({ points: [] })), 'empty');
    assert.equal(getChartDisplayState(chart({ points: [], totals: { stock_in: 9, stock_out: 1 } })), 'empty');
  });

  it('data: empty buckets between buckets with movements', () => {
    assert.equal(getChartDisplayState(INTERLEAVED), 'data');
  });

  it('follows the backend totals, never the sum of the points', () => {
    // Buckets with movements but zero totals: the totals win.
    assert.equal(
      getChartDisplayState(chart({
        points: [{ period: '2026-11-01', stock_in: 5, stock_out: 5 }],
        totals: { stock_in: 0, stock_out: 0 },
      })),
      'all_zero'
    );
    // Zero buckets but non-zero totals: the totals win.
    assert.equal(
      getChartDisplayState(chart({
        points: [{ period: '2026-11-01', stock_in: 0, stock_out: 0 }],
        totals: { stock_in: 2, stock_out: 0 },
      })),
      'data'
    );
  });
});

describe('toStockMovementChartModel', () => {
  it('maps a daily response: same order, periods, values, granularity, range and totals', () => {
    const response = parse(DAILY_JSON);
    assert.deepEqual(toStockMovementChartModel(response), {
      state: 'data',
      granularity: 'day',
      from: '2026-11-01',
      to: '2026-11-03',
      timezone: 'Asia/Ho_Chi_Minh',
      rows: [
        { period: '2026-11-01', stock_in: 0, stock_out: 0 },
        { period: '2026-11-02', stock_in: 120, stock_out: 45 },
        { period: '2026-11-03', stock_in: 0, stock_out: 0 },
      ],
      totals: { stock_in: 120, stock_out: 45 },
    } satisfies StockMovementChartModel);
  });

  it('maps a monthly response with partial first and last months', () => {
    const model = toStockMovementChartModel(MONTHLY_DATA);
    assert.equal(model.state, 'data');
    assert.equal(model.granularity, 'month');
    assert.equal(model.from, '2026-01-15');
    assert.equal(model.to, '2026-03-20');
    assert.equal(model.timezone, 'UTC');
    assert.deepEqual(model.rows.map((r) => r.period), ['2026-01-01', '2026-02-01', '2026-03-01']);
    assert.deepEqual(model.rows, MONTHLY_DATA.points);
    assert.deepEqual(model.totals, { stock_in: 12, stock_out: 11 });
  });

  it('maps an all-zero monthly response, keeping every bucket', () => {
    const model = toStockMovementChartModel(parse(MONTHLY_ZERO_JSON));
    assert.equal(model.state, 'all_zero');
    assert.equal(model.rows.length, 3);
    assert.ok(model.rows.every((r) => r.stock_in === 0 && r.stock_out === 0));
  });

  it('maps an empty response to no rows', () => {
    const model = toStockMovementChartModel(chart({ points: [] }));
    assert.equal(model.state, 'empty');
    assert.deepEqual(model.rows, []);
    assert.deepEqual(model.totals, { stock_in: 0, stock_out: 0 });
  });

  it('keeps the order of the points as sent, even if not sorted', () => {
    const unsorted = chart({
      points: [
        { period: '2026-11-03', stock_in: 1, stock_out: 0 },
        { period: '2026-11-01', stock_in: 2, stock_out: 0 },
        { period: '2026-11-02', stock_in: 3, stock_out: 0 },
      ],
      totals: { stock_in: 6, stock_out: 0 },
    });
    assert.deepEqual(toStockMovementChartModel(unsorted).rows.map((r) => r.period), ['2026-11-03', '2026-11-01', '2026-11-02']);
  });

  it('keeps empty buckets between buckets with movements, values unchanged', () => {
    const model = toStockMovementChartModel(INTERLEAVED);
    assert.equal(model.state, 'data');
    assert.deepEqual(model.rows, INTERLEAVED.points);
    assert.deepEqual(model.rows.map((r) => [r.stock_in, r.stock_out]), [[0, 0], [3, 0], [0, 0], [0, 4], [0, 0]]);
  });

  it('keeps stock_out positive', () => {
    const model = toStockMovementChartModel(INTERLEAVED);
    assert.equal(model.rows[3].stock_out, 4);
    assert.ok(model.rows.every((r) => r.stock_out >= 0));
  });

  it('copies the backend totals, never recomputes them from the points', () => {
    const inconsistent = chart({
      points: [
        { period: '2026-11-01', stock_in: 10, stock_out: 1 },
        { period: '2026-11-02', stock_in: 10, stock_out: 1 },
      ],
      totals: { stock_in: 7, stock_out: 3 },
    });
    const model = toStockMovementChartModel(inconsistent);
    assert.deepEqual(model.totals, { stock_in: 7, stock_out: 3 });
    assert.deepEqual(model.rows, inconsistent.points);
  });

  it('does not mutate the response (frozen input) and shares no object with it', () => {
    const response = deepFreeze(parse(DAILY_JSON));
    const before = JSON.stringify(response);
    const model = toStockMovementChartModel(response);
    assert.equal(JSON.stringify(response), before);
    assert.notEqual(model.rows, response.points);
    assert.ok(model.rows.every((row, i) => row !== response.points[i]));
    assert.notEqual(model.totals, response.totals);
  });

  it('a later change to the model does not reach the response', () => {
    const response = parse(DAILY_JSON);
    const model = toStockMovementChartModel(response);
    (model.rows[1] as { stock_in: number }).stock_in = 999;
    (model.totals as { stock_in: number }).stock_in = 999;
    assert.equal(response.points[1].stock_in, 120);
    assert.equal(response.totals.stock_in, 120);
  });

  it('is type-safe: the model is read-only and the state is a closed set', () => {
    const model = toStockMovementChartModel(parse(DAILY_JSON));
    const typeChecks = () => {
      // @ts-expect-error rows are read-only
      model.rows.push({ period: '2026-11-04', stock_in: 1, stock_out: 1 });
      // @ts-expect-error the state is read-only
      model.state = 'empty';
      // @ts-expect-error unknown display state
      const unknown: ChartDisplayState = 'loading';
      return unknown;
    };
    assert.equal(typeof typeChecks, 'function'); // compiled by tsc, never run
  });
});
