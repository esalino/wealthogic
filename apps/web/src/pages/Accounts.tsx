import { useEffect, useRef, useState } from 'react'
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useOutletContext } from 'react-router-dom'
import { type Account, createAccount, getAccounts, updateAccount } from '../api/accounts'
import { getUsers } from '../api/users'

interface OutletCtx {
  addAccountOpen: boolean
  setAddAccountOpen: (v: boolean) => void
}

const ACCOUNT_TYPES = ['Brokerage', 'Savings', 'Checking', 'Credit Card', 'Crypto']
const TAX_TYPES = ['Personal', 'IRA', 'Roth']

// ── Shared form fields ────────────────────────────────────────────────────────

interface AccountFormFieldsProps {
  nickname: string
  setNickname: (v: string) => void
  accountType: string
  setAccountType: (v: string) => void
  taxType: string
  setTaxType: (v: string) => void
  balance: string
  setBalance: (v: string) => void
  ownerIds: string[]
  toggleOwner: (id: string) => void
  users: { id: string; first_name: string; last_name: string }[]
}

function AccountFormFields({
  nickname, setNickname,
  accountType, setAccountType,
  taxType, setTaxType,
  balance, setBalance,
  ownerIds, toggleOwner,
  users,
}: AccountFormFieldsProps) {
  const inputCls = 'w-full px-3 py-2.5 bg-surface-container-low border border-outline-variant rounded-lg text-body-md text-on-surface placeholder:text-on-surface-variant focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors'

  return (
    <div className="space-y-4">
      <div>
        <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Account Name</label>
        <input
          type="text"
          value={nickname}
          onChange={(e) => setNickname(e.target.value)}
          placeholder="e.g. My Fidelity IRA"
          className={inputCls}
        />
      </div>

      <div>
        <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Account Type</label>
        <select value={accountType} onChange={(e) => setAccountType(e.target.value)} className={inputCls}>
          {ACCOUNT_TYPES.map((t) => <option key={t}>{t}</option>)}
        </select>
      </div>

      <div>
        <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Tax Type</label>
        <select value={taxType} onChange={(e) => setTaxType(e.target.value)} className={inputCls}>
          {TAX_TYPES.map((t) => <option key={t}>{t}</option>)}
        </select>
      </div>

      {users.length > 0 && (
        <div>
          <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Owners</label>
          <div className="space-y-1.5">
            {users.map((user) => (
              <label
                key={user.id}
                className="flex items-center gap-2.5 px-3 py-2 rounded-lg cursor-pointer hover:bg-surface-container-low transition-colors"
              >
                <input
                  type="checkbox"
                  checked={ownerIds.includes(user.id)}
                  onChange={() => toggleOwner(user.id)}
                  className="w-4 h-4 accent-secondary"
                />
                <span className="text-body-md text-on-surface">{user.first_name} {user.last_name}</span>
              </label>
            ))}
          </div>
        </div>
      )}

      <div>
        <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Balance</label>
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
  )
}

// ── Add modal ─────────────────────────────────────────────────────────────────

interface AddAccountModalProps {
  isOpen: boolean
  onClose: () => void
}

