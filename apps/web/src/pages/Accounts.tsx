import { useState } from 'react'
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useOutletContext } from 'react-router-dom'
import { type Account, createAccount, getAccounts } from '../api/accounts'

interface OutletCtx {
  addAccountOpen: boolean
  setAddAccountOpen: (v: boolean) => void
}

const columnHelper = createColumnHelper<Account>()

const columns = [
  columnHelper.accessor('account_name', {
    header: 'Account',
    cell: (info) => (
      <div className="flex items-center gap-3">
        <div className="w-9 h-9 rounded-lg bg-primary-container flex items-center justify-center flex-shrink-0">
          <span className="material-symbols-outlined text-xl text-on-surface">account_balance</span>
        </div>
        <span className="text-body-md font-medium text-on-surface">{info.getValue()}</span>
      </div>
    ),
  }),
  columnHelper.accessor('account_type', {
    header: 'Type',
    cell: (info) => (
      <span className="text-body-md text-on-surface-variant">{info.getValue()}</span>
    ),
  }),
  columnHelper.accessor('balance', {
    header: () => <span className="block text-right">Balance</span>,
    cell: (info) => (
      <span className="block text-right text-data-tabular font-semibold text-on-surface tabular-nums">
        {info.getValue().toLocaleString('en-US', { style: 'currency', currency: 'USD' })}
      </span>
    ),
  }),
  columnHelper.display({
    id: 'actions',
    cell: () => (
      <div className="flex justify-end">
        <button className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors">
          <span className="material-symbols-outlined text-xl">more_vert</span>
        </button>
      </div>
    ),
  }),
]

interface AddAccountModalProps {
  isOpen: boolean
  onClose: () => void
}

function AddAccountModal({ isOpen, onClose }: AddAccountModalProps) {
  const queryClient = useQueryClient()
  const [nickname, setNickname] = useState('')
  const [accountType, setAccountType] = useState('Brokerage')
  const [balance, setBalance] = useState('')

  const { mutate, isPending, error, reset } = useMutation({
    mutationFn: createAccount,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['accounts'] })
      setNickname('')
      setAccountType('Brokerage')
      setBalance('')
      reset()
      onClose()
    },
  })

  function handleSubmit() {
    mutate({
      account_name: nickname,
      account_type: accountType,
      balance: parseFloat(balance) || 0,
    })
  }

  if (!isOpen) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-end sm:items-center justify-center p-4"
      onClick={onClose}
    >
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" />

      <div
        className="relative bg-surface-container-lowest rounded-xl shadow-card w-full max-w-md p-6"
        onClick={(e) => e.stopPropagation()}
        style={{ animation: 'slideUp 0.25s ease-out' }}
      >
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-headline-sm text-on-surface">Add New Account</h2>
          <button
            onClick={onClose}
            className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors"
          >
            <span className="material-symbols-outlined text-xl">close</span>
          </button>
        </div>

        <div className="space-y-4">
          <div>
            <label className="block text-label-sm font-semibold text-on-surface mb-1.5">
              Account Name
            </label>
            <input
              type="text"
              value={nickname}
              onChange={(e) => setNickname(e.target.value)}
              placeholder="e.g. My Fidelity IRA"
              className="w-full px-3 py-2.5 bg-surface-container-low border border-outline-variant rounded-lg text-body-md text-on-surface placeholder:text-on-surface-variant focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors"
            />
          </div>

          <div>
            <label className="block text-label-sm font-semibold text-on-surface mb-1.5">
              Account Type
            </label>
            <select
              value={accountType}
              onChange={(e) => setAccountType(e.target.value)}
              className="w-full px-3 py-2.5 bg-surface-container-low border border-outline-variant rounded-lg text-body-md text-on-surface focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors"
            >
              <option>Brokerage</option>
              <option>Savings</option>
              <option>Checking</option>
              <option>Credit Card</option>
              <option>Crypto</option>
            </select>
          </div>

          <div>
            <label className="block text-label-sm font-semibold text-on-surface mb-1.5">
              Initial Balance
            </label>
            <div className="relative">
              <span className="absolute left-3 top-1/2 -translate-y-1/2 text-body-md text-on-surface-variant">$</span>
              <input
                type="number"
                value={balance}
                onChange={(e) => setBalance(e.target.value)}
                placeholder="0.00"
                className="w-full pl-7 pr-3 py-2.5 bg-surface-container-low border border-outline-variant rounded-lg text-body-md text-on-surface placeholder:text-on-surface-variant focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors tabular-nums"
              />
            </div>
          </div>
        </div>

        {error && (
          <p className="mt-4 text-body-sm text-error">{(error as Error).message}</p>
        )}

        <div className="flex gap-3 mt-6">
          <button
            onClick={onClose}
            disabled={isPending}
            className="flex-1 px-4 py-2.5 border border-outline-variant rounded-lg text-body-md text-on-surface hover:bg-surface-container-high transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={isPending}
            className="flex-1 px-4 py-2.5 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {isPending ? 'Saving…' : 'Add Account'}
          </button>
        </div>
      </div>
    </div>
  )
}

