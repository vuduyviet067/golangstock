import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import {
  CUSTOM_RANGE_ERROR_MESSAGES,
  DASHBOARD_PRESETS,
  DASHBOARD_TIMEZONE,
  MAX_AUTO_DAY_DAYS,
  MAX_DAY_DAYS,
  MAX_MONTH_DAYS,
  addDays,
  addMonths,
  daysInclusive,
  isCalendarDate,
  parseCalendarDate,
  presetRange,
  startOfMonth,
  todayIn,
  validateCustomRange,
  type CustomRangeError,
  type DashboardPreset,
} from '../src/utils/dashboardDates';
import type { StockMovementChartQuery } from '../src/types/dashboard';

const at = (iso: string) => new Date(iso);

describe('test environment', () => {
  it('runs in the time zone set by the runner', (t) => {
    const expected = process.env.DASHBOARD_TEST_TZ;
    if (expected === undefined) {
      t.skip('not started by run-dashboard-tests.mjs');
      return;
    }
    assert.equal(Intl.DateTimeFormat().resolvedOptions().timeZone, expected);
  });
});

describe('constants', () => {
  it('match the backend', () => {
    assert.equal(DASHBOARD_TIMEZONE, 'Asia/Ho_Chi_Minh');
    assert.equal(MAX_AUTO_DAY_DAYS, 62);
    assert.equal(MAX_DAY_DAYS, 92);
    assert.equal(MAX_MONTH_DAYS, 731);
  });
});

describe('todayIn', () => {
  it('switches to the next day at midnight in Ho Chi Minh City (UTC+7)', () => {
    assert.equal(todayIn('Asia/Ho_Chi_Minh', at('2026-11-11T16:59:59.999Z')), '2026-11-11');
    assert.equal(todayIn('Asia/Ho_Chi_Minh', at('2026-11-11T17:00:00Z')), '2026-11-12');
  });

  it('crosses the new year and 29 February', () => {
    assert.equal(todayIn('Asia/Ho_Chi_Minh', at('2026-12-31T16:59:59Z')), '2026-12-31');
    assert.equal(todayIn('Asia/Ho_Chi_Minh', at('2026-12-31T17:00:00Z')), '2027-01-01');
    assert.equal(todayIn('Asia/Ho_Chi_Minh', at('2028-02-28T17:00:00Z')), '2028-02-29');
  });

  it('uses Asia/Ho_Chi_Minh by default', () => {
    assert.equal(todayIn(undefined, at('2026-11-11T17:00:00Z')), '2026-11-12');
  });

  it('uses the given zone, not the machine zone', () => {
    const instant = at('2026-11-11T23:30:00Z');
    assert.equal(todayIn('UTC', instant), '2026-11-11');
    assert.equal(todayIn('Asia/Ho_Chi_Minh', instant), '2026-11-12');
    assert.equal(todayIn('Pacific/Kiritimati', at('2026-11-11T10:00:00Z')), '2026-11-12');
    assert.equal(todayIn('America/Santiago', at('2026-11-12T02:00:00Z')), '2026-11-11');
  });

  it('follows the America/Santiago transitions (midnight skipped, then 23:00 repeated)', () => {
    // 6 Sep 2026: 00:00 -04 jumps to 01:00 -03.
    assert.equal(todayIn('America/Santiago', at('2026-09-06T03:59:59Z')), '2026-09-05');
    assert.equal(todayIn('America/Santiago', at('2026-09-06T04:00:00Z')), '2026-09-06');
    // 5 Apr 2026: 00:00 -03 goes back to 23:00 -04 on 4 Apr.
    assert.equal(todayIn('America/Santiago', at('2026-04-05T02:59:59Z')), '2026-04-04');
    assert.equal(todayIn('America/Santiago', at('2026-04-05T03:00:00Z')), '2026-04-04');
    assert.equal(todayIn('America/Santiago', at('2026-04-05T03:59:59Z')), '2026-04-04');
    assert.equal(todayIn('America/Santiago', at('2026-04-05T04:00:00Z')), '2026-04-05');
  });

  it('follows the America/Havana transitions (midnight repeated, then midnight skipped)', () => {
    // 2 Nov 2025: 01:00 -04 goes back to 00:00 -05, so midnight happens twice.
    assert.equal(todayIn('America/Havana', at('2025-11-02T03:59:59Z')), '2025-11-01');
    assert.equal(todayIn('America/Havana', at('2025-11-02T04:00:00Z')), '2025-11-02');
    assert.equal(todayIn('America/Havana', at('2025-11-02T04:59:59Z')), '2025-11-02');
    assert.equal(todayIn('America/Havana', at('2025-11-02T05:00:00Z')), '2025-11-02');
    // 8 Mar 2026: 00:00 -05 jumps to 01:00 -04.
    assert.equal(todayIn('America/Havana', at('2026-03-08T04:59:59Z')), '2026-03-07');
    assert.equal(todayIn('America/Havana', at('2026-03-08T05:00:00Z')), '2026-03-08');
  });

  it('is computed on every call, not cached', () => {
    assert.equal(todayIn(DASHBOARD_TIMEZONE, at('2026-11-11T12:00:00Z')), '2026-11-11');
    assert.equal(todayIn(DASHBOARD_TIMEZONE, at('2026-11-12T12:00:00Z')), '2026-11-12');
  });

  it('defaults now to the current time', () => {
    const before = todayIn(DASHBOARD_TIMEZONE, new Date());
    const today = todayIn();
    const after = todayIn(DASHBOARD_TIMEZONE, new Date());
    assert.ok(isCalendarDate(today), today);
    assert.ok(today === before || today === after, `${before} <= ${today} <= ${after}`);
  });

  it('rejects an unknown zone and an invalid instant', () => {
    assert.throws(() => todayIn('Mars/Base', at('2026-11-11T00:00:00Z')), RangeError);
    assert.throws(() => todayIn(DASHBOARD_TIMEZONE, new Date(Number.NaN)), RangeError);
  });
});

