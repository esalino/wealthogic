import { NavLink } from 'react-router-dom'
import { useState } from 'react'

interface NavItem {
  to: string
  icon: string
  label: string
}

const navItems: NavItem[] = [
  { to: '/dashboard', icon: 'dashboard', label: 'Dashboard' },
  { to: '/accounts', icon: 'account_balance', label: 'Accounts' },
  { to: '/upload', icon: 'cloud_upload', label: 'Upload Center' },
  { to: '/portfolio', icon: 'pie_chart', label: 'Breakdown' },
  { to: '/tax', icon: 'receipt_long', label: 'Tax Center' },
]

interface SidebarProps {
  pinned: boolean
  onPinnedChange: (pinned: boolean) => void
}

export default function Sidebar({ pinned, onPinnedChange }: SidebarProps) {
  // When not pinned the rail shows icons only; hovering pops the full nav out
  // as an overlay (it doesn't shift page content, which stays offset by the rail).
  const [hovered, setHovered] = useState(false)
  // Clicking collapse happens while the cursor is still over the nav, so without
  // this the lingering hover would keep it popped out. Suppress the current hover
  // session until the cursor leaves and a fresh hover begins.
  const [suppressHover, setSuppressHover] = useState(false)
  const expanded = pinned || (hovered && !suppressHover)

  function handleToggle() {
    if (pinned) {
      onPinnedChange(false)
      setSuppressHover(true)
    } else {
      onPinnedChange(true)
    }
  }

  const linkClasses = (isActive: boolean) =>
    `flex items-center ${expanded ? 'gap-3 px-3 justify-start' : 'justify-center px-0'} py-2.5 rounded-lg text-body-md font-medium transition-colors ${
      isActive
        ? 'text-secondary font-bold bg-surface-container-high'
        : 'text-on-surface-variant hover:bg-surface-container-high'
    }`

  return (
    <aside
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => { setHovered(false); setSuppressHover(false) }}
      className={`fixed top-0 left-0 h-screen ${expanded ? 'w-nav-width' : 'w-20'} bg-surface-container-lowest border-r border-outline-variant flex flex-col z-30 overflow-hidden transition-[width] duration-200 ease-out ${
        hovered && !pinned ? 'shadow-2xl' : ''
      }`}
    >
      {/* Logo / header */}
      <div className={`h-16 flex items-center border-b border-outline-variant flex-shrink-0 ${expanded ? 'px-6 justify-between' : 'px-0 justify-center'}`}>
        <div className="flex items-center gap-2 min-w-0">
          <div className="w-8 h-8 rounded-lg bg-primary flex items-center justify-center flex-shrink-0">
            <span className="material-symbols-outlined text-on-primary text-sm">account_balance</span>
          </div>
          {expanded && (
            <div className="min-w-0">
              <p className="text-headline-sm text-on-surface leading-none whitespace-nowrap">Wealthogic</p>
              <p className="text-label-sm text-on-surface-variant whitespace-nowrap">Portfolio Tracker</p>
            </div>
          )}
        </div>
        {expanded && (
          <button
            onClick={handleToggle}
            className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors flex-shrink-0"
            aria-label={pinned ? 'Collapse sidebar' : 'Pin sidebar open'}
            title={pinned ? 'Collapse' : 'Pin open'}
          >
            <span className="material-symbols-outlined text-xl">{pinned ? 'left_panel_close' : 'keep'}</span>
          </button>
        )}
      </div>

      {/* Nav items */}
      <nav className="flex-1 px-3 py-4 space-y-1">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            title={item.label}
            className={({ isActive }) => linkClasses(isActive)}
          >
            <span className="material-symbols-outlined text-xl flex-shrink-0 w-8 text-center">{item.icon}</span>
            {expanded && <span className="whitespace-nowrap">{item.label}</span>}
          </NavLink>
        ))}
      </nav>

      {/* Bottom links */}
      <div className="px-3 py-3 border-t border-outline-variant space-y-1 flex-shrink-0">
        <NavLink to="/settings" title="Settings" className={({ isActive }) => linkClasses(isActive)}>
          <span className="material-symbols-outlined text-xl flex-shrink-0 w-8 text-center">settings</span>
          {expanded && <span className="whitespace-nowrap">Settings</span>}
        </NavLink>
        <NavLink to="/support" title="Support" className={({ isActive }) => linkClasses(isActive)}>
          <span className="material-symbols-outlined text-xl flex-shrink-0 w-8 text-center">help_outline</span>
          {expanded && <span className="whitespace-nowrap">Support</span>}
        </NavLink>

        {/* User profile — min-height keeps the row the same height whether or not
            the two-line name text is shown, so the bottom block doesn't shift. */}
        <div className={`flex items-center py-2 mt-2 min-h-[52px] ${expanded ? 'gap-3 px-3' : 'justify-center px-0'}`}>
          <div className="w-8 h-8 rounded-full bg-primary-container flex items-center justify-center flex-shrink-0">
            <span className="text-label-sm font-bold text-on-primary">AS</span>
          </div>
          {expanded && (
            <div className="min-w-0">
              <p className="text-body-md font-semibold text-on-surface truncate">Alex Sterling</p>
              <p className="text-label-sm text-on-surface-variant truncate">Premium Member</p>
            </div>
          )}
        </div>
      </div>
    </aside>
  )
}
