import { useMemo } from 'react'
import { useDashboardRequest } from '../hooks/useDashboardRequest'
import { getDashboardSummary } from '../services/dashboardApi'
import { toInventoryStatusPieModel } from '../utils/dashboardInventoryStatus'
import { DashboardErrorNotice } from './DashboardErrorNotice'
import { SummaryCards } from './SummaryCards'
import { InventoryStatusPie } from './InventoryStatusPie'
import { StockMovementSection } from './StockMovementSection'

// Dashboard tab (admin only, loaded lazily by DashboardPanel). It loads the
// summary and shows it as cards and a status pie; below, the stock movement
// section loads its chart on its own, whatever happens to the summary.

// Kept here as well: DashboardErrorNotice used to live in this file.
export { DashboardErrorNotice }

// Stable loader: useDashboardRequest restarts whenever it changes.
const loadSummary = (signal) => getDashboardSummary({ signal })

export default function DashboardView({ onReauthenticate }) {
  const { state, reload } = useDashboardRequest(loadSummary)
  // Rebuilt only when a new summary arrives, so the pie is not redrawn on
  // unrelated renders.
  const pieModel = useMemo(() => (state.data ? toInventoryStatusPieModel(state.data) : null), [state.data])

  return (
    <section className="dashboard-view" aria-labelledby="dashboard-title">
      <h2 id="dashboard-title" className="dashboard-title">
        Tổng quan tồn kho
      </h2>

      {state.status === 'loading' && (
        <div className="dashboard-state" role="status">
          <span className="spinner" aria-hidden="true"></span>
          <span>Đang tải số liệu...</span>
        </div>
      )}

      {state.status === 'error' && (
        <DashboardErrorNotice kind={state.error} onRetry={reload} onReauthenticate={onReauthenticate} />
      )}

      {state.status === 'success' && state.data && (
        <>
          <SummaryCards summary={state.data} />
          <InventoryStatusPie model={pieModel} />
        </>
      )}

      <StockMovementSection onReauthenticate={onReauthenticate} />
    </section>
  )
}
