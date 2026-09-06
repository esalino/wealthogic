import { useState } from 'react'

// Hardcoded placeholders until this is wired to the tax-lot / transaction APIs.
// Each row is a realized lot disposal: shares of a purchase lot that were sold.
interface RealizedGain {
  asset: string
  type: string // Stock, Treasury, ETF, Option, Crypto, ...
  acquired: string // acquisition date (ISO)
  realized: string // sale date (ISO)
  gain: number // capital gain/loss for this disposal
}

const realizedGains: RealizedGain[] = [
  { asset: 'AAPL', type: 'Stock', acquired: '2023-05-10', realized: '2026-02-15', gain: 4200 },
  { asset: 'MSFT', type: 'Stock', acquired: '2025-11-01', realized: '2026-03-20', gain: 1500 },
  { asset: 'TLT', type: 'Treasury', acquired: '2024-01-15', realized: '2026-04-10', gain: -800 },
  { asset: 'NVDA', type: 'Stock', acquired: '2026-01-05', realized: '2026-06-18', gain: 9800 },
  { asset: 'VTI', type: 'ETF', acquired: '2022-03-01', realized: '2026-05-22', gain: 12500 },
  { asset: 'BND', type: 'ETF', acquired: '2025-08-10', realized: '2026-07-01', gain: -350 },
  { asset: 'KO', type: 'Stock', acquired: '2021-06-15', realized: '2026-01-30', gain: 2100 },
  { asset: 'T-BILL 04/16', type: 'Treasury', acquired: '2025-10-16', realized: '2026-04-16', gain: 180 },
  { asset: 'TSLA', type: 'Stock', acquired: '2024-09-12', realized: '2026-08-05', gain: -2300 },
  { asset: 'AMZN', type: 'Stock', acquired: '2026-02-01', realized: '2026-07-20', gain: 3400 },
  { asset: 'JPM', type: 'Stock', acquired: '2023-11-20', realized: '2026-03-11', gain: 5600 },
  { asset: 'ETH', type: 'Crypto', acquired: '2025-12-01', realized: '2026-06-30', gain: 7200 },
  { asset: 'GOOGL', type: 'Stock', acquired: '2022-01-10', realized: '2025-09-15', gain: 8900 },
  { asset: 'XOM', type: 'Stock', acquired: '2025-02-01', realized: '2025-11-20', gain: -1200 },
  { asset: 'BND', type: 'ETF', acquired: '2020-05-05', realized: '2025-06-10', gain: 900 },
  { asset: 'JNJ', type: 'Stock', acquired: '2019-03-01', realized: '2024-10-05', gain: 3300 },
]

const YEARS = [2026, 2025, 2024]
const PAGE_SIZES = [10, 20, 50]

interface Pagination {
  pageIndex: number
  pageSize: number
}

const fmtCurrency = (n: number) => (n ?? 0).toLocaleString('en-US', { style: 'currency', currency: 'USD' })
const fmtSigned = (n: number) => `${(n ?? 0) < 0 ? '-' : '+'}${fmtCurrency(Math.abs(n ?? 0))}`
const fmtDate = (iso: string) =>
  new Date(iso).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: '2-digit', timeZone: 'UTC' })

function gainColor(n: number) {
  if (n < 0) return 'text-error'
  if (n > 0) return 'text-secondary'
  return 'text-on-surface'
}

function daysHeld(acquired: string, realized: string) {
  return Math.round((new Date(realized).getTime() - new Date(acquired).getTime()) / 86_400_000)
}
function isLongTerm(acquired: string, realized: string) {
  return daysHeld(acquired, realized) >= 365
}
function heldLabel(acquired: string, realized: string) {
  const d = daysHeld(acquired, realized)
  const y = Math.floor(d / 365)
  const m = Math.floor((d % 365) / 30)
  if (y > 0) return `${y}y ${m}m`
  if (m > 0) return `${m}m`
  return `${d}d`
}

function StatTile({ label, value }: { label: string; value: number }) {
  return (
    <div className="bg-surface-container-lowest rounded-xl shadow-card p-6">
      <p className="text-label-caps text-on-surface-variant uppercase mb-1">{label}</p>
      <p className={`text-headline-md font-bold tabular-nums ${gainColor(value)}`}>{fmtSigned(value)}</p>
    </div>
  )
}

