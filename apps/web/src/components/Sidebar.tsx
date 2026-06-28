import { NavLink } from 'react-router-dom'

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
]

interface SidebarProps {
  onAddAccount: () => void
}

export default function Sidebar({ onAddAccount }: SidebarProps) {
  return (
    <aside className="fixed top-0 left-0 h-screen w-nav-width bg-surface-container-lowest border-r border-outline-variant flex flex-col z-30">
      {/* Logo */}
      <div className="px-6 py-5 border-b border-outline-variant">
        <div className="flex items-center gap-2">
          <div className="w-8 h-8 rounded-lg bg-primary flex items-center justify-center">
            <span className="material-symbols-outlined text-on-primary text-sm">account_balance</span>
          </div>
          <div>
            <p className="text-headline-sm text-on-surface leading-none">Wealthogic</p>
            <p className="text-label-sm text-on-surface-variant">Personal Finance</p>
          </div>
        </div>
      </div>

      {/* Nav items */}
      <nav className="flex-1 px-3 py-4 space-y-1">
        <p className="text-label-caps text-on-surface-variant px-3 mb-2 uppercase">Menu</p>
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            className={({ isActive }) =>
              `flex items-center gap-3 px-3 py-2.5 rounded-lg text-body-md font-medium transition-colors ${
                isActive
                  ? 'text-secondary font-bold bg-surface-container-high'
                  : 'text-on-surface-variant hover:bg-surface-container-high'
              }`
            }
          >
            <span className="material-symbols-outlined text-xl">{item.icon}</span>
            {item.label}
          </NavLink>
        ))}
      </nav>

      {/* Add Account button */}
      <div className="px-3 pb-2">
        <button
          onClick={onAddAccount}
          className="w-full flex items-center justify-center gap-2 px-4 py-2.5 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity"
        >
          <span className="material-symbols-outlined text-xl">add_circle</span>
          Add Account
        </button>
      </div>

      {/* Bottom links */}
      <div className="px-3 py-3 border-t border-outline-variant space-y-1">
        <NavLink
          to="/settings"
          className="flex items-center gap-3 px-3 py-2 rounded-lg text-body-md text-on-surface-variant hover:bg-surface-container-high transition-colors"
        >
          <span className="material-symbols-outlined text-xl">settings</span>
          Settings
        </NavLink>
        <NavLink
          to="/support"
          className="flex items-center gap-3 px-3 py-2 rounded-lg text-body-md text-on-surface-variant hover:bg-surface-container-high transition-colors"
        >
          <span className="material-symbols-outlined text-xl">help_outline</span>
          Support
        </NavLink>

        {/* User profile */}
        <div className="flex items-center gap-3 px-3 py-2 mt-2">
          <div className="w-8 h-8 rounded-full bg-primary-container flex items-center justify-center flex-shrink-0">
            <span className="text-label-sm font-bold text-on-primary">AS</span>
          </div>
          <div className="min-w-0">
            <p className="text-body-md font-semibold text-on-surface truncate">Alex Sterling</p>
            <p className="text-label-sm text-on-surface-variant truncate">Premium Member</p>
          </div>
        </div>
      </div>
    </aside>
  )
}
