import { DASHBOARD_ERROR_MESSAGES, dashboardErrorAction } from '../utils/dashboardErrors'

// The error notice of a dashboard request: the message of the error kind and
// the action it allows (Thử lại for network and 5xx, a session or role check
// for 401 and 403, nothing otherwise). Shared by the summary (DashboardView)
// and the stock movement chart (StockMovementSection).

export function DashboardErrorNotice({ kind, onRetry, onReauthenticate }) {
  const message = DASHBOARD_ERROR_MESSAGES[kind] ?? DASHBOARD_ERROR_MESSAGES.unexpected
  const action = dashboardErrorAction(kind)
  const onAction = action === 'retry' ? onRetry : action === 'reauthenticate' ? onReauthenticate : null

  return (
    <div className="alert alert-danger dashboard-alert" role="alert">
      <div className="dashboard-alert-text">
        <strong>{message.title}</strong>
        <span>{message.description}</span>
      </div>
      {onAction && message.actionLabel && (
        <div className="dashboard-alert-actions">
          <button type="button" className="btn btn-retry" onClick={onAction}>
            {message.actionLabel}
          </button>
        </div>
      )}
    </div>
  )
}
