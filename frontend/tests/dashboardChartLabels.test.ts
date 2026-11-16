import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import {
  CHART_GRANULARITY_LABELS,
  formatChartAxisLabel,
  formatChartGranularity,
  formatChartRange,
  formatChartTooltipLabel,
} from '../src/utils/dashboardChartLabels';
import type { StockMovementGranularity } from '../src/types/dashboard';

// The runner executes this file in UTC, America/Santiago and
// Pacific/Kiritimati: a label computed through a local Date would come out
// one day off in at least one of them.

const INVALID: [unknown, string][] = [
  ['', 'empty'],
  ['2026-13-01', 'month 13'],
  ['2027-02-29', 'not a leap year'],
  ['12/11/2026', 'other format'],
  [' 2026-11-12', 'leading space'],
  ['2026-11-12T00:00:00Z', 'date and time'],
  ['0099-01-01', 'year before 1000'],
];

describe('formatChartAxisLabel', () => {
  it('by day: DD/MM', () => {
    assert.equal(formatChartAxisLabel('2026-11-12', 'day'), '12/11');
    assert.equal(formatChartAxisLabel('2026-11-01', 'day'), '01/11');
  });

  it('by month: MM/YYYY', () => {
    assert.equal(formatChartAxisLabel('2026-11-01', 'month'), '11/2026');
    assert.equal(formatChartAxisLabel('2026-01-01', 'month'), '01/2026');
  });

  it('year ends, December and 29 February', () => {
    assert.equal(formatChartAxisLabel('2026-12-31', 'day'), '31/12');
    assert.equal(formatChartAxisLabel('2027-01-01', 'day'), '01/01');
    assert.equal(formatChartAxisLabel('2026-12-01', 'month'), '12/2026');
    assert.equal(formatChartAxisLabel('2027-01-01', 'month'), '01/2027');
    assert.equal(formatChartAxisLabel('2028-02-29', 'day'), '29/02');
    assert.equal(formatChartAxisLabel('2028-02-01', 'month'), '02/2028');
  });

  for (const [value, why] of INVALID) {
    it(`returns an invalid period unchanged (${why})`, () => {
      assert.equal(formatChartAxisLabel(value as string, 'day'), value);
      assert.equal(formatChartAxisLabel(value as string, 'month'), value);
    });
  }

  it('returns the period unchanged for an unknown granularity', () => {
    assert.equal(formatChartAxisLabel('2026-11-12', 'week' as StockMovementGranularity), '2026-11-12');
  });
});

describe('formatChartTooltipLabel', () => {
  it('by day: DD/MM/YYYY, no weekday', () => {
    assert.equal(formatChartTooltipLabel('2026-11-12', 'day'), '12/11/2026');
    assert.match(formatChartTooltipLabel('2026-11-12', 'day'), /^\d{2}\/\d{2}\/\d{4}$/);
  });

  it('by month: Tháng MM/YYYY', () => {
    assert.equal(formatChartTooltipLabel('2026-11-01', 'month'), 'Tháng 11/2026');
  });

  it('year ends, December and 29 February', () => {
    assert.equal(formatChartTooltipLabel('2026-12-31', 'day'), '31/12/2026');
    assert.equal(formatChartTooltipLabel('2027-01-01', 'day'), '01/01/2027');
    assert.equal(formatChartTooltipLabel('2026-12-01', 'month'), 'Tháng 12/2026');
    assert.equal(formatChartTooltipLabel('2027-01-01', 'month'), 'Tháng 01/2027');
    assert.equal(formatChartTooltipLabel('2028-02-29', 'day'), '29/02/2028');
  });

  for (const [value, why] of INVALID) {
    it(`returns an invalid period unchanged (${why})`, () => {
      assert.equal(formatChartTooltipLabel(value as string, 'day'), value);
      assert.equal(formatChartTooltipLabel(value as string, 'month'), value);
    });
  }

  it('returns the period unchanged for an unknown granularity', () => {
    assert.equal(formatChartTooltipLabel('2026-11-12', 'week' as StockMovementGranularity), '2026-11-12');
  });
});

describe('formatChartRange', () => {
  it('DD/MM/YYYY – DD/MM/YYYY, both days as given', () => {
    assert.equal(formatChartRange('2026-11-01', '2026-11-30'), '01/11/2026 – 30/11/2026');
    assert.equal(formatChartRange('2025-12-01', '2026-11-12'), '01/12/2025 – 12/11/2026');
    assert.equal(formatChartRange('2028-02-29', '2028-02-29'), '29/02/2028 – 29/02/2028');
  });

  it('a partial first or last month keeps the requested days', () => {
    assert.equal(formatChartRange('2026-01-15', '2026-03-20'), '15/01/2026 – 20/03/2026');
  });

  it('shows an invalid bound unchanged', () => {
    assert.equal(formatChartRange('2026-02-30', '2026-03-01'), '2026-02-30 – 01/03/2026');
    assert.equal(formatChartRange('2026-03-01', ''), '01/03/2026 – ');
  });
});

describe('formatChartGranularity', () => {
  it('Theo ngày / Theo tháng', () => {
    assert.equal(formatChartGranularity('day'), 'Theo ngày');
    assert.equal(formatChartGranularity('month'), 'Theo tháng');
    assert.deepEqual(CHART_GRANULARITY_LABELS, { day: 'Theo ngày', month: 'Theo tháng' });
  });

  it('returns any other value unchanged, prototype names included', () => {
    assert.equal(formatChartGranularity('auto' as StockMovementGranularity), 'auto');
    assert.equal(formatChartGranularity('toString' as StockMovementGranularity), 'toString');
  });
});