export default function Accounts() {
  const { addAccountOpen, setAddAccountOpen } = useOutletContext<OutletCtx>()

  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 })

  const { data, isLoading, isError } = useQuery({
    queryKey: ['accounts', pagination.pageIndex, pagination.pageSize],
    queryFn: () => getAccounts(pagination.pageIndex + 1, pagination.pageSize),
  })

  const accounts = data?.data ?? []
  const totalBalance = accounts.reduce((sum, acc) => sum + acc.balance, 0)
  const pageCount = data ? Math.ceil(data.total / pagination.pageSize) : 0

  const table = useReactTable({
    data: accounts,
    columns,
    pageCount,
    state: { pagination },
    onPaginationChange: setPagination,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
  })

  return (
    <>
      <div className="p-8">
        {/* Page header */}
        <div className="flex items-start justify-between mb-8">
          <div>
            <h1 className="text-headline-lg text-on-surface mb-1">Linked Accounts</h1>
            <p className="text-body-md text-on-surface-variant">Manage and monitor all your connected financial institutions.</p>
          </div>
          <div className="flex gap-3">
            <button className="flex items-center gap-2 px-4 py-2 border border-outline-variant rounded-lg text-body-md text-on-surface hover:bg-surface-container-high transition-colors">
              <span className="material-symbols-outlined text-xl">download</span>
              Export CSV
            </button>
            <button
              onClick={() => setAddAccountOpen(true)}
              className="flex items-center gap-2 px-4 py-2 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity"
            >
              <span className="material-symbols-outlined text-xl">add</span>
              Add New Account
            </button>
          </div>
        </div>

        {/* Stats bento */}
        <div className="grid grid-cols-12 gap-6 mb-6">
          <div className="col-span-12 lg:col-span-8 bg-surface-container-lowest rounded-xl shadow-card p-6">
            <p className="text-label-caps text-on-surface-variant uppercase mb-1">Total Managed Balance</p>
            <div className="flex items-baseline gap-3 mb-1">
              <span className="text-headline-lg text-on-surface tabular-nums">
                {totalBalance.toLocaleString('en-US', { style: 'currency', currency: 'USD' })}
              </span>
            </div>
            <p className="text-body-md text-on-surface-variant">
              Across {data?.total ?? 0} account{data?.total !== 1 ? 's' : ''}
            </p>
          </div>

          <div className="col-span-12 lg:col-span-4 bg-primary-container rounded-xl shadow-card p-6">
            <p className="text-label-caps text-on-primary-container uppercase mb-3">Active Connections</p>
            <p className="text-headline-md text-on-primary font-bold mb-1">
              {data?.total ?? 0} Account{data?.total !== 1 ? 's' : ''}
            </p>
          </div>
        </div>

        {/* Accounts table */}
        <div className="bg-surface-container-lowest rounded-xl shadow-card mb-6">
          <div className="flex items-center justify-between px-6 py-4 border-b border-outline-variant">
            <h2 className="text-headline-sm text-on-surface">Account List</h2>
            <button className="text-body-md text-secondary font-semibold hover:opacity-80 transition-opacity">Manage All</button>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                {table.getHeaderGroups().map((headerGroup) => (
                  <tr key={headerGroup.id} className="border-b border-outline-variant">
                    {headerGroup.headers.map((header) => (
                      <th
                        key={header.id}
                        className="px-6 py-3 text-label-caps text-on-surface-variant uppercase text-left"
                      >
                        {flexRender(header.column.columnDef.header, header.getContext())}
                      </th>
                    ))}
                  </tr>
                ))}
              </thead>
              <tbody>
                {isLoading && (
                  <tr>
                    <td colSpan={columns.length} className="px-6 py-8 text-center text-body-md text-on-surface-variant">
                      Loading accounts…
                    </td>
                  </tr>
                )}
                {isError && (
                  <tr>
                    <td colSpan={columns.length} className="px-6 py-8 text-center text-body-md text-error">
                      Failed to load accounts.
                    </td>
                  </tr>
                )}
                {!isLoading && !isError && accounts.length === 0 && (
                  <tr>
                    <td colSpan={columns.length} className="px-6 py-8 text-center text-body-md text-on-surface-variant">
                      No accounts yet. Add one to get started.
                    </td>
                  </tr>
                )}
                {table.getRowModel().rows.map((row) => (
                  <tr key={row.id} className="border-b border-outline-variant last:border-0 hover:bg-surface-container-low transition-colors">
                    {row.getVisibleCells().map((cell) => (
                      <td key={cell.id} className="px-6 py-4">
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Pagination controls */}
          <div className="flex items-center justify-between px-6 py-4 border-t border-outline-variant">
            <div className="flex items-center gap-2">
              <span className="text-body-sm text-on-surface-variant">Rows per page</span>
              <select
                value={pagination.pageSize}
                onChange={(e) => table.setPageSize(Number(e.target.value))}
                className="px-2 py-1 bg-surface-container-low border border-outline-variant rounded-lg text-body-sm text-on-surface focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors"
              >
                {[10, 20, 50].map((size) => (
                  <option key={size} value={size}>{size}</option>
                ))}
              </select>
            </div>
            <div className="flex items-center gap-3">
              <p className="text-body-sm text-on-surface-variant">
                Page {table.getState().pagination.pageIndex + 1} of {Math.max(pageCount, 1)}
              </p>
              <div className="flex items-center gap-1">
                <button
                  onClick={() => table.previousPage()}
                  disabled={!table.getCanPreviousPage()}
                  className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors disabled:opacity-40"
                >
                  <span className="material-symbols-outlined text-xl">chevron_left</span>
                </button>
                <button
                  onClick={() => table.nextPage()}
                  disabled={!table.getCanNextPage()}
                  className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors disabled:opacity-40"
                >
                  <span className="material-symbols-outlined text-xl">chevron_right</span>
                </button>
              </div>
            </div>
          </div>
        </div>

        {/* Footer CTAs */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div className="bg-surface-container-lowest rounded-xl shadow-card p-6">
            <div className="w-10 h-10 rounded-lg bg-secondary-container flex items-center justify-center mb-4">
              <span className="material-symbols-outlined text-secondary text-xl">verified_user</span>
            </div>
            <h3 className="text-headline-sm text-on-surface mb-2">Security First</h3>
            <p className="text-body-md text-on-surface-variant">
              Your data is protected with bank-level 256-bit encryption. We never store your banking credentials — only read-only access tokens.
            </p>
          </div>

          <div className="bg-surface-container-lowest rounded-xl shadow-card p-6">
            <div className="w-10 h-10 rounded-lg bg-primary-fixed flex items-center justify-center mb-4">
              <span className="material-symbols-outlined text-primary text-xl">auto_awesome</span>
            </div>
            <h3 className="text-headline-sm text-on-surface mb-2">Smart Categorization</h3>
            <p className="text-body-md text-on-surface-variant">
              Our AI automatically categorizes your transactions and tracks portfolio performance so you always have an accurate financial picture.
            </p>
          </div>
        </div>
      </div>

      <AddAccountModal isOpen={addAccountOpen} onClose={() => setAddAccountOpen(false)} />

      <style>{`
        @keyframes slideUp {
          from { opacity: 0; transform: translateY(24px); }
          to { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </>
  )
}
