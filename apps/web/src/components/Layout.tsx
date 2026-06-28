import { Outlet } from 'react-router-dom'
import { useState } from 'react'
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

  return (
    <LayoutContext.Provider value={{ openAddAccount: () => setAddAccountOpen(true) }}>
      <div className="min-h-screen bg-surface">
        <Sidebar onAddAccount={() => setAddAccountOpen(true)} />
        <TopNav />
        <main className="ml-nav-width pt-16 min-h-screen bg-surface">
          <Outlet context={{ addAccountOpen, setAddAccountOpen }} />
        </main>
      </div>
    </LayoutContext.Provider>
  )
}
