import { useCallback, useEffect, useMemo, useState } from 'react'
import { useDashboardRequest } from '../hooks/useDashboardRequest'
import { getStockMovementChart } from '../services/dashboardApi'
import { toStockMovementChartModel } from '../utils/dashboardChart'
import { DASHBOARD_TIMEZONE, todayIn } from '../utils/dashboardDates'
import { defaultRangeSelection, describeRangeValidationError } from '../utils/dashboardRangeSelection'
import { DashboardErrorNotice } from './DashboardErrorNotice'
import { DateRangeFilter } from './DateRangeFilter'
import { StockMovementBarChart } from './StockMovementBarChart'

// Stock movement part of the dashboard: the date range filter and the bar
// chart of the applied range. It owns the applied selection (the last 30
// days at first) and loads its chart independently of the summary, so one
// failing does not hide the other.
//
// The applied range is fixed when it is chosen and never moves on its own.
// Today's date (Asia/Ho_Chi_Minh) is read again every minute, only so that
// the filter's max date and the next preset follow midnight.

export const TODAY_REFRESH_MS = 60_000

/**
 * Loads the chart of a range. A 400 is the backend rejecting the range: it
 * resolves to Vietnamese messages per field (describeRangeValidationError),
 * because the request hook keeps only the error kind. Any other error,
 * aborts included, is rethrown for the hook to classify.
 */
export async function loadStockMovements(range, signal, request = getStockMovementChart) {
  try {
    const chart = await request({ from: range.from, to: range.to, granularity: range.granularity }, { signal })
    return { chart, rejected: null }
  } catch (err) {
    const rejected = describeRangeValidationError(err)
    if (rejected) {
      return { chart: null, rejected }
    }
    throw err
  }
}

/**
 * What the section shows for a request state, and whether it is busy:
 *   loading  - nothing to show yet (first load, or reloading after a 400)
 *   chart    - the last chart, dimmed while the next range loads
 *   rejected - the backend's 400, in Vietnamese
 *   error    - the error notice (an older chart is not shown under it)
 */
export function stockMovementView(state) {
  if (state.status === 'error') {
    return { view: 'error', busy: false }
  }
  const busy = state.status === 'loading'
  const result = state.data
  if (!result || (busy && result.rejected)) {
    return { view: 'loading', busy }
  }
  return { view: result.rejected ? 'rejected' : 'chart', busy }
}

export function StockMovementSection({ onReauthenticate }) {
  const [today, setToday] = useState(() => todayIn(DASHBOARD_TIMEZONE))
  const [selection, setSelection] = useState(() => defaultRangeSelection(today))

  useEffect(() => {
    const timer = setInterval(() => setToday(todayIn(DASHBOARD_TIMEZONE)), TODAY_REFRESH_MS)
    return () => clearInterval(timer)
  }, [])

  const { from, to, granularity } = selection.range
  // Depends on the range values only: useDashboardRequest starts a request
  // (aborting the previous one) whenever the loader changes.
  const load = useCallback((signal) => loadStockMovements({ from, to, granularity }, signal), [from, to, granularity])
  const { state, reload } = useDashboardRequest(load)
  const { view, busy } = stockMovementView(state)
  const chart = state.data?.chart ?? null
  const model = useMemo(() => (chart ? toStockMovementChartModel(chart) : null), [chart])

  return (
    <section className="stock-movement" aria-label="Nhập và xuất kho" aria-busy={busy}>
      <div className="stock-movement-filter">
        <DateRangeFilter selection={selection} today={today} onApply={setSelection} />
      </div>

      {view === 'loading' && (
        <div className="stock-movement-placeholder" role="status">
          <span className="spinner" aria-hidden="true"></span>
          <span>Đang tải biểu đồ nhập xuất kho...</span>
        </div>
      )}

      {view === 'error' && (
        <DashboardErrorNotice kind={state.error} onRetry={reload} onReauthenticate={onReauthenticate} />
      )}

      {view === 'rejected' && (
        <div className="alert alert-danger dashboard-alert" role="alert">
          <div className="dashboard-alert-text">
            <strong>Không thể xem khoảng thời gian này</strong>
            {state.data.rejected.map((item) => (
              <span key={item.field ?? 'range'}>{item.message}</span>
            ))}
          </div>
        </div>
      )}

      {view === 'chart' && (
        <div className="stock-movement-chart" data-stale={busy ? '' : undefined}>
          <StockMovementBarChart model={model} />
          {busy && (
            <p className="stock-movement-updating" role="status">
              <span className="spinner" aria-hidden="true"></span>
              Đang tải khoảng thời gian mới...
            </p>
          )}
        </div>
      )}
    </section>
  )
}
