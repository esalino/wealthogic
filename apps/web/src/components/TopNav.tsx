export default function TopNav() {
  return (
    <header className="fixed top-0 left-nav-width right-0 h-16 bg-surface-container-lowest border-b border-outline-variant flex items-center px-6 gap-4 z-20">
      {/* Search */}
      <div className="flex-1 max-w-xl">
        <div className="relative">
          <span className="material-symbols-outlined absolute left-3 top-1/2 -translate-y-1/2 text-on-surface-variant text-xl">search</span>
          <input
            type="text"
            placeholder="Search assets, accounts, transactions..."
            className="w-full pl-10 pr-4 py-2 bg-surface-container-low border border-outline-variant rounded-lg text-body-md text-on-surface placeholder:text-on-surface-variant focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors"
          />
        </div>
      </div>

      {/* Actions */}
      <div className="flex items-center gap-2">
        <button className="w-9 h-9 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors">
          <span className="material-symbols-outlined text-xl">notifications</span>
        </button>
        <button className="w-9 h-9 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors">
          <span className="material-symbols-outlined text-xl">history</span>
        </button>

        <div className="w-px h-6 bg-outline-variant mx-1" />

        {/* User */}
        <div className="hidden md:flex items-center gap-3">
          <div className="text-right">
            <p className="text-body-md font-semibold text-on-surface leading-none">Alex Sterling</p>
            <p className="text-label-sm text-on-surface-variant">Premium Member</p>
          </div>
          <div className="w-9 h-9 rounded-full bg-primary-container flex items-center justify-center flex-shrink-0">
            <span className="text-label-sm font-bold text-on-primary">AS</span>
          </div>
        </div>
      </div>
    </header>
  )
}
