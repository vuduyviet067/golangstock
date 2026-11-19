import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import {
  DateRangeFilter,
  PRESET_LABELS,
  initialRangeFilterState,
  invalidInputs,
  rangeFilterHandlers,
  rangeFilterReducer,
  selectionKey,
} from '../src/components/DateRangeFilter';
import { CUSTOM_RANGE_ERROR_MESSAGES, DASHBOARD_PRESETS, type DashboardPreset } from '../src/utils/dashboardDates';
import { applyCustomRange, defaultRangeSelection, selectPreset, type RangeSelection } from '../src/utils/dashboardRangeSelection';

// Server rendering covers the markup; the handlers and the reducer are
// driven here the way the component wires them (dispatch -> reducer). The
// browser test covers the same flows through real clicks and typing.

const TODAY = '2026-11-12';

// The reducer is plain JavaScript; its state as the component uses it.
type FilterState = {
  appliedKey: string;
  draft: { from: string; to: string } | null;
  error: { code: string; message: string } | null;
};
const reduce = (state: FilterState, action: unknown) => rangeFilterReducer(state, action) as FilterState;

function customSelection(from: string, to: string): RangeSelection {
  const result = applyCustomRange(from, to, TODAY);
  assert.ok(result.ok);
  return result.selection;
}

// The filter's state and handlers for an applied selection, with a counter
// of onApply calls.
function simulate(selection: RangeSelection, today = TODAY) {
  let state: FilterState = initialRangeFilterState(selectionKey(selection)) as FilterState;
  const applied: RangeSelection[] = [];
  let prevented = 0;
  const handlers = () =>
    rangeFilterHandlers({
      selection,
      today,
      state,
      dispatch: (action: unknown) => {
        state = reduce(state, action);
      },
      onApply: (next: RangeSelection) => {
        applied.push(next);
      },
    });
  return {
    get state() {
      return state;
    },
    get prevented() {
      return prevented;
    },
    applied,
    edit(field: 'from' | 'to', value: string) {
      handlers().editDraft(field, value);
    },
    submit() {
      handlers().submitDraft({
        preventDefault() {
          prevented += 1;
        },
      });
    },
    preset(preset: DashboardPreset) {
      handlers().choosePreset(preset);
    },
  };
}

const render = (props: { selection: RangeSelection; today?: string; onApply?: (s: RangeSelection) => void }) =>
  renderToStaticMarkup(createElement(DateRangeFilter, { today: TODAY, onApply: () => {}, ...props }));

const inputs = (html: string) => html.match(/<input [^>]*>/g) ?? [];
const attr = (tag: string | undefined, name: string) => new RegExp(` ${name}="([^"]*)"`).exec(tag ?? '')?.[1];

const globalWithFetch = globalThis as { fetch: typeof fetch };
const originalFetch = globalWithFetch.fetch;
let fetchCalls = 0;
beforeEach(() => {
  fetchCalls = 0;
  globalWithFetch.fetch = async () => {
    fetchCalls += 1;
    throw new Error('DateRangeFilter must not request anything');
  };
});
afterEach(() => {
  globalWithFetch.fetch = originalFetch;
});