function AddAccountModal({ isOpen, onClose }: AddAccountModalProps) {
  const queryClient = useQueryClient()
  const [nickname, setNickname] = useState('')
  const [accountType, setAccountType] = useState('Brokerage')
  const [taxType, setTaxType] = useState('Personal')
  const [balance, setBalance] = useState('')
  const [ownerIds, setOwnerIds] = useState<string[]>([])

  const { data: users = [] } = useQuery({ queryKey: ['users'], queryFn: getUsers })

  const { mutate, isPending, error, reset } = useMutation({
    mutationFn: createAccount,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['accounts'] })
      setNickname(''); setAccountType('Brokerage'); setTaxType('Personal')
      setBalance(''); setOwnerIds([])
      reset(); onClose()
    },
  })

  function toggleOwner(id: string) {
    setOwnerIds((prev) => prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id])
  }

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center p-4" onClick={onClose}>
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" />
      <div
        className="relative bg-surface-container-lowest rounded-xl shadow-card w-full max-w-md p-6"
        onClick={(e) => e.stopPropagation()}
        style={{ animation: 'slideUp 0.25s ease-out' }}
      >
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-headline-sm text-on-surface">Add New Account</h2>
          <button onClick={onClose} className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors">
            <span className="material-symbols-outlined text-xl">close</span>
          </button>
        </div>

        <AccountFormFields
          nickname={nickname} setNickname={setNickname}
          accountType={accountType} setAccountType={setAccountType}
          taxType={taxType} setTaxType={setTaxType}
          balance={balance} setBalance={setBalance}
          ownerIds={ownerIds} toggleOwner={toggleOwner}
          users={users}
        />

        {error && <p className="mt-4 text-body-sm text-error">{(error as Error).message}</p>}

        <div className="flex gap-3 mt-6">
          <button onClick={onClose} disabled={isPending} className="flex-1 px-4 py-2.5 border border-outline-variant rounded-lg text-body-md text-on-surface hover:bg-surface-container-high transition-colors disabled:opacity-50">
            Cancel
          </button>
          <button
            onClick={() => mutate({ account_name: nickname, account_type: accountType, tax_type: taxType, balance: parseFloat(balance) || 0, owner_ids: ownerIds })}
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

// ── Edit modal ────────────────────────────────────────────────────────────────

interface EditAccountModalProps {
  account: Account | null
  onClose: () => void
}