describe('parseCalendarDate', () => {
  it('accepts real dates', () => {
    assert.deepEqual(parseCalendarDate('2026-11-01'), { year: 2026, month: 11, day: 1 });
    assert.deepEqual(parseCalendarDate('2026-12-31'), { year: 2026, month: 12, day: 31 });
    assert.deepEqual(parseCalendarDate('2028-02-29'), { year: 2028, month: 2, day: 29 });
    assert.deepEqual(parseCalendarDate('2000-02-29'), { year: 2000, month: 2, day: 29 });
    assert.deepEqual(parseCalendarDate('1000-01-01'), { year: 1000, month: 1, day: 1 });
    assert.deepEqual(parseCalendarDate('9999-12-31'), { year: 9999, month: 12, day: 31 });
  });

  const invalid: [string, string][] = [
    ['2026-02-30', 'day missing from February'],
    ['2027-02-29', 'not a leap year'],
    ['2100-02-29', 'century that is not a leap year'],
    ['2026-04-31', 'day missing from April'],
    ['2026-11-00', 'day 0'],
    ['2026-13-01', 'month 13'],
    ['2026-00-10', 'month 0'],
    ['2026-1-5', 'single-digit month and day'],
    ['11/01/2026', 'other format'],
    ['01-11-2026', 'day first'],
    ['0099-01-01', 'year 0-99, read by Date.UTC as 1900-1999'],
    ['0999-12-31', 'year before 1000'],
    ['10000-01-01', 'five-digit year'],
    ['+02026-11-01', 'expanded year'],
    ['', 'empty'],
    [' 2026-11-01', 'leading space'],
    ['2026-11-01 ', 'trailing space'],
    ['2026-11-01\n', 'trailing newline'],
    ['2026-11-01T00:00', 'date and time'],
    ['２０２６-11-01', 'full-width digits'],
  ];
  for (const [value, why] of invalid) {
    it(`rejects ${JSON.stringify(value)} (${why})`, () => {
      assert.equal(parseCalendarDate(value), null);
      assert.equal(isCalendarDate(value), false);
    });
  }

  it('rejects values that are not strings', () => {
    assert.equal(parseCalendarDate(undefined as unknown as string), null);
    assert.equal(parseCalendarDate(20261101 as unknown as string), null);
  });
});