describe('DateRangeFilter state', () => {
  it('labels the four presets, in DASHBOARD_PRESETS order', () => {
    assert.deepEqual(Object.keys(PRESET_LABELS), [...DASHBOARD_PRESETS]);
    assert.deepEqual(Object.values(PRESET_LABELS), ['7 ngày', '30 ngày', '90 ngày', '12 tháng']);
  });

  it('selectionKey compares values, not objects', () => {
    const a = selectPreset('30d', TODAY);
    assert.equal(selectionKey(a), selectionKey(selectPreset('30d', TODAY)));
    assert.equal(selectionKey(a), selectionKey(JSON.parse(JSON.stringify(a))));
    const others = [
      selectPreset('7d', TODAY),
      selectPreset('30d', '2026-11-13'),
      // Same days as the 30d preset, but custom (granularity auto).
      customSelection('2026-10-14', TODAY),
      { kind: 'custom', range: { from: '2026-10-14', to: TODAY, granularity: 'day' } } as RangeSelection,
      customSelection('2026-10-15', TODAY),
      customSelection('2026-10-14', '2026-11-11'),
    ];
    const keys = new Set([a, ...others].map(selectionKey));
    assert.equal(keys.size, others.length + 1);
  });

  it('starts with no draft and no error', () => {
    assert.deepEqual(initialRangeFilterState('k'), { appliedKey: 'k', draft: null, error: null });
  });

  it('sync: drops the draft and the error when the applied selection changed, keeps the state otherwise', () => {
    const editing = { appliedKey: 'a', draft: { from: '2026-11-01', to: '' }, error: { code: 'invalid_date', message: 'x' } };
    assert.equal(rangeFilterReducer(editing, { type: 'sync', appliedKey: 'a' }), editing);
    assert.deepEqual(rangeFilterReducer(editing, { type: 'sync', appliedKey: 'b' }), initialRangeFilterState('b'));
  });

  it('edit: the first edit starts from the applied range, later edits keep the other date, and the error goes', () => {
    const base = { from: '2026-10-14', to: TODAY, granularity: 'day' };
    let state = initialRangeFilterState('a') as FilterState;
    state = reduce(state, { type: 'edit', field: 'from', value: '2026-11-01', base });
    assert.deepEqual(state, { appliedKey: 'a', draft: { from: '2026-11-01', to: TODAY }, error: null });
    state = reduce({ ...state, error: { code: 'future', message: 'x' } }, { type: 'edit', field: 'to', value: '2026-11-05', base });
    assert.deepEqual(state, { appliedKey: 'a', draft: { from: '2026-11-01', to: '2026-11-05' }, error: null });
  });

  it('reject keeps the draft; clear drops draft and error, and is a no-op when there is nothing to drop', () => {
    const draft = { from: '2026-11-10', to: '2026-11-01' };
    const error = { code: 'reversed', message: CUSTOM_RANGE_ERROR_MESSAGES.reversed };
    const rejected = rangeFilterReducer({ appliedKey: 'a', draft, error: null }, { type: 'reject', error });
    assert.deepEqual(rejected, { appliedKey: 'a', draft, error });
    assert.deepEqual(rangeFilterReducer(rejected, { type: 'clear' }), initialRangeFilterState('a'));
    const clean = initialRangeFilterState('a');
    assert.equal(rangeFilterReducer(clean, { type: 'clear' }), clean);
    assert.equal(rangeFilterReducer(clean, { type: 'unknown' }), clean);
  });

  it('invalidInputs marks the inputs an error is about', () => {
    const at = (code: string | null, from = TODAY, to = TODAY) => invalidInputs(code && { code, message: '' }, from, to);
    assert.deepEqual(at(null), { from: false, to: false });
    assert.deepEqual(at('invalid_date', '', TODAY), { from: true, to: false });
    assert.deepEqual(at('invalid_date', TODAY, '0999-01-01'), { from: false, to: true });
    assert.deepEqual(at('future'), { from: false, to: true });
    assert.deepEqual(at('reversed'), { from: true, to: true });
    assert.deepEqual(at('too_long'), { from: true, to: true });
  });
});

describe('DateRangeFilter presets', () => {
  it('a preset is applied at once, exactly once per click, with the range of today', () => {
    const expected: Record<DashboardPreset, [string, string, string]> = {
      '7d': ['2026-11-06', TODAY, 'day'],
      '30d': ['2026-10-14', TODAY, 'day'],
      '90d': ['2026-08-15', TODAY, 'day'],
      '12mo': ['2025-12-01', TODAY, 'month'],
    };
    for (const preset of DASHBOARD_PRESETS) {
      const filter = simulate(defaultRangeSelection(TODAY));
      filter.preset(preset);
      assert.equal(filter.applied.length, 1, preset);
      const [from, to, granularity] = expected[preset];
      assert.deepEqual(filter.applied[0], { kind: 'preset', preset, range: { from, to, granularity } });
    }
  });

  it('90 ngày is by day and 12 tháng by month', () => {
    const filter = simulate(defaultRangeSelection(TODAY));
    filter.preset('90d');
    filter.preset('12mo');
    assert.deepEqual(
      filter.applied.map((s) => s.range.granularity),
      ['day', 'month']
    );
  });

  it('a preset uses the today of the click; the applied selection is never recomputed', () => {
    const selection = defaultRangeSelection(TODAY);
    const before = JSON.stringify(selection);
    const filter = simulate(selection, '2026-11-13');
    assert.equal(JSON.stringify(selection), before);
    filter.preset('7d');
    assert.deepEqual(filter.applied[0].range, { from: '2026-11-07', to: '2026-11-13', granularity: 'day' });
  });

  it('a preset drops the custom draft and its error', () => {
    const filter = simulate(defaultRangeSelection(TODAY));
    filter.edit('to', '2026-12-01');
    filter.submit();
    assert.equal(filter.state.error?.code, 'future');
    filter.preset('90d');
    assert.equal(filter.state.draft, null);
    assert.equal(filter.state.error, null);
    assert.equal(filter.applied.length, 1);
  });
});

