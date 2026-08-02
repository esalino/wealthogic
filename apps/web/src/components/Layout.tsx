import { Outlet } from 'react-router-dom'
import { useEffect, useState } from 'react'
import Sidebar from './Sidebar'
import TopNav from './TopNav'

// Context so child pages (Accounts) can open the add-account modal via sidebar
import { createContext, useContext } from 'react'

interface LayoutContextValue {
  openAddAccount: () => void
}

export const LayoutContext = createContext<LayoutContextValue>({
  openAddAccount: () => undefined,
})

export function useLayoutContext() {
  return useContext(LayoutContext)
}

export default function Layout() {
  const [addAccountOpen, setAddAccountOpen] = useState(false)
  const [navPinned, setNavPinned] = useState(() => {
    const saved = localStorage.getItem('navPinned')
    return saved === null ? true : saved === 'true'
  })

  useEffect(() => {
    localStorage.setItem('navPinned', String(navPinned))
  }, [navPinned])

  return (
    <LayoutContext.Provider value={{ openAddAccount: () => setAddAccountOpen(true) }}>
      <div className="min-h-screen bg-surface">
        <Sidebar pinned={navPinned} onPinnedChange={setNavPinned} />
        <TopNav pinned={navPinned} />
        <main
          className={`${navPinned ? 'ml-nav-width' : 'ml-20'} pt-16 min-h-screen bg-surface transition-[margin] duration-200 ease-out`}
        >
          <Outlet context={{ addAccountOpen, setAddAccountOpen }} />
        </main>
      </div>
    </LayoutContext.Provider>
  )
}