describe('addDays', () => {
  it('crosses month, year and February ends', () => {
    assert.equal(addDays('2026-01-31', 1), '2026-02-01');
    assert.equal(addDays('2026-12-31', 1), '2027-01-01');
    assert.equal(addDays('2028-02-28', 1), '2028-02-29');
    assert.equal(addDays('2028-02-29', 1), '2028-03-01');
    assert.equal(addDays('2026-03-01', -1), '2026-02-28');
    assert.equal(addDays('2027-01-01', -1), '2026-12-31');
  });

  it('handles zero and long spans', () => {
    assert.equal(addDays('2026-11-12', 0), '2026-11-12');
    assert.equal(addDays('2026-11-12', -89), '2026-08-15');
    assert.equal(addDays('2026-11-12', -730), '2024-11-12');
    assert.equal(addDays('2026-11-12', 365), '2027-11-12');
  });

  it('is not affected by daylight saving time days', () => {
    assert.equal(addDays('2026-09-05', 1), '2026-09-06');
    assert.equal(addDays('2026-09-06', 1), '2026-09-07');
    assert.equal(addDays('2026-04-04', 1), '2026-04-05');
    assert.equal(addDays('2025-11-01', 1), '2025-11-02');
    assert.equal(addDays('2025-11-02', 1), '2025-11-03');
    assert.equal(addDays('2026-03-29', 1), '2026-03-30');
  });

  it('rejects invalid input', () => {
    assert.throws(() => addDays('2026-02-30', 1), RangeError);
    assert.throws(() => addDays('2026-11-12', 1.5), RangeError);
    assert.throws(() => addDays('2026-11-12', Number.NaN), RangeError);
    assert.throws(() => addDays('9999-12-31', 1), RangeError);
    assert.throws(() => addDays('1000-01-01', -1), RangeError);
  });
});

describe('addMonths', () => {
  it('clamps to the last day of the target month', () => {
    assert.equal(addMonths('2026-01-31', 1), '2026-02-28');
    assert.equal(addMonths('2028-01-31', 1), '2028-02-29');
    assert.equal(addMonths('2026-03-31', -1), '2026-02-28');
    assert.equal(addMonths('2026-05-31', 1), '2026-06-30');
    assert.equal(addMonths('2026-08-31', -6), '2026-02-28');
    assert.equal(addMonths('2026-01-31', 13), '2027-02-28');
    assert.equal(addMonths('2028-02-29', 12), '2029-02-28');
  });

  it('crosses years in both directions', () => {
    assert.equal(addMonths('2026-12-01', 1), '2027-01-01');
    assert.equal(addMonths('2026-01-01', -1), '2025-12-01');
    assert.equal(addMonths('2026-11-12', -11), '2025-12-12');
    assert.equal(addMonths('2026-01-15', -11), '2025-02-15');
    assert.equal(addMonths('2026-01-01', -24), '2024-01-01');
    assert.equal(addMonths('2026-11-12', 0), '2026-11-12');
  });

  it('rejects invalid input', () => {
    assert.throws(() => addMonths('2026-13-01', 1), RangeError);
    assert.throws(() => addMonths('2026-11-12', 0.5), RangeError);
    assert.throws(() => addMonths('9999-12-01', 1), RangeError);
    assert.throws(() => addMonths('1000-01-31', -1), RangeError);
  });
});

