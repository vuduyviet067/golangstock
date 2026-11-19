import { useCallback, useEffect, useRef, useState } from 'react'
import { CloseIcon, LogoMark, MenuIcon, UserIcon } from './icons'

const NAV_PANEL_ID = 'app-sidebar'
const ACCOUNT_PANEL_ID = 'account-panel'

// Three-column frame of the signed-in app: navigation on the left, the page in
// the middle, the account on the right. Below 768 px both side columns become
// drawers opened from a top bar; at most one is open at a time. Escape, the
// backdrop and the drawer's close button close it, and focus returns to the
// button that opened it. Above 768 px the drawer state has no effect.
//
// navigation and account are render props that receive closeDrawer, so a
// choice made inside a drawer can close it.
export function AppShell({ navigation, account, children }) {
  const [openDrawer, setOpenDrawer] = useState(null) // 'nav' | 'account' | null
  const navToggle = useRef(null)
  const accountToggle = useRef(null)
  const navClose = useRef(null)
  const accountClose = useRef(null)
  const lastToggle = useRef(null)

  // lastToggle is set only while a drawer is open, so closing when none is
  // open (e.g. a tab click on desktop) moves no focus.
  const closeDrawer = useCallback(() => {
    setOpenDrawer(null)
    const toggle = lastToggle.current
    lastToggle.current = null
    if (toggle) {
      // Return focus once the drawer is hidden again.
      requestAnimationFrame(() => toggle.focus())
    }
  }, [])

  const openNav = () => {
    lastToggle.current = navToggle.current
    setOpenDrawer('nav')
  }
  const openAccount = () => {
    lastToggle.current = accountToggle.current
    setOpenDrawer('account')
  }

  // Move focus into the drawer that just opened, and close it on Escape.
  useEffect(() => {
    if (!openDrawer) return undefined
    const target = openDrawer === 'nav' ? navClose.current : accountClose.current
    target?.focus()
    const onKeyDown = (event) => {
      if (event.key === 'Escape') closeDrawer()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [openDrawer, closeDrawer])

  return (
    <div className="app-shell">
      <header className="mobile-topbar">
        <button
          ref={navToggle}
          type="button"
          className="topbar-btn"
          aria-controls={NAV_PANEL_ID}
          aria-expanded={openDrawer === 'nav'}
          onClick={openNav}
        >
          <MenuIcon />
          <span className="sr-only">Mở menu điều hướng</span>
        </button>
        <span className="topbar-brand">
          <LogoMark size={20} />
          <span>GoStock</span>
        </span>
        <button
          ref={accountToggle}
          type="button"
          className="topbar-btn"
          aria-controls={ACCOUNT_PANEL_ID}
          aria-expanded={openDrawer === 'account'}
          onClick={openAccount}
        >
          <UserIcon />
          <span className="sr-only">Mở thông tin tài khoản</span>
        </button>
      </header>

      <aside
        id={NAV_PANEL_ID}
        className={`app-sidebar ${openDrawer === 'nav' ? 'is-open' : ''}`}
        aria-label="Điều hướng"
      >
        <button ref={navClose} type="button" className="drawer-close-btn" onClick={closeDrawer}>
          <CloseIcon size={18} />
          <span className="sr-only">Đóng menu điều hướng</span>
        </button>
        {navigation(closeDrawer)}
      </aside>

      <div className="app-main">{children}</div>

      <aside
        id={ACCOUNT_PANEL_ID}
        className={`account-sidebar ${openDrawer === 'account' ? 'is-open' : ''}`}
        aria-label="Tài khoản"
      >
        <button ref={accountClose} type="button" className="drawer-close-btn" onClick={closeDrawer}>
          <CloseIcon size={18} />
          <span className="sr-only">Đóng thông tin tài khoản</span>
        </button>
        {account(closeDrawer)}
      </aside>

      {openDrawer && <div className="drawer-backdrop" aria-hidden="true" onClick={closeDrawer} />}
    </div>
  )
}