describe('DateRangeFilter custom range', () => {
  it('editing the dates only changes the draft', () => {
    const filter = simulate(defaultRangeSelection(TODAY));
    filter.edit('from', '2026-11-01');
    filter.edit('to', '2026-11-05');
    filter.edit('to', '');
    filter.edit('to', '2026-11-10');
    assert.equal(filter.applied.length, 0);
    assert.deepEqual(filter.state.draft, { from: '2026-11-01', to: '2026-11-10' });
  });

  it('Áp dụng with a valid range applies it once, with granularity auto, and shows the applied range again', () => {
    const filter = simulate(defaultRangeSelection(TODAY));
    filter.edit('from', '2026-11-01');
    filter.edit('to', '2026-11-05');
    filter.submit();
    assert.equal(filter.prevented, 1);
    assert.deepEqual(filter.applied, [{ kind: 'custom', range: { from: '2026-11-01', to: '2026-11-05', granularity: 'auto' } }]);
    assert.equal(filter.state.draft, null);
    assert.equal(filter.state.error, null);
  });

  it('Áp dụng without a change applies nothing (and still keeps the page from reloading)', () => {
    const filter = simulate(defaultRangeSelection(TODAY));
    filter.submit();
    assert.equal(filter.prevented, 1);
    assert.equal(filter.applied.length, 0);
    assert.equal(filter.state.error, null);
  });

  it('an invalid range shows its Vietnamese message, applies nothing and keeps the draft', () => {
    const cases: [string, string, string, string][] = [
      ['', TODAY, 'invalid_date', 'Ngày không hợp lệ. Vui lòng nhập ngày theo định dạng YYYY-MM-DD.'],
      ['0999-01-01', TODAY, 'invalid_date', 'Ngày không hợp lệ. Vui lòng nhập ngày theo định dạng YYYY-MM-DD.'],
      ['2026-11-10', '2026-11-01', 'reversed', 'Ngày bắt đầu phải trước hoặc bằng ngày kết thúc.'],
      ['2026-11-01', '2026-11-13', 'future', 'Ngày kết thúc không được sau ngày hôm nay.'],
      ['2024-11-11', TODAY, 'too_long', 'Khoảng thời gian tối đa là 731 ngày.'],
    ];
    for (const [from, to, code, message] of cases) {
      const selection = defaultRangeSelection(TODAY);
      const filter = simulate(selection);
      filter.edit('from', from);
      filter.edit('to', to);
      filter.submit();
      assert.equal(filter.applied.length, 0, code);
      assert.deepEqual(filter.state.error, { code, message }, code);
      assert.equal(message, CUSTOM_RANGE_ERROR_MESSAGES[code as keyof typeof CUSTOM_RANGE_ERROR_MESSAGES]);
      assert.deepEqual(filter.state.draft, { from, to }, code);
      // The applied selection is untouched.
      assert.equal(filter.state.appliedKey, selectionKey(selection));
      assert.deepEqual(selection, defaultRangeSelection(TODAY));
    }
  });

  it('731 days is still valid', () => {
    const filter = simulate(defaultRangeSelection(TODAY));
    filter.edit('from', '2024-11-12');
    filter.submit();
    assert.deepEqual(filter.applied, [{ kind: 'custom', range: { from: '2024-11-12', to: TODAY, granularity: 'auto' } }]);
  });

  it('after an error, editing removes the message and the next Áp dụng can succeed', () => {
    const filter = simulate(defaultRangeSelection(TODAY));
    filter.edit('from', '2026-11-10');
    filter.edit('to', '2026-11-01');
    filter.submit();
    filter.submit();
    assert.equal(filter.state.error?.code, 'reversed');
    filter.edit('to', '2026-11-11');
    assert.equal(filter.state.error, null);
    filter.submit();
    assert.deepEqual(filter.applied, [{ kind: 'custom', range: { from: '2026-11-10', to: '2026-11-11', granularity: 'auto' } }]);
  });
});

