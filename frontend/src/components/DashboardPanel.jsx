import { lazy, Suspense, useState } from 'react'
import { ErrorBoundary } from './ErrorBoundary'

// Loads the dashboard code on demand: Vite puts DashboardView in its own
// chunk, which only an admin who opens the Dashboard tab downloads.
const loadDashboardView = () => import('./DashboardView.jsx')

// React.lazy keeps a failed import for good, so a retry needs a new lazy
// component. The latest one is shared by later mounts, so switching tabs
// after a successful load does not show the loading state again.
let currentDashboardView = lazy(loadDashboardView)

function DashboardLoading() {
  return (
    <div className="dashboard-state" role="status">
      <span className="spinner" aria-hidden="true"></span>
      <span>Đang tải Dashboard...</span>
    </div>
  )
}

function DashboardLoadError({ onRetry }) {
  return (
    <div className="alert alert-danger dashboard-alert" role="alert">
      <div className="dashboard-alert-text">
        <strong>Không thể tải Dashboard.</strong>
        <span>Kiểm tra kết nối rồi thử lại, hoặc tải lại trang nếu ứng dụng vừa được cập nhật.</span>
      </div>
      <div className="dashboard-alert-actions">
        <button type="button" className="btn btn-retry" onClick={onRetry}>
          Thử lại
        </button>
        <button type="button" className="btn btn-refresh" onClick={() => window.location.reload()}>
          Tải lại trang
        </button>
      </div>
    </div>
  )
}

export function DashboardPanel({ onReauthenticate }) {
  const [DashboardView, setDashboardView] = useState(() => currentDashboardView)
  const [attempt, setAttempt] = useState(0)

  const retry = () => {
    currentDashboardView = lazy(loadDashboardView)
    setDashboardView(() => currentDashboardView)
    setAttempt((value) => value + 1)
  }

  return (
    <ErrorBoundary key={attempt} name="DashboardPanel" fallback={() => <DashboardLoadError onRetry={retry} />}>
      <Suspense fallback={<DashboardLoading />}>
        <DashboardView onReauthenticate={onReauthenticate} />
      </Suspense>
    </ErrorBoundary>
  )
}