describe('startOfMonth', () => {
  it('returns the first day of the month', () => {
    assert.equal(startOfMonth('2026-11-12'), '2026-11-01');
    assert.equal(startOfMonth('2026-11-01'), '2026-11-01');
    assert.equal(startOfMonth('2028-02-29'), '2028-02-01');
    assert.equal(startOfMonth('2026-12-31'), '2026-12-01');
  });

  it('rejects an invalid date', () => {
    assert.throws(() => startOfMonth('2026-02-30'), RangeError);
  });
});

describe('daysInclusive', () => {
  it('counts both ends', () => {
    assert.equal(daysInclusive('2026-11-12', '2026-11-12'), 1);
    assert.equal(daysInclusive('2026-11-01', '2026-11-30'), 30);
    assert.equal(daysInclusive('2026-01-01', '2026-12-31'), 365);
    assert.equal(daysInclusive('2028-01-01', '2028-12-31'), 366);
  });

  it('counts 29 February', () => {
    assert.equal(daysInclusive('2026-02-01', '2026-03-01'), 29);
    assert.equal(daysInclusive('2028-02-01', '2028-03-01'), 30);
  });

  it('is 0 or less for a reversed range', () => {
    assert.equal(daysInclusive('2026-11-02', '2026-11-01'), 0);
    assert.equal(daysInclusive('2026-11-12', '2026-11-01'), -10);
  });

  it('is not affected by daylight saving time days', () => {
    assert.equal(daysInclusive('2026-09-05', '2026-09-07'), 3);
    assert.equal(daysInclusive('2026-04-04', '2026-04-06'), 3);
    assert.equal(daysInclusive('2025-11-01', '2025-11-03'), 3);
    assert.equal(daysInclusive('2026-03-07', '2026-03-09'), 3);
    assert.equal(daysInclusive('2026-03-28', '2026-03-30'), 3);
  });

  it('rejects invalid dates', () => {
    assert.throws(() => daysInclusive('2026-02-30', '2026-03-01'), RangeError);
    assert.throws(() => daysInclusive('2026-03-01', ''), RangeError);
  });
});

describe('presetRange', () => {
  const today = '2026-11-12';

  it('lists the four presets', () => {
    assert.deepEqual([...DASHBOARD_PRESETS], ['7d', '30d', '90d', '12mo']);
  });

  it('7d: the last 7 days by day', () => {
    assert.deepEqual(presetRange('7d', today), { from: '2026-11-06', to: today, granularity: 'day' });
  });

  it('30d: the last 30 days by day, like the backend default', () => {
    assert.deepEqual(presetRange('30d', today), { from: '2026-10-14', to: today, granularity: 'day' });
  });

  it('90d: the last 90 days, explicitly by day because auto would group them by month', () => {
    const range = presetRange('90d', today);
    assert.deepEqual(range, { from: '2026-08-15', to: today, granularity: 'day' });
    const days = daysInclusive(range.from, range.to);
    assert.equal(days, 90);
    assert.ok(days > MAX_AUTO_DAY_DAYS && days <= MAX_DAY_DAYS);
  });

  it('12mo: twelve whole calendar months by month, ending today', () => {
    const range = presetRange('12mo', today);
    assert.deepEqual(range, { from: '2025-12-01', to: today, granularity: 'month' });
    assert.ok(daysInclusive(range.from, range.to) <= MAX_MONTH_DAYS);
  });

  const expected: Record<string, Record<DashboardPreset, string>> = {
    '2026-01-01': { '7d': '2025-12-26', '30d': '2025-12-03', '90d': '2025-10-04', '12mo': '2025-02-01' },
    '2028-02-29': { '7d': '2028-02-23', '30d': '2028-01-31', '90d': '2027-12-02', '12mo': '2027-03-01' },
    '2026-12-31': { '7d': '2026-12-25', '30d': '2026-12-02', '90d': '2026-10-03', '12mo': '2026-01-01' },
    '2026-03-01': { '7d': '2026-02-23', '30d': '2026-01-31', '90d': '2025-12-02', '12mo': '2025-04-01' },
  };
  for (const [day, froms] of Object.entries(expected)) {
    it(`gives consistent ranges when today is ${day}`, () => {
      for (const preset of DASHBOARD_PRESETS) {
        const range = presetRange(preset, day);
        assert.equal(range.to, day, `${preset} to`);
        assert.equal(range.from, froms[preset], `${preset} from`);
      }
      assert.equal(daysInclusive(presetRange('7d', day).from, day), 7);
      assert.equal(daysInclusive(presetRange('30d', day).from, day), 30);
      assert.equal(daysInclusive(presetRange('90d', day).from, day), 90);
      const twelve = presetRange('12mo', day);
      assert.equal(twelve.from, startOfMonth(twelve.from));
      assert.equal(addMonths(twelve.from, 11), startOfMonth(day));
    });
  }

  it('is a valid chart query', () => {
    const query: StockMovementChartQuery = presetRange('30d', today);
    assert.equal(query.granularity, 'day');
  });

  it('rejects an invalid today or preset', () => {
    assert.throws(() => presetRange('7d', '2026-02-30'), RangeError);
    assert.throws(() => presetRange('1y' as DashboardPreset, today), RangeError);
  });
});

