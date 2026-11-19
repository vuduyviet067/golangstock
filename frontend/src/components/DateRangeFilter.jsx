import { useId, useReducer } from 'react'
import { DASHBOARD_PRESETS, isCalendarDate } from '../utils/dashboardDates'
import { applyCustomRange, selectPreset } from '../utils/dashboardRangeSelection'

// Date range filter of the stock movement chart. The parent owns the applied
// selection (a RangeSelection) and today's date (YYYY-MM-DD, normally
// todayIn(DASHBOARD_TIMEZONE)); this component never reads the clock and
// requests nothing. A preset applies at once. The custom dates are a draft:
// editing them calls nothing, Áp dụng checks them with applyCustomRange and
// applies them only when valid. Ranges and checks all come from
// utils/dashboardRangeSelection and utils/dashboardDates.
//
// State: the draft (null while the inputs show the applied range), the error
// of the last Áp dụng, and the key of the applied selection they belong to.
// When the parent applies a selection with other values, the draft and the
// error are dropped; a re-render with the same values, or a new today, keeps
// them.

export const PRESET_LABELS = {
  '7d': '7 ngày',
  '30d': '30 ngày',
  '90d': '90 ngày',
  '12mo': '12 tháng',
}

/** Identifies an applied selection by its values, not by object identity. */
export function selectionKey(selection) {
  const { kind, range } = selection
  return [kind, kind === 'preset' ? selection.preset : '', range.from, range.to, range.granularity].join('|')
}

export function initialRangeFilterState(appliedKey) {
  return { appliedKey, draft: null, error: null }
}

export function rangeFilterReducer(state, action) {
  switch (action.type) {
    // The parent applied a selection with other values.
    case 'sync':
      return state.appliedKey === action.appliedKey ? state : initialRangeFilterState(action.appliedKey)
    // A date input changed. The first edit starts from the applied range.
    case 'edit': {
      const draft = state.draft ?? { from: action.base.from, to: action.base.to }
      return { ...state, draft: { ...draft, [action.field]: action.value }, error: null }
    }
    // Áp dụng with an invalid range: keep the draft so it can be fixed.
    case 'reject':
      return { ...state, error: action.error }
    // A selection was applied from here: show the applied range again.
    case 'clear':
      return state.draft === null && state.error === null ? state : { ...state, draft: null, error: null }
    default:
      return state
  }
}

/**
 * What the controls do. onApply is called from these event handlers only,
 * never from the reducer or during render, so it runs once per click.
 */
export function rangeFilterHandlers({ selection, today, state, dispatch, onApply }) {
  return {
    choosePreset(preset) {
      dispatch({ type: 'clear' })
      onApply(selectPreset(preset, today))
    },
    editDraft(field, value) {
      dispatch({ type: 'edit', field, value, base: selection.range })
    },
    submitDraft(event) {
      event.preventDefault()
      if (state.draft === null) {
        return
      }
      const result = applyCustomRange(state.draft.from, state.draft.to, today)
      if (!result.ok) {
        dispatch({ type: 'reject', error: { code: result.error, message: result.message } })
        return
      }
      dispatch({ type: 'clear' })
      onApply(result.selection)
    },
  }
}

/** The inputs an error is about, to mark them aria-invalid. */
export function invalidInputs(error, from, to) {
  switch (error?.code) {
    case 'invalid_date':
      return { from: !isCalendarDate(from), to: !isCalendarDate(to) }
    case 'future':
      return { from: false, to: true }
    case 'reversed':
    case 'too_long':
      return { from: true, to: true }
    default:
      return { from: false, to: false }
  }
}

export function DateRangeFilter({ selection, today, onApply }) {
  const appliedKey = selectionKey(selection)
  const [stored, dispatch] = useReducer(rangeFilterReducer, appliedKey, initialRangeFilterState)
  // Adjust the state while rendering when the applied selection changed
  // (React's pattern for state derived from a changed prop), and render
  // this pass from the reset state already.
  if (stored.appliedKey !== appliedKey) {
    dispatch({ type: 'sync', appliedKey })
  }
  const state = stored.appliedKey === appliedKey ? stored : initialRangeFilterState(appliedKey)
  const { choosePreset, editDraft, submitDraft } = rangeFilterHandlers({ selection, today, state, dispatch, onApply })

  const id = useId()
  const fromId = `${id}-from`
  const toId = `${id}-to`
  const errorId = `${id}-error`

  const from = state.draft ? state.draft.from : selection.range.from
  const to = state.draft ? state.draft.to : selection.range.to
  const invalid = invalidInputs(state.error, from, to)
  const describedBy = state.error ? errorId : undefined
  const isCustom = selection.kind === 'custom'

  return (
    <div className="range-filter">
      <div className="range-filter-presets" role="group" aria-label="Khoảng thời gian">
        {DASHBOARD_PRESETS.map((preset) => (
          <button
            key={preset}
            type="button"
            className="range-filter-preset"
            aria-pressed={selection.kind === 'preset' && selection.preset === preset}
            onClick={() => choosePreset(preset)}
          >
            {PRESET_LABELS[preset]}
          </button>
        ))}
      </div>

      {/* noValidate: the browser must not block Áp dụng with its own message
          (max is only a hint for the date picker); applyCustomRange checks. */}
      <form className="range-filter-custom" data-active={isCustom ? '' : undefined} onSubmit={submitDraft} noValidate>
        <fieldset className="range-filter-fieldset">
          <legend className="range-filter-legend">
            Khoảng tùy chỉnh
            {isCustom && <span className="range-filter-badge">Đang áp dụng</span>}
          </legend>
          <div className="range-filter-fields">
            <div className="range-filter-field">
              <label className="form-label" htmlFor={fromId}>
                Từ ngày
              </label>
              <input
                id={fromId}
                className={invalid.from ? 'form-input has-error' : 'form-input'}
                type="date"
                value={from}
                max={today}
                aria-invalid={invalid.from || undefined}
                aria-describedby={describedBy}
                onChange={(event) => editDraft('from', event.target.value)}
              />
            </div>
            <div className="range-filter-field">
              <label className="form-label" htmlFor={toId}>
                Đến ngày
              </label>
              <input
                id={toId}
                className={invalid.to ? 'form-input has-error' : 'form-input'}
                type="date"
                value={to}
                max={today}
                aria-invalid={invalid.to || undefined}
                aria-describedby={describedBy}
                onChange={(event) => editDraft('to', event.target.value)}
              />
            </div>
            {/* Disabled until a date is changed: there is nothing else to apply. */}
            <button type="submit" className="btn btn-primary range-filter-apply" disabled={state.draft === null}>
              Áp dụng
            </button>
          </div>
          {state.error && (
            <p id={errorId} className="field-error range-filter-error" role="alert">
              {state.error.message}
            </p>
          )}
        </fieldset>
      </form>
    </div>
  )
}
