import { isAdminUser } from '../utils/dashboardTabs'
import { LogoutIcon } from './icons'

// What each role may do, as shown to the signed-in user. Display only: the
// backend authorizes every request.
const ROLE_INFO = {
  admin: {
    label: 'Quản trị viên',
    scope: 'Xem Dashboard, quản lý sản phẩm, nhập và xuất kho.',
  },
  user: {
    label: 'Người dùng',
    scope: 'Xem sản phẩm, nhập và xuất kho.',
  },
}

// Content of the right sidebar: the signed-in account and the logout button,
// kept apart from the navigation so it is never mistaken for a page.
export function AccountPanel({ user, loggingOut, onLogout }) {
  const isAdmin = isAdminUser(user)
  const info = ROLE_INFO[user.role] ?? { label: user.role, scope: '' }
  const initial = (user.email || '?').trim().charAt(0).toUpperCase()

  return (
    <div className="account-panel">
      <p className="account-section-label">Tài khoản</p>

      <div className="account-card">
        <span className="account-avatar" aria-hidden="true">
          {initial}
        </span>
        <div className="account-identity">
          <span className="account-email" title={user.email}>
            {user.email}
          </span>
          <span className={`account-role-pill ${isAdmin ? 'role-admin' : 'role-user'}`}>
            {info.label}
            <span className="account-role-code"> ({user.role})</span>
          </span>
        </div>
      </div>

      {info.scope && <p className="account-scope">{info.scope}</p>}

      <div className="account-actions">
        <button
          type="button"
          className="btn btn-logout"
          onClick={onLogout}
          disabled={loggingOut}
          title="Đăng xuất khỏi hệ thống"
        >
          {loggingOut ? (
            <>
              <span className="spinner" aria-hidden="true"></span>
              <span>Đang đăng xuất...</span>
            </>
          ) : (
            <>
              <LogoutIcon size={18} />
              <span>Đăng xuất</span>
            </>
          )}
        </button>
      </div>
    </div>
  )
}
