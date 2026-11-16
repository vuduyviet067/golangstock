import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { ApiError } from '../src/types/api';
import { CUSTOM_RANGE_ERROR_MESSAGES, DASHBOARD_PRESETS, addDays, daysInclusive, presetRange } from '../src/utils/dashboardDates';
import {
  DEFAULT_RANGE_PRESET,
  GENERIC_RANGE_ERROR_MESSAGE,
  applyCustomRange,
  defaultRangeSelection,
  describeRangeValidationError,
  selectPreset,
  type RangeSelection,
} from '../src/utils/dashboardRangeSelection';

// "today" is always passed in, so nothing here depends on the clock.
const TODAY = '2026-11-12';

// 400 bodies asserted by backend/internal/handle/http/dashboard_handler_test.go.
const badRequest = (fields: unknown) =>
  new ApiError('invalid query parameters', 400, { success: false, message: 'invalid query parameters', data: { fields } });

describe('preset selection', () => {
  it('defaults to the last 30 days, by day', () => {
    assert.equal(DEFAULT_RANGE_PRESET, '30d');
    assert.deepEqual(defaultRangeSelection(TODAY), {
      kind: 'preset',
      preset: '30d',
      range: { from: '2026-10-14', to: TODAY, granularity: 'day' },
    });
    assert.equal(daysInclusive(defaultRangeSelection(TODAY).range.from, TODAY), 30);
  });

  it('selects each of the four presets with the range of presetRange', () => {
    assert.deepEqual([...DASHBOARD_PRESETS], ['7d', '30d', '90d', '12mo']);
    for (const preset of DASHBOARD_PRESETS) {
      assert.deepEqual(selectPreset(preset, TODAY), { kind: 'preset', preset, range: presetRange(preset, TODAY) });
    }
    assert.deepEqual(selectPreset('7d', TODAY).range, { from: '2026-11-06', to: TODAY, granularity: 'day' });
    assert.deepEqual(selectPreset('12mo', TODAY).range, { from: '2025-12-01', to: TODAY, granularity: 'month' });
  });

  it('90d is always by day and 12mo always by month, whatever today is', () => {
    for (const today of ['2026-01-01', '2026-03-01', '2026-12-31', '2028-02-29', TODAY]) {
      const ninety = selectPreset('90d', today).range;
      assert.equal(ninety.granularity, 'day', today);
      assert.equal(daysInclusive(ninety.from, today), 90, today);
      assert.equal(selectPreset('12mo', today).range.granularity, 'month', today);
    }
  });

  it('resolves the range when selected: it is plain data that does not follow the clock', () => {
    const selection = selectPreset('7d', TODAY);
    assert.deepEqual(JSON.parse(JSON.stringify(selection)), selection);
    // Another day gives another range; this selection keeps its own.
    assert.notEqual(selectPreset('7d', '2026-11-13').range.from, selection.range.from);
    assert.equal(selection.range.from, '2026-11-06');
  });

  it('rejects an invalid today, like presetRange', () => {
    assert.throws(() => selectPreset('7d', '2026-02-30'), RangeError);
    assert.throws(() => defaultRangeSelection(''), RangeError);
  });
});

describe('applyCustomRange', () => {
  it('a valid range is selected with granularity auto', () => {
    assert.deepEqual(applyCustomRange('2026-11-01', '2026-11-10', TODAY), {
      ok: true,
      selection: { kind: 'custom', range: { from: '2026-11-01', to: '2026-11-10', granularity: 'auto' } },
    });
  });

  it('a single day, today included, is valid', () => {
    const result = applyCustomRange(TODAY, TODAY, TODAY);
    assert.ok(result.ok);
    assert.deepEqual(result.selection.range, { from: TODAY, to: TODAY, granularity: 'auto' });
  });

  it('a long valid range keeps granularity auto (the backend picks month)', () => {
    const result = applyCustomRange(addDays(TODAY, -730), TODAY, TODAY);
    assert.ok(result.ok);
    assert.equal(result.selection.range.granularity, 'auto');
  });

  const fails = (from: string, to: string, error: keyof typeof CUSTOM_RANGE_ERROR_MESSAGES) => {
    assert.deepEqual(applyCustomRange(from, to, TODAY), { ok: false, error, message: CUSTOM_RANGE_ERROR_MESSAGES[error] });
  };

  it('invalid date', () => {
    fails('2026-02-30', '2026-03-01', 'invalid_date');
    fails('', TODAY, 'invalid_date');
    fails('2026-11-01', '12/11/2026', 'invalid_date');
  });

  it('reversed range', () => {
    fails('2026-11-02', '2026-11-01', 'reversed');
  });

  it('future date', () => {
    fails('2026-11-01', '2026-11-13', 'future');
  });

  it('range over the limit (732 days)', () => {
    fails(addDays(TODAY, -731), TODAY, 'too_long');
  });

  it('messages are the Vietnamese ones of dashboardDates', () => {
    const result = applyCustomRange('2026-11-02', '2026-11-01', TODAY);
    assert.equal(result.ok, false);
    assert.equal(!result.ok && result.message, 'Ngày bắt đầu phải trước hoặc bằng ngày kết thúc.');
  });
});

