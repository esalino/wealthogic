import { Fragment, useRef, useState, useEffect } from 'react'
import { createPortal } from 'react-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createHolding,
  getHoldings,
  updateHolding,
  type CreateHoldingPayload,
  type Holding as ApiHolding,
} from '../api/holdings'

type ChangeDirection = 'positive' | 'negative' | 'neutral'

interface TaxLot {
  dateAcquired: string
  quantity: string
  costBasis: string
  currentValue: string
}

interface Transaction {
  date: string
  action: string
  quantity: string
  price: string
  amount: string
  amountDir: ChangeDirection
}

interface Dividend {
  date: string
  perShare: string
  shares: string
  total: string
}

interface Holding {
  id: string
  symbol: string
  assetClass: string
  price: string
  quantity: string
  marketValue: string
  allocation: string
  averageCostBasis: string
  costBasisTotal: string
  gainUnrealizedPercent: string
  gainUnrealizedAmount: string
  gainRealizedPercent: string
  gainRealizedAmount: string
  dividendIncome: string
  taxLots: TaxLot[]
  transactions: Transaction[]
  dividends: Dividend[]
}

const fmtCurrency = (n: number) =>
  (n ?? 0).toLocaleString('en-US', { style: 'currency', currency: 'USD' })

const fmtNumber = (n: number) =>
  (n ?? 0).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })

function fmtSignedCurrency(n: number) {
  const v = n ?? 0
  return `${v < 0 ? '-' : '+'}${fmtCurrency(Math.abs(v))}`
}

function fmtPercent(n: number) {
  const v = n ?? 0
  return `${v >= 0 ? '+' : ''}${v.toFixed(2)}%`
}

// toViewHolding maps an API holding into the display shape used by the table.
// Sub-records (tax lots, transactions, dividends) are intentionally empty for
// now — they'll be lazy-loaded when a row is expanded.
function toViewHolding(h: ApiHolding, totalMarketValue: number): Holding {
  const allocation = totalMarketValue > 0 ? (h.current_value / totalMarketValue) * 100 : 0
  return {
    id: h.id,
    symbol: h.symbol || '—',
    assetClass: h.asset_type || h.description || '',
    price: fmtCurrency(h.last_price),
    quantity: fmtNumber(h.purchase_quantity),
    marketValue: fmtCurrency(h.current_value),
    allocation: `${allocation.toFixed(2)}%`,
    averageCostBasis: fmtCurrency(h.average_cost_basis),
    costBasisTotal: fmtCurrency(h.cost_basis_total),
    gainUnrealizedPercent: fmtPercent(h.gain_unrealized_percent),
    gainUnrealizedAmount: fmtSignedCurrency(h.gain_unrealized_amount),
    gainRealizedPercent: fmtPercent(h.gain_realized_percent),
    gainRealizedAmount: fmtSignedCurrency(h.gain_realized_amount),
    dividendIncome: fmtCurrency(h.dividend_income),
    taxLots: [],
    transactions: [],
    dividends: [],
  }
}

const subTabs = ['Tax Lots', 'Transactions', 'Dividends'] as const
type SubTab = (typeof subTabs)[number]

const addLabels: Record<SubTab, string> = {
  'Tax Lots': 'Add Lot',
  Transactions: 'Add Transaction',
  Dividends: 'Add Dividend',
}

function gainDir(amount: string): ChangeDirection {
  const n = parseFloat(amount.replace(/[^0-9.-]/g, ''))
  if (n > 0) return 'positive'
  if (n < 0) return 'negative'
  return 'neutral'
}

function gainColor(dir: ChangeDirection) {
  if (dir === 'positive') return 'text-secondary'
  if (dir === 'negative') return 'text-error'
  return 'text-on-surface'
}

function gainCell(amount: string, percent: string) {
  const color = gainColor(gainDir(amount))
  return (
    <td className="px-4 py-4 text-right tabular-nums">
      <div className={`text-data-tabular font-semibold ${color}`}>{amount}</div>
      <div className={`text-label-sm ${color}`}>{percent}</div>
    </td>
  )
}

