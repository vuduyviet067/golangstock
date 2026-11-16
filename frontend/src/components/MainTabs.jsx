import { useRef } from 'react'
import { MAIN_TAB_LABELS } from '../utils/dashboardTabs'
import { DashboardIcon, ProductsIcon } from './icons'

export const mainTabId = (tab) => `main-tab-${tab}`
export const mainPanelId = (tab) => `main-panel-${tab}`

const TAB_ICONS = {
  dashboard: DashboardIcon,
  products: ProductsIcon,
}

// Main navigation, shown as a vertical list in the left sidebar. Renders only
// the tabs it is given (visibleTabs(user)), so a user never sees a Dashboard
// tab. Arrow keys (up/down and left/right), Home and End move between tabs.
//
// The label is the button's first child and the icon follows it; CSS shows
// the icon first. onActivate (optional) runs on a click or Enter/Space only,
// not on arrow-key moves, so the mobile drawer can close after a deliberate
// choice.
export function MainTabs({ tabs, activeTab, onChange, onActivate }) {
  const buttons = useRef({})

  const select = (tab) => {
    onChange(tab)
    buttons.current[tab]?.focus()
  }

  const handleKeyDown = (event, index) => {
    let next = null
    if (event.key === 'ArrowDown' || event.key === 'ArrowRight') next = tabs[(index + 1) % tabs.length]
    else if (event.key === 'ArrowUp' || event.key === 'ArrowLeft') next = tabs[(index - 1 + tabs.length) % tabs.length]
    else if (event.key === 'Home') next = tabs[0]
    else if (event.key === 'End') next = tabs[tabs.length - 1]
    if (next) {
      event.preventDefault()
      select(next)
    }
  }

  return (
    <nav className="main-tabs-nav" aria-label="Điều hướng chính">
      <div className="main-tabs" role="tablist" aria-orientation="vertical">
        {tabs.map((tab, index) => {
          const selected = tab === activeTab
          const TabIcon = TAB_ICONS[tab]
          return (
            <button
              key={tab}
              ref={(el) => {
                buttons.current[tab] = el
              }}
              type="button"
              role="tab"
              id={mainTabId(tab)}
              className={`main-tab ${selected ? 'active' : ''}`}
              aria-selected={selected}
              aria-controls={mainPanelId(tab)}
              tabIndex={selected ? 0 : -1}
              title={MAIN_TAB_LABELS[tab]}
              onClick={() => {
                onChange(tab)
                onActivate?.(tab)
              }}
              onKeyDown={(event) => handleKeyDown(event, index)}
            >
              {MAIN_TAB_LABELS[tab]}
              {TabIcon && <TabIcon className="main-tab-icon" />}
            </button>
          )
        })}
      </div>
    </nav>
  )
}