describe('describeRangeValidationError', () => {
  const only = (field: string, text: string) => describeRangeValidationError(badRequest({ [field]: text }));

  it('translates every confirmed backend message', () => {
    assert.deepEqual(only('from', 'must be a date in YYYY-MM-DD format'), [{ field: 'from', message: 'Ngày bắt đầu không hợp lệ.' }]);
    assert.deepEqual(only('to', 'must be a date in YYYY-MM-DD format'), [{ field: 'to', message: 'Ngày kết thúc không hợp lệ.' }]);
    assert.deepEqual(only('from', 'must be on or before to'), [{ field: 'from', message: CUSTOM_RANGE_ERROR_MESSAGES.reversed }]);
    assert.deepEqual(only('to', 'range must be at most 92 days by day or 731 days by month'), [
      { field: 'to', message: 'Khoảng thời gian quá dài: tối đa 92 ngày khi xem theo ngày hoặc 731 ngày khi xem theo tháng.' },
    ]);
    assert.deepEqual(only('granularity', 'must be auto, day or month'), [{ field: 'granularity', message: 'Cách gộp dữ liệu không hợp lệ.' }]);
    assert.deepEqual(only('tz', 'must be an IANA time zone name'), [{ field: 'tz', message: 'Múi giờ không hợp lệ.' }]);
  });

  it('a known message on another field is not reused there', () => {
    assert.deepEqual(only('to', 'must be on or before to'), [{ field: 'to', message: 'Ngày kết thúc không được máy chủ chấp nhận.' }]);
  });

  it('falls back to a message for the field when the text is unknown', () => {
    assert.deepEqual(only('from', 'something new'), [{ field: 'from', message: 'Ngày bắt đầu không được máy chủ chấp nhận.' }]);
    assert.deepEqual(only('tz', 'unknown zone'), [{ field: 'tz', message: 'Múi giờ không được máy chủ chấp nhận.' }]);
  });

  it('is not fooled by prototype names', () => {
    assert.deepEqual(only('from', 'toString'), [{ field: 'from', message: 'Ngày bắt đầu không được máy chủ chấp nhận.' }]);
    assert.deepEqual(only('to', '__proto__'), [{ field: 'to', message: 'Ngày kết thúc không được máy chủ chấp nhận.' }]);
  });

  it('lists several fields in the order from, to, granularity, tz', () => {
    const messages = describeRangeValidationError(badRequest({
      tz: 'must be an IANA time zone name',
      from: 'must be a date in YYYY-MM-DD format',
      granularity: 'must be auto, day or month',
    }));
    assert.deepEqual(messages?.map((m) => m.field), ['from', 'granularity', 'tz']);
  });

  it('falls back to a generic message for a 400 without a known field', () => {
    const generic = [{ field: null, message: GENERIC_RANGE_ERROR_MESSAGE }];
    assert.deepEqual(describeRangeValidationError(badRequest({ sku: 'must be set' })), generic);
    assert.deepEqual(describeRangeValidationError(badRequest({})), generic);
    assert.deepEqual(describeRangeValidationError(new ApiError('bad', 400, { success: false, message: 'bad', data: null })), generic);
    assert.deepEqual(describeRangeValidationError(new ApiError('bad', 400)), generic);
    assert.deepEqual(describeRangeValidationError(badRequest({ from: 42 })), generic);
  });

  it('never shows the English text of the backend', () => {
    const all = describeRangeValidationError(badRequest({
      from: 'must be on or before to',
      to: 'range must be at most 92 days by day or 731 days by month',
      granularity: 'must be auto, day or month',
      tz: 'must be an IANA time zone name',
    })) ?? [];
    assert.equal(all.length, 4);
    for (const { message } of all) assert.equal(/must|range|IANA|format/.test(message), false, message);
  });

  it('returns null for anything but a 400', () => {
    for (const status of [0, 401, 403, 404, 500]) {
      assert.equal(describeRangeValidationError(new ApiError('x', status, { data: { fields: { from: 'x' } } })), null, String(status));
    }
    assert.equal(describeRangeValidationError(new Error('x')), null);
    assert.equal(describeRangeValidationError(null), null);
  });

  it('types: a selection is read-only', () => {
    const selection: RangeSelection = defaultRangeSelection(TODAY);
    const typeChecks = () => {
      // @ts-expect-error the range of a selection is read-only
      selection.range.from = '2026-01-01';
      // @ts-expect-error the kind is read-only
      selection.kind = 'custom';
    };
    assert.equal(typeof typeChecks, 'function'); // compiled by tsc, never run
  });
});