function rowActions() {
  return (
    <td className="px-4 py-3 text-right w-10">
      <button className="text-on-surface-variant hover:text-primary transition-colors" aria-label="Edit record">
        <span className="material-symbols-outlined text-lg align-middle">more_vert</span>
      </button>
    </td>
  )
}

function SubPanel({ holding, activeTab, onTabChange }: { holding: Holding; activeTab: SubTab; onTabChange: (t: SubTab) => void }) {
  return (
    <div className="p-6 space-y-4">
      {/* Tab bar */}
      <div className="flex items-center justify-between border-b border-outline-variant">
        <div className="flex gap-6">
          {subTabs.map((tab) => {
            const active = tab === activeTab
            return (
              <button
                key={tab}
                onClick={() => onTabChange(tab)}
                className={`pb-2 text-label-sm transition-colors ${
                  active
                    ? 'border-b-2 border-primary font-bold text-primary'
                    : 'text-on-surface-variant hover:text-primary'
                }`}
              >
                {tab}
              </button>
            )
          })}
        </div>
        <button className="flex items-center gap-1 pb-2 text-label-sm font-semibold text-secondary hover:opacity-80 transition-opacity">
          <span className="material-symbols-outlined text-base">add</span>
          {addLabels[activeTab]}
        </button>
      </div>

      {/* Tab content */}
      <div className="overflow-hidden rounded-lg border border-outline-variant">
        <table className="w-full text-left border-collapse">
          {activeTab === 'Tax Lots' && (
            <>
              <thead className="bg-surface-container-low">
                <tr className="text-[10px] text-label-caps text-on-surface-variant uppercase">
                  <th className="px-4 py-2">Date Acquired</th>
                  <th className="px-4 py-2 text-right">Quantity</th>
                  <th className="px-4 py-2 text-right">Cost Basis</th>
                  <th className="px-4 py-2 text-right">Current Value</th>
                  <th className="px-4 py-2 w-10" />
                </tr>
              </thead>
              <tbody className="divide-y divide-outline-variant">
                {holding.taxLots.map((lot, i) => (
                  <tr key={i} className="text-data-tabular text-on-surface tabular-nums">
                    <td className="px-4 py-3">{lot.dateAcquired}</td>
                    <td className="px-4 py-3 text-right">{lot.quantity}</td>
                    <td className="px-4 py-3 text-right">{lot.costBasis}</td>
                    <td className="px-4 py-3 text-right">{lot.currentValue}</td>
                    {rowActions()}
                  </tr>
                ))}
              </tbody>
            </>
          )}

          {activeTab === 'Transactions' && (
            <>
              <thead className="bg-surface-container-low">
                <tr className="text-[10px] text-label-caps text-on-surface-variant uppercase">
                  <th className="px-4 py-2">Date</th>
                  <th className="px-4 py-2">Action</th>
                  <th className="px-4 py-2 text-right">Quantity</th>
                  <th className="px-4 py-2 text-right">Price</th>
                  <th className="px-4 py-2 text-right">Amount</th>
                  <th className="px-4 py-2 w-10" />
                </tr>
              </thead>
              <tbody className="divide-y divide-outline-variant">
                {holding.transactions.map((txn, i) => (
                  <tr key={i} className="text-data-tabular text-on-surface tabular-nums">
                    <td className="px-4 py-3">{txn.date}</td>
                    <td className="px-4 py-3">
                      <span className="inline-flex items-center px-2 py-0.5 rounded-full bg-surface-container-high text-on-surface-variant text-[10px] font-bold">
                        {txn.action}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right">{txn.quantity}</td>
                    <td className="px-4 py-3 text-right">{txn.price}</td>
                    <td
                      className={`px-4 py-3 text-right ${
                        txn.amountDir === 'positive'
                          ? 'text-secondary'
                          : txn.amountDir === 'negative'
                          ? 'text-error'
                          : 'text-on-surface'
                      }`}
                    >
                      {txn.amount}
                    </td>
                    {rowActions()}
                  </tr>
                ))}
              </tbody>
            </>
          )}

          {activeTab === 'Dividends' && (
            <>
              <thead className="bg-surface-container-low">
                <tr className="text-[10px] text-label-caps text-on-surface-variant uppercase">
                  <th className="px-4 py-2">Date</th>
                  <th className="px-4 py-2 text-right">Per Share</th>
                  <th className="px-4 py-2 text-right">Shares</th>
                  <th className="px-4 py-2 text-right">Total</th>
                  <th className="px-4 py-2 w-10" />
                </tr>
              </thead>
              <tbody className="divide-y divide-outline-variant">
                {holding.dividends.map((div, i) => (
                  <tr key={i} className="text-data-tabular text-on-surface tabular-nums">
                    <td className="px-4 py-3">{div.date}</td>
                    <td className="px-4 py-3 text-right">{div.perShare}</td>
                    <td className="px-4 py-3 text-right">{div.shares}</td>
                    <td className="px-4 py-3 text-right text-secondary">{div.total}</td>
                    {rowActions()}
                  </tr>
                ))}
              </tbody>
            </>
          )}
        </table>
      </div>
    </div>
  )
}