describe('validateCustomRange', () => {
  const today = '2026-11-12';
  const fails = (from: string, to: string, error: CustomRangeError) => {
    assert.deepEqual(validateCustomRange(from, to, today), { ok: false, error });
  };

  it('accepts a range with granularity auto', () => {
    assert.deepEqual(validateCustomRange('2026-11-01', '2026-11-10', today), {
      ok: true,
      range: { from: '2026-11-01', to: '2026-11-10', granularity: 'auto' },
    });
  });

  it('accepts a single day, today included', () => {
    assert.deepEqual(validateCustomRange(today, today, today), {
      ok: true,
      range: { from: today, to: today, granularity: 'auto' },
    });
    assert.equal(validateCustomRange('2026-01-05', '2026-01-05', today).ok, true);
  });

  it('accepts 731 days and rejects 732 (too_long)', () => {
    const from731 = addDays(today, -730);
    assert.equal(daysInclusive(from731, today), 731);
    assert.equal(validateCustomRange(from731, today, today).ok, true);

    const from732 = addDays(today, -731);
    assert.equal(daysInclusive(from732, today), 732);
    fails(from732, today, 'too_long');
  });

  it('rejects invalid dates (invalid_date)', () => {
    fails('2026-02-30', '2026-03-01', 'invalid_date');
    fails('', '2026-11-01', 'invalid_date');
    fails('2026-11-01', '', 'invalid_date');
    fails('2026-11-01', '11/12/2026', 'invalid_date');
    fails('0099-01-01', '2026-11-01', 'invalid_date');
  });

  it('rejects from after to (reversed)', () => {
    fails('2026-11-02', '2026-11-01', 'reversed');
  });

  it('rejects a range ending after today (future)', () => {
    fails('2026-11-01', '2026-11-13', 'future');
    fails('2026-11-13', '2026-11-14', 'future');
  });

  it('reports one error, in the order invalid_date, reversed, future, too_long', () => {
    fails('2026-11-20', '2026-02-30', 'invalid_date');
    fails('2026-11-20', '2026-11-15', 'reversed');
    fails('2020-01-01', '2026-11-13', 'future');
  });

  it('rejects an invalid today', () => {
    assert.throws(() => validateCustomRange('2026-11-01', '2026-11-02', 'today'), RangeError);
  });

  it('has a Vietnamese message for every error', () => {
    assert.deepEqual(Object.keys(CUSTOM_RANGE_ERROR_MESSAGES).sort(), ['future', 'invalid_date', 'reversed', 'too_long']);
    for (const message of Object.values(CUSTOM_RANGE_ERROR_MESSAGES)) {
      assert.ok(message.trim().length > 0);
    }
    assert.match(CUSTOM_RANGE_ERROR_MESSAGES.too_long, /731/);
    assert.match(CUSTOM_RANGE_ERROR_MESSAGES.future, /hôm nay/);
  });
});