describe('DateRangeFilter markup', () => {
  it('four preset buttons, the applied one pressed', () => {
    const html = render({ selection: defaultRangeSelection(TODAY) });
    assert.match(html, /<div class="range-filter-presets" role="group" aria-label="Khoảng thời gian">/);
    const buttons = [...html.matchAll(/<button type="button" class="range-filter-preset" aria-pressed="(true|false)">([^<]*)<\/button>/g)];
    assert.deepEqual(
      buttons.map((m) => [m[2], m[1]]),
      [
        ['7 ngày', 'false'],
        ['30 ngày', 'true'],
        ['90 ngày', 'false'],
        ['12 tháng', 'false'],
      ]
    );
  });

  it('labelled date inputs showing the applied range, max today, and Áp dụng disabled with no draft', () => {
    const html = render({ selection: selectPreset('12mo', TODAY) });
    assert.match(html, /aria-pressed="true">12 tháng</);
    const [from, to] = inputs(html);
    for (const [input, label, value] of [
      [from, 'Từ ngày', '2025-12-01'],
      [to, 'Đến ngày', TODAY],
    ] as const) {
      const id = attr(input, 'id');
      assert.ok(id);
      assert.match(html, new RegExp(`<label class="form-label" for="${id}">${label}</label>`));
      assert.equal(attr(input, 'type'), 'date');
      assert.equal(attr(input, 'value'), value);
      assert.equal(attr(input, 'max'), TODAY);
      assert.equal(attr(input, 'class'), 'form-input');
      assert.equal(attr(input, 'aria-invalid'), undefined);
      assert.equal(attr(input, 'aria-describedby'), undefined);
    }
    assert.notEqual(attr(from, 'id'), attr(to, 'id'));
    assert.match(html, /<form class="range-filter-custom" novalidate="">/);
    assert.match(html, /<legend class="range-filter-legend">Khoảng tùy chỉnh<\/legend>/);
    assert.match(html, /<button type="submit" class="btn btn-primary range-filter-apply" disabled="">Áp dụng<\/button>/);
    assert.equal(html.includes('role="alert"'), false);
  });

  it('custom selection: no preset pressed, the custom form marked as applied', () => {
    const html = render({ selection: customSelection('2026-11-01', '2026-11-05') });
    assert.equal(html.includes('aria-pressed="true"'), false);
    assert.match(html, /<form class="range-filter-custom" data-active="" novalidate="">/);
    assert.match(html, /<legend class="range-filter-legend">Khoảng tùy chỉnh<span class="range-filter-badge">Đang áp dụng<\/span><\/legend>/);
    const [from, to] = inputs(html);
    assert.equal(attr(from, 'value'), '2026-11-01');
    assert.equal(attr(to, 'value'), '2026-11-05');
  });

  it('a later today moves nothing: same range, same pressed preset, only max follows today', () => {
    const selection = defaultRangeSelection(TODAY);
    const html = render({ selection, today: '2026-11-13' });
    assert.match(html, /aria-pressed="true">30 ngày</);
    const [from, to] = inputs(html);
    assert.equal(attr(from, 'value'), '2026-10-14');
    assert.equal(attr(to, 'value'), TODAY);
    assert.equal(attr(to, 'max'), '2026-11-13');
  });

  it('two filters on one page get different ids', () => {
    const one = render({ selection: defaultRangeSelection(TODAY) });
    const html = renderToStaticMarkup(
      createElement('div', null, ...[1, 2].map((key) => createElement(DateRangeFilter, { key, selection: defaultRangeSelection(TODAY), today: TODAY, onApply: () => {} })))
    );
    const ids = inputs(html).map((input) => attr(input, 'id'));
    assert.equal(new Set(ids).size, 4);
    assert.ok(inputs(one).length === 2);
  });

  it('rendering applies nothing and requests nothing', () => {
    let calls = 0;
    const onApply = () => {
      calls += 1;
    };
    render({ selection: defaultRangeSelection(TODAY), onApply });
    render({ selection: customSelection('2026-11-01', '2026-11-05'), onApply });
    assert.equal(calls, 0);
    assert.equal(fetchCalls, 0);
  });
});