interface Sector {
  label: string
  pct: number
}

const sectors: Sector[] = [
  { label: 'Technology', pct: 42.5 },
  { label: 'Financial Services', pct: 18.2 },
  { label: 'Healthcare', pct: 12.8 },
  { label: 'Consumer Discretionary', pct: 10.5 },
  { label: 'Others', pct: 16.0 },
]

const ASSET_TYPES = ['Stock', 'ETF', 'Mutual Fund', 'Bond', 'Money Market', 'Crypto', 'Other']

// ── Shared holding form ───────────────────────────────────────────────────────

interface HoldingFields {
  assetType: string
  symbol: string
  description: string
  quantity: string
  lastPrice: string
  avgCostBasis: string
  dividendIncome: string
}

const emptyHoldingFields: HoldingFields = {
  assetType: 'Stock',
  symbol: '',
  description: '',
  quantity: '',
  lastPrice: '',
  avgCostBasis: '',
  dividendIncome: '',
}

function fieldsFromHolding(h: ApiHolding): HoldingFields {
  return {
    assetType: h.asset_type || 'Stock',
    symbol: h.symbol ?? '',
    description: h.description ?? '',
    quantity: String(h.purchase_quantity ?? ''),
    lastPrice: String(h.last_price ?? ''),
    avgCostBasis: String(h.average_cost_basis ?? ''),
    dividendIncome: String(h.dividend_income ?? ''),
  }
}

function fieldsToPayload(f: HoldingFields): CreateHoldingPayload {
  const qty = parseFloat(f.quantity) || 0
  const price = parseFloat(f.lastPrice) || 0
  const avgCost = parseFloat(f.avgCostBasis) || 0
  return {
    asset_type: f.assetType,
    symbol: f.symbol.trim(),
    description: f.description.trim(),
    last_price: price,
    purchase_quantity: qty,
    // current_value and cost_basis_total are derived from the entered
    // quantity, price, and average cost rather than asked for directly.
    current_value: price * qty,
    average_cost_basis: avgCost,
    cost_basis_total: avgCost * qty,
    dividend_income: parseFloat(f.dividendIncome) || 0,
  }
}

const modalInputCls = 'w-full px-3 py-2.5 bg-surface-container-low border border-outline-variant rounded-lg text-body-md text-on-surface placeholder:text-on-surface-variant focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors'
const modalNumInputCls = `${modalInputCls} tabular-nums`