function EditAccountModal({ account, onClose }: EditAccountModalProps) {
  const queryClient = useQueryClient()
  const [nickname, setNickname] = useState('')
  const [accountType, setAccountType] = useState('Brokerage')
  const [taxType, setTaxType] = useState('Personal')
  const [balance, setBalance] = useState('')
  const [ownerIds, setOwnerIds] = useState<string[]>([])

  const { data: users = [] } = useQuery({ queryKey: ['users'], queryFn: getUsers })

  useEffect(() => {
    if (account) {
      setNickname(account.account_name)
      setAccountType(account.account_type)
      setTaxType(account.tax_type ?? 'Personal')
      setBalance(String(account.balance))
      setOwnerIds((account.owners ?? []).map((o) => o.id))
    }
  }, [account])

  const { mutate, isPending, error } = useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: Parameters<typeof updateAccount>[1] }) =>
      updateAccount(id, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['accounts'] })
      onClose()
    },
  })

  function toggleOwner(id: string) {
    setOwnerIds((prev) => prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id])
  }

  if (!account) return null

  return (
    <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center p-4" onClick={onClose}>
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" />
      <div
        className="relative bg-surface-container-lowest rounded-xl shadow-card w-full max-w-md p-6"
        onClick={(e) => e.stopPropagation()}
        style={{ animation: 'slideUp 0.25s ease-out' }}
      >
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-headline-sm text-on-surface">Edit Account</h2>
          <button onClick={onClose} className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors">
            <span className="material-symbols-outlined text-xl">close</span>
          </button>
        </div>

        <AccountFormFields
          nickname={nickname} setNickname={setNickname}
          accountType={accountType} setAccountType={setAccountType}
          taxType={taxType} setTaxType={setTaxType}
          balance={balance} setBalance={setBalance}
          ownerIds={ownerIds} toggleOwner={toggleOwner}
          users={users}
        />

        {error && <p className="mt-4 text-body-sm text-error">{(error as Error).message}</p>}

        <div className="flex gap-3 mt-6">
          <button onClick={onClose} disabled={isPending} className="flex-1 px-4 py-2.5 border border-outline-variant rounded-lg text-body-md text-on-surface hover:bg-surface-container-high transition-colors disabled:opacity-50">
            Cancel
          </button>
          <button
            onClick={() => mutate({ id: account.id, payload: { account_name: nickname, account_type: accountType, tax_type: taxType, balance: parseFloat(balance) || 0, owner_ids: ownerIds } })}
            disabled={isPending}
            className="flex-1 px-4 py-2.5 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {isPending ? 'Saving…' : 'Save Changes'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Row action menu ───────────────────────────────────────────────────────────

function RowMenu({ onEdit }: { onEdit: () => void }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    function handler(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  return (
    <div ref={ref} className="relative flex justify-end">
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors"
      >
        <span className="material-symbols-outlined text-xl">more_vert</span>
      </button>

      {open && (
        <div className="absolute right-0 top-9 z-20 w-36 bg-surface-container-lowest rounded-lg shadow-card border border-outline-variant py-1">
          <button
            onClick={() => { setOpen(false); onEdit() }}
            className="w-full flex items-center gap-2 px-3 py-2 text-body-md text-on-surface hover:bg-surface-container-low transition-colors"
          >
            <span className="material-symbols-outlined text-base">edit</span>
            Edit
          </button>
        </div>
      )}
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

const columnHelper = createColumnHelper<Account>()

export default function Accounts() {
  const { addAccountOpen, setAddAccountOpen } = useOutletContext<OutletCtx>()
  const [editingAccount, setEditingAccount] = useState<Account | null>(null)
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 })

  const { data, isLoading, isError } = useQuery({
    queryKey: ['accounts', pagination.pageIndex, pagination.pageSize],
    queryFn: () => getAccounts(pagination.pageIndex + 1, pagination.pageSize),
  })

  const accounts = data?.data ?? []
  const totalBalance = accounts.reduce((sum, acc) => sum + acc.balance, 0)
  const pageCount = data ? Math.ceil(data.total / pagination.pageSize) : 0

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
      cell: (info) => <span className="text-body-md text-on-surface-variant">{info.getValue()}</span>,
    }),
    columnHelper.accessor('owners', {
      header: 'Owners',
      cell: (info) => (
        <span className="text-body-md text-on-surface-variant">
          {(info.getValue() ?? []).map((o) => o.first_name).join(', ') || '—'}
        </span>
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
      cell: (info) => <RowMenu onEdit={() => setEditingAccount(info.row.original)} />,
    }),
  ]

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
            <h1 className="text-headline-lg text-on-surface mb-1">Accounts</h1>
            <p className="text-body-md text-on-surface-variant">Manage and monitor all your financial institutions.</p>
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
                      <th key={header.id} className="px-6 py-3 text-label-caps text-on-surface-variant uppercase text-left">
                        {flexRender(header.column.columnDef.header, header.getContext())}
                      </th>
                    ))}
                  </tr>
                ))}
              </thead>
              <tbody>
                {isLoading && (
                  <tr><td colSpan={columns.length} className="px-6 py-8 text-center text-body-md text-on-surface-variant">Loading accounts…</td></tr>
                )}
                {isError && (
                  <tr><td colSpan={columns.length} className="px-6 py-8 text-center text-body-md text-error">Failed to load accounts.</td></tr>
                )}
                {!isLoading && !isError && accounts.length === 0 && (
                  <tr><td colSpan={columns.length} className="px-6 py-8 text-center text-body-md text-on-surface-variant">No accounts yet. Add one to get started.</td></tr>
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

          {/* Pagination */}
          <div className="flex items-center justify-between px-6 py-4 border-t border-outline-variant">
            <div className="flex items-center gap-2">
              <span className="text-body-sm text-on-surface-variant">Rows per page</span>
              <select
                value={pagination.pageSize}
                onChange={(e) => table.setPageSize(Number(e.target.value))}
                className="px-2 py-1 bg-surface-container-low border border-outline-variant rounded-lg text-body-sm text-on-surface focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors"
              >
                {[10, 20, 50].map((size) => <option key={size} value={size}>{size}</option>)}
              </select>
            </div>
            <div className="flex items-center gap-3">
              <p className="text-body-sm text-on-surface-variant">
                Page {table.getState().pagination.pageIndex + 1} of {Math.max(pageCount, 1)}
              </p>
              <div className="flex items-center gap-1">
                <button onClick={() => table.previousPage()} disabled={!table.getCanPreviousPage()} className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors disabled:opacity-40">
                  <span className="material-symbols-outlined text-xl">chevron_left</span>
                </button>
                <button onClick={() => table.nextPage()} disabled={!table.getCanNextPage()} className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors disabled:opacity-40">
                  <span className="material-symbols-outlined text-xl">chevron_right</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <AddAccountModal isOpen={addAccountOpen} onClose={() => setAddAccountOpen(false)} />
      <EditAccountModal account={editingAccount} onClose={() => setEditingAccount(null)} />

      <style>{`
        @keyframes slideUp {
          from { opacity: 0; transform: translateY(24px); }
          to { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </>
  )
}
