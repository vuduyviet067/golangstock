import { MainTabs } from './MainTabs'
import { LogoMark } from './icons'

// Content of the left sidebar: the brand, then the main navigation.
export function SidebarNav({ tabs, activeTab, onChange, onActivate }) {
  return (
    <div className="sidebar-nav">
      <div className="sidebar-brand">
        <span className="sidebar-brand-mark">
          <LogoMark size={22} />
        </span>
        <span className="sidebar-brand-text">
          <span className="sidebar-brand-name">GoStock</span>
          <span className="sidebar-brand-tagline">Quản lý kho hàng</span>
        </span>
      </div>

      <p className="sidebar-section-label" aria-hidden="true">
        Danh mục
      </p>
      <MainTabs tabs={tabs} activeTab={activeTab} onChange={onChange} onActivate={onActivate} />
    </div>
  )
}