function HoldingFormFields({
  fields,
  set,
}: {
  fields: HoldingFields
  set: <K extends keyof HoldingFields>(key: K, value: HoldingFields[K]) => void
}) {
  return (
    <div className="space-y-4">
      <div>
        <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Symbol</label>
        <input
          type="text"
          value={fields.symbol}
          onChange={(e) => set('symbol', e.target.value)}
          placeholder="e.g. AAPL"
          className={modalInputCls}
        />
      </div>

      <div>
        <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Description</label>
        <input
          type="text"
          value={fields.description}
          onChange={(e) => set('description', e.target.value)}
          placeholder="e.g. Apple Inc."
          className={modalInputCls}
        />
      </div>

      <div>
        <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Asset Type</label>
        <select value={fields.assetType} onChange={(e) => set('assetType', e.target.value)} className={modalInputCls}>
          {ASSET_TYPES.map((t) => <option key={t}>{t}</option>)}
        </select>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Quantity</label>
          <input
            type="number"
            value={fields.quantity}
            onChange={(e) => set('quantity', e.target.value)}
            placeholder="0.00"
            className={modalNumInputCls}
          />
        </div>
        <div>
          <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Last Price</label>
          <div className="relative">
            <span className="absolute left-3 top-1/2 -translate-y-1/2 text-body-md text-on-surface-variant">$</span>
            <input
              type="number"
              value={fields.lastPrice}
              onChange={(e) => set('lastPrice', e.target.value)}
              placeholder="0.00"
              className={`${modalNumInputCls} pl-7`}
            />
          </div>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Avg Cost Basis</label>
          <div className="relative">
            <span className="absolute left-3 top-1/2 -translate-y-1/2 text-body-md text-on-surface-variant">$</span>
            <input
              type="number"
              value={fields.avgCostBasis}
              onChange={(e) => set('avgCostBasis', e.target.value)}
              placeholder="0.00"
              className={`${modalNumInputCls} pl-7`}
            />
          </div>
        </div>
        <div>
          <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Dividend Income</label>
          <div className="relative">
            <span className="absolute left-3 top-1/2 -translate-y-1/2 text-body-md text-on-surface-variant">$</span>
            <input
              type="number"
              value={fields.dividendIncome}
              onChange={(e) => set('dividendIncome', e.target.value)}
              placeholder="0.00"
              className={`${modalNumInputCls} pl-7`}
            />
          </div>
        </div>
      </div>
    </div>
  )
}

// ── Add holding modal ─────────────────────────────────────────────────────────

function AddHoldingModal({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [fields, setFields] = useState<HoldingFields>(emptyHoldingFields)
  const set = <K extends keyof HoldingFields>(key: K, value: HoldingFields[K]) =>
    setFields((f) => ({ ...f, [key]: value }))

  const { mutate, isPending, error, reset } = useMutation({
    mutationFn: createHolding,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['holdings'] })
      setFields(emptyHoldingFields)
      reset()
      onClose()
    },
  })

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
          <h2 className="text-headline-sm text-on-surface">Add Holding</h2>
          <button onClick={onClose} className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors">
            <span className="material-symbols-outlined text-xl">close</span>
          </button>
        </div>

        <HoldingFormFields fields={fields} set={set} />

        {error && <p className="mt-4 text-body-sm text-error">{(error as Error).message}</p>}

        <div className="flex gap-3 mt-6">
          <button onClick={onClose} disabled={isPending} className="flex-1 px-4 py-2.5 border border-outline-variant rounded-lg text-body-md text-on-surface hover:bg-surface-container-high transition-colors disabled:opacity-50">
            Cancel
          </button>
          <button
            onClick={() => mutate(fieldsToPayload(fields))}
            disabled={isPending || !fields.description.trim()}
            className="flex-1 px-4 py-2.5 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {isPending ? 'Saving…' : 'Add Holding'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Edit holding modal ────────────────────────────────────────────────────────