function TablePagination({ page, setPage, total }: { page: Pagination; setPage: (p: Pagination) => void; total: number }) {
  const pageCount = Math.max(1, Math.ceil(total / page.pageSize))
  const canPrev = page.pageIndex > 0
  const canNext = page.pageIndex < pageCount - 1

  return (
    <div className="flex items-center justify-between px-6 py-4 border-t border-outline-variant">
      <div className="flex items-center gap-2">
        <span className="text-label-sm text-on-surface-variant">Rows per page</span>
        <select
          value={page.pageSize}
          onChange={(e) => setPage({ pageIndex: 0, pageSize: Number(e.target.value) })}
          className="px-2 py-1 bg-surface-container-low border border-outline-variant rounded-lg text-label-sm text-on-surface focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors"
        >
          {PAGE_SIZES.map((size) => <option key={size} value={size}>{size}</option>)}
        </select>
      </div>
      <div className="flex items-center gap-3">
        <p className="text-label-sm text-on-surface-variant">Page {page.pageIndex + 1} of {pageCount}</p>
        <div className="flex items-center gap-1">
          <button
            onClick={() => setPage({ ...page, pageIndex: page.pageIndex - 1 })}
            disabled={!canPrev}
            className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors disabled:opacity-40"
          >
            <span className="material-symbols-outlined text-xl">chevron_left</span>
          </button>
          <button
            onClick={() => setPage({ ...page, pageIndex: page.pageIndex + 1 })}
            disabled={!canNext}
            className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors disabled:opacity-40"
          >
            <span className="material-symbols-outlined text-xl">chevron_right</span>
          </button>
        </div>
      </div>
    </div>
  )
}

export default function TaxCenter() {
  const [year, setYear] = useState(2026)
  const [page, setPage] = useState<Pagination>({ pageIndex: 0, pageSize: 10 })

  const rows = realizedGains
    .filter((r) => new Date(r.realized).getUTCFullYear() === year)
    .sort((a, b) => new Date(b.realized).getTime() - new Date(a.realized).getTime())

  const total = rows.reduce((sum, r) => sum + r.gain, 0)
  const shortTerm = rows.filter((r) => !isLongTerm(r.acquired, r.realized)).reduce((s, r) => s + r.gain, 0)
  const longTerm = rows.filter((r) => isLongTerm(r.acquired, r.realized)).reduce((s, r) => s + r.gain, 0)

  const pageRows = rows.slice(page.pageIndex * page.pageSize, page.pageIndex * page.pageSize + page.pageSize)

  return (
    <div className="p-8">
      {/* Page header */}
      <div className="flex items-start justify-between mb-8">
        <div>
          <h1 className="text-headline-lg text-on-surface mb-1">Tax Center</h1>
          <p className="text-body-lg text-on-surface-variant">Realized capital gains and losses for the tax year.</p>
        </div>
        <div className="flex items-center gap-2">
          <label className="text-label-sm text-on-surface-variant">Tax year</label>
          <select
            value={year}
            onChange={(e) => { setYear(Number(e.target.value)); setPage({ pageIndex: 0, pageSize: page.pageSize }) }}
            className="px-3 py-2 bg-surface-container-lowest border border-outline-variant rounded-lg text-body-md font-semibold text-on-surface focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors"
          >
            {YEARS.map((y) => <option key={y} value={y}>{y}</option>)}
          </select>
        </div>
      </div>

      {/* Summary tiles */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-6 mb-6">
        <StatTile label={`Total Realized ${year}`} value={total} />
        <StatTile label="Short-term" value={shortTerm} />
        <StatTile label="Long-term" value={longTerm} />
      </div>

      {/* Realized gains table */}
      <div className="bg-surface-container-lowest rounded-xl shadow-card">
        <div className="px-6 py-4 border-b border-outline-variant">
          <h2 className="text-headline-sm text-on-surface">Realized Gains &amp; Losses</h2>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="border-b border-outline-variant">
                <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Asset</th>
                <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Type</th>
                <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Realized Date</th>
                <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Holding Period</th>
                <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Capital Gain/Loss</th>
              </tr>
            </thead>
            <tbody>
              {pageRows.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-6 py-10 text-center text-body-md text-on-surface-variant">No realized gains in {year}.</td>
                </tr>
              )}
              {pageRows.map((r, i) => {
                const long = isLongTerm(r.acquired, r.realized)
                return (
                  <tr key={i} className="border-b border-outline-variant last:border-0 hover:bg-surface-container-low transition-colors">
                    <td className="px-6 py-4 text-body-md font-medium text-on-surface">{r.asset}</td>
                    <td className="px-6 py-4">
                      <span className="inline-flex items-center px-2 py-0.5 rounded-full bg-surface-container-high text-on-surface-variant text-label-sm font-semibold">{r.type}</span>
                    </td>
                    <td className="px-6 py-4 text-body-md text-on-surface-variant tabular-nums whitespace-nowrap">{fmtDate(r.realized)}</td>
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-2">
                        <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-label-sm font-semibold ${long ? 'bg-secondary-container text-on-secondary-container' : 'bg-surface-container-high text-on-surface-variant'}`}>
                          {long ? 'Long-term' : 'Short-term'}
                        </span>
                        <span className="text-label-sm text-on-surface-variant tabular-nums">{heldLabel(r.acquired, r.realized)}</span>
                      </div>
                    </td>
                    <td className={`px-6 py-4 text-right text-data-tabular font-semibold tabular-nums ${gainColor(r.gain)}`}>{fmtSigned(r.gain)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        <TablePagination page={page} setPage={setPage} total={rows.length} />
      </div>
    </div>
  )
}