function EditHoldingModal({ holding, onClose }: { holding: ApiHolding | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  // Initialized once from the holding; the parent remounts this modal per
  // holding via a `key`, so no effect is needed to sync when it changes.
  const [fields, setFields] = useState<HoldingFields>(() =>
    holding ? fieldsFromHolding(holding) : emptyHoldingFields,
  )
  const set = <K extends keyof HoldingFields>(key: K, value: HoldingFields[K]) =>
    setFields((f) => ({ ...f, [key]: value }))

  const { mutate, isPending, error } = useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: Parameters<typeof updateHolding>[1] }) =>
      updateHolding(id, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['holdings'] })
      onClose()
    },
  })

  if (!holding) return null

  return (
    <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center p-4" onClick={onClose}>
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" />
      <div
        className="relative bg-surface-container-lowest rounded-xl shadow-card w-full max-w-md p-6"
        onClick={(e) => e.stopPropagation()}
        style={{ animation: 'slideUp 0.25s ease-out' }}
      >
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-headline-sm text-on-surface">Edit Holding</h2>
          <button onClick={onClose} className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors">
            <span className="material-symbols-outlined text-xl">close</span>
          </button>
        </div>

        <HoldingFormFields fields={fields} set={set} />

        {error && <p className="mt-4 text-body-sm text-error">{(error as Error).message}</p>}

        <div className="flex gap-3 mt-6">
          <button onClick={onClose} disabled={isPending} className="flex-1 px-4 py-2.5 border border-outline-variant rounded-lg text-body-md text-on-surface hover:bg-surface-container-high transition-colors disabled:opacity-50">
            Cancel
          </button>
          <button
            onClick={() => mutate({ id: holding.id, payload: fieldsToPayload(fields) })}
            disabled={isPending || !fields.description.trim()}
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
  const [pos, setPos] = useState({ top: 0, right: 0 })
  const btnRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    function handler(e: MouseEvent) {
      if (btnRef.current?.contains(e.target as Node)) return
      if (menuRef.current?.contains(e.target as Node)) return
      setOpen(false)
    }
    // The menu is fixed-positioned, so close it if the page scrolls or resizes
    function close() { setOpen(false) }
    document.addEventListener('mousedown', handler)
    window.addEventListener('scroll', close, true)
    window.addEventListener('resize', close)
    return () => {
      document.removeEventListener('mousedown', handler)
      window.removeEventListener('scroll', close, true)
      window.removeEventListener('resize', close)
    }
  }, [open])

  function toggle() {
    if (!open && btnRef.current) {
      const rect = btnRef.current.getBoundingClientRect()
      setPos({ top: rect.bottom + 4, right: window.innerWidth - rect.right })
    }
    setOpen((v) => !v)
  }

  return (
    <div className="relative inline-flex">
      <button
        ref={btnRef}
        onClick={toggle}
        className="text-on-surface-variant hover:text-primary transition-colors"
        aria-label="More actions"
      >
        <span className="material-symbols-outlined text-lg align-middle">more_vert</span>
      </button>

      {open && createPortal(
        <div
          ref={menuRef}
          style={{ top: pos.top, right: pos.right }}
          className="fixed z-40 w-36 bg-surface-container-lowest rounded-lg shadow-card border border-outline-variant py-1"
        >
          <button
            onClick={() => { setOpen(false); onEdit() }}
            className="w-full flex items-center gap-2 px-3 py-2 text-body-md text-on-surface hover:bg-surface-container-low transition-colors"
          >
            <span className="material-symbols-outlined text-base">edit</span>
            Edit
          </button>
        </div>,
        document.body,
      )}
    </div>
  )
}

export default function Portfolio() {
  const [mounted, setMounted] = useState(false)
  const [sortBy, setSortBy] = useState('Market Value High to Low')
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<SubTab>('Tax Lots')
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 10 })
  const [addOpen, setAddOpen] = useState(false)
  const [editingHolding, setEditingHolding] = useState<ApiHolding | null>(null)

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['holdings', pagination.pageIndex, pagination.pageSize],
    queryFn: () => getHoldings(pagination.pageIndex + 1, pagination.pageSize),
  })

  useEffect(() => {
    const t1 = setTimeout(() => setMounted(true), 100)
    return () => clearTimeout(t1)
  }, [])

  function toggleRow(id: string) {
    if (expandedId === id) {
      setExpandedId(null)
    } else {
      setExpandedId(id)
      setActiveTab('Tax Lots')
    }
  }

  const holdings = data?.data ?? []
  // Allocation is computed relative to the market value of the holdings on the
  // current page — a stand-in until a portfolio-total endpoint exists.
  const totalMarketValue = holdings.reduce((sum, h) => sum + (h.current_value ?? 0), 0)
  const pageHoldings = holdings.map((h) => toViewHolding(h, totalMarketValue))
  const pageCount = Math.max(1, Math.ceil((data?.total ?? 0) / pagination.pageSize))
  const canPrev = pagination.pageIndex > 0
  const canNext = pagination.pageIndex < pageCount - 1

  return (
    <>
      <div className="p-8">
        {/* Page header */}
        <div className="flex items-start justify-between mb-8">
          <div>
            <h1 className="text-headline-lg text-on-surface mb-1">Portfolio Breakdown</h1>
            <p className="text-body-lg text-on-surface-variant">Detailed analysis of your $2,482,190.00 net worth.</p>
          </div>
          <div className="flex gap-3">
            <button className="flex items-center gap-2 px-4 py-2 border border-outline-variant rounded-lg text-body-md text-on-surface hover:bg-surface-container-high transition-colors">
              <span className="material-symbols-outlined text-xl">picture_as_pdf</span>
              Export PDF
            </button>
            <button className="flex items-center gap-2 px-4 py-2 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity">
              <span className="material-symbols-outlined text-xl">tune</span>
              Adjust Weights
            </button>
          </div>
        </div>

        {/* Bento grid */}
        <div className="grid grid-cols-12 gap-6 mb-6">
          {/* Asset Allocation */}
          <div className="col-span-12 lg:col-span-5 bg-surface-container-lowest rounded-xl shadow-card p-6">
            <h2 className="text-headline-sm text-on-surface mb-1">Asset Allocation</h2>
            <p className="text-body-md text-on-surface-variant mb-6">Portfolio composition by asset class</p>

            {/* Doughnut chart */}
            <div className="flex justify-center mb-6">
              <div className="relative w-48 h-48">
                <div
                  className="w-48 h-48 rounded-full"
                  style={{
                    background: 'conic-gradient(#091426 0% 65%, #006c49 65% 85%, #bcc7de 85% 100%)',
                  }}
                />
                {/* White inner circle */}
                <div className="absolute inset-0 flex items-center justify-center">
                  <div className="w-28 h-28 rounded-full bg-surface-container-lowest flex flex-col items-center justify-center">
                    <span className="text-headline-sm text-on-surface font-bold">65%</span>
                    <span className="text-label-caps text-on-surface-variant uppercase">STOCKS</span>
                  </div>
                </div>
              </div>
            </div>

            {/* Legend */}
            <div className="space-y-3">
              {[
                { label: 'Stocks', value: '$1.61M', color: 'bg-primary', pct: '65%' },
                { label: 'Bonds', value: '$496k', color: 'bg-secondary', pct: '20%' },
                { label: 'Cash', value: '$372k', color: 'bg-primary-fixed-dim', pct: '15%' },
              ].map((item) => (
                <div key={item.label} className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <div className={`w-3 h-3 rounded-full ${item.color}`} />
                    <span className="text-body-md text-on-surface">{item.label}</span>
                  </div>
                  <div className="flex items-center gap-4">
                    <span className="text-body-md font-semibold text-on-surface tabular-nums">{item.value}</span>
                    <span className="text-label-sm text-on-surface-variant w-8 text-right">{item.pct}</span>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Sector Exposure */}
          <div className="col-span-12 lg:col-span-7 bg-surface-container-lowest rounded-xl shadow-card p-6">
            <h2 className="text-headline-sm text-on-surface mb-1">Sector Exposure</h2>
            <p className="text-body-md text-on-surface-variant mb-6">Allocation by market sector</p>

            <div className="space-y-5">
              {sectors.map((sector, i) => (
                <div key={sector.label}>
                  <div className="flex justify-between mb-1.5">
                    <span className="text-body-md text-on-surface">{sector.label}</span>
                    <span className="text-body-md font-semibold text-on-surface tabular-nums">{sector.pct}%</span>
                  </div>
                  <div className="w-full bg-surface-container-high rounded-full h-2">
                    <div
                      className="h-2 rounded-full bg-primary transition-all duration-700 ease-out"
                      style={{
                        width: mounted ? `${sector.pct}%` : '0%',
                        transitionDelay: `${i * 80}ms`,
                      }}
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Holdings table */}
        <div className="bg-surface-container-lowest rounded-xl shadow-card">
          <div className="flex items-center justify-between px-6 py-4 border-b border-outline-variant">
            <h2 className="text-headline-sm text-on-surface">Current Holdings</h2>
            <div className="flex items-center gap-3">
              <label className="text-label-sm text-on-surface-variant">Sort by</label>
              <select
                value={sortBy}
                onChange={(e) => setSortBy(e.target.value)}
                className="px-3 py-1.5 bg-surface-container-low border border-outline-variant rounded-lg text-body-md text-on-surface focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors"
              >
                <option>Market Value High to Low</option>
                <option>Performance 24h</option>
                <option>Asset Class</option>
              </select>
              <button
                onClick={() => setAddOpen(true)}
                className="flex items-center gap-1.5 px-4 py-1.5 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity"
              >
                <span className="material-symbols-outlined text-lg">add</span>
                Add Holding
              </button>
            </div>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-outline-variant">
                  <th className="text-left px-4 py-3 text-label-caps text-on-surface-variant uppercase">Symbol</th>
                  <th className="text-right px-4 py-3 text-label-caps text-on-surface-variant uppercase">Price</th>
                  <th className="text-right px-4 py-3 text-label-caps text-on-surface-variant uppercase">Quantity</th>
                  <th className="text-right px-4 py-3 text-label-caps text-on-surface-variant uppercase">Market Value</th>
                  <th className="text-right px-4 py-3 text-label-caps text-on-surface-variant uppercase">Avg Cost</th>
                  <th className="text-right px-4 py-3 text-label-caps text-on-surface-variant uppercase">Cost Basis</th>
                  <th className="text-right px-4 py-3 text-label-caps text-on-surface-variant uppercase">Unrealized Gain</th>
                  <th className="text-right px-4 py-3 text-label-caps text-on-surface-variant uppercase">Realized Gain</th>
                  <th className="text-right px-4 py-3 text-label-caps text-on-surface-variant uppercase">Dividend Income</th>
                  <th className="text-right px-4 py-3 text-label-caps text-on-surface-variant uppercase">Allocation</th>
                </tr>
              </thead>
              <tbody>
                {isLoading && (
                  <tr>
                    <td colSpan={10} className="px-4 py-10 text-center text-body-md text-on-surface-variant">
                      Loading holdings…
                    </td>
                  </tr>
                )}
                {!isLoading && isError && (
                  <tr>
                    <td colSpan={10} className="px-4 py-10 text-center text-body-md text-error">
                      {error instanceof Error ? error.message : 'Failed to load holdings'}
                    </td>
                  </tr>
                )}
                {!isLoading && !isError && pageHoldings.length === 0 && (
                  <tr>
                    <td colSpan={10} className="px-4 py-10 text-center text-body-md text-on-surface-variant">
                      No holdings yet.
                    </td>
                  </tr>
                )}
                {!isLoading && !isError && pageHoldings.map((h) => {
                  const expanded = expandedId === h.id
                  return (
                    <Fragment key={h.id}>
                      <tr
                        onClick={() => toggleRow(h.id)}
                        className="group border-b border-outline-variant hover:bg-surface-container-low transition-colors cursor-pointer"
                      >
                        <td className="px-4 py-4">
                          <div className="flex items-center gap-3">
                            <span
                              className={`material-symbols-outlined text-lg text-on-surface-variant transition-transform group-hover:text-primary ${
                                expanded ? 'rotate-90' : ''
                              }`}
                            >
                              chevron_right
                            </span>
                            <div>
                              <p className="text-body-md font-semibold text-on-surface">{h.symbol}</p>
                              <p className="text-label-sm text-on-surface-variant">{h.assetClass}</p>
                            </div>
                          </div>
                        </td>
                        <td className="px-4 py-4 text-right text-data-tabular text-on-surface tabular-nums">{h.price}</td>
                        <td className="px-4 py-4 text-right text-data-tabular text-on-surface tabular-nums">{h.quantity}</td>
                        <td className="px-4 py-4 text-right text-data-tabular font-semibold text-on-surface tabular-nums">{h.marketValue}</td>
                        <td className="px-4 py-4 text-right text-data-tabular text-on-surface tabular-nums">{h.averageCostBasis}</td>
                        <td className="px-4 py-4 text-right text-data-tabular text-on-surface tabular-nums">{h.costBasisTotal}</td>
                        {gainCell(h.gainUnrealizedAmount, h.gainUnrealizedPercent)}
                        {gainCell(h.gainRealizedAmount, h.gainRealizedPercent)}
                        <td className="px-4 py-4 text-right text-data-tabular text-on-surface tabular-nums">{h.dividendIncome}</td>
                        <td className="px-4 py-4 text-right text-data-tabular text-on-surface-variant tabular-nums">
                          <span className="inline-flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
                            {h.allocation}
                            <RowMenu
                              onEdit={() => {
                                const raw = holdings.find((x) => x.id === h.id)
                                if (raw) setEditingHolding(raw)
                              }}
                            />
                          </span>
                        </td>
                      </tr>
                      {expanded && (
                        <tr className="bg-surface-container-lowest border-b border-outline-variant">
                          <td className="p-0" colSpan={10}>
                            <SubPanel holding={h} activeTab={activeTab} onTabChange={setActiveTab} />
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  )
                })}
              </tbody>
            </table>
          </div>
          {/* Pagination */}
          <div className="flex items-center justify-between px-6 py-4 border-t border-outline-variant">
            <div className="flex items-center gap-2">
              <span className="text-label-sm text-on-surface-variant">Rows per page</span>
              <select
                value={pagination.pageSize}
                onChange={(e) => setPagination({ pageIndex: 0, pageSize: Number(e.target.value) })}
                className="px-2 py-1 bg-surface-container-low border border-outline-variant rounded-lg text-label-sm text-on-surface focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors"
              >
                {[10, 20, 50].map((size) => <option key={size} value={size}>{size}</option>)}
              </select>
            </div>
            <div className="flex items-center gap-3">
              <p className="text-label-sm text-on-surface-variant">
                Page {pagination.pageIndex + 1} of {pageCount}
              </p>
              <div className="flex items-center gap-1">
                <button
                  onClick={() => setPagination((p) => ({ ...p, pageIndex: p.pageIndex - 1 }))}
                  disabled={!canPrev}
                  className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors disabled:opacity-40"
                >
                  <span className="material-symbols-outlined text-xl">chevron_left</span>
                </button>
                <button
                  onClick={() => setPagination((p) => ({ ...p, pageIndex: p.pageIndex + 1 }))}
                  disabled={!canNext}
                  className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors disabled:opacity-40"
                >
                  <span className="material-symbols-outlined text-xl">chevron_right</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <AddHoldingModal isOpen={addOpen} onClose={() => setAddOpen(false)} />
      <EditHoldingModal key={editingHolding?.id} holding={editingHolding} onClose={() => setEditingHolding(null)} />
    </>
  )
}
