import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getGains } from '../api/gains'
import { getDistributions } from '../api/distributions'

const PAGE_SIZES = [10, 20, 50]

// Recent tax years for the selector; the data drives what actually shows.
const now = new Date()
const YEARS = Array.from({ length: 5 }, (_, i) => now.getUTCFullYear() - i)

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

function heldLabel(acquired: string, realized: string) {
  const d = Math.round((new Date(realized).getTime() - new Date(acquired).getTime()) / 86_400_000)
  const y = Math.floor(d / 365)
  const m = Math.floor((d % 365) / 30)
  if (y > 0) return `${y}y ${m}m`
  if (m > 0) return `${m}m`
  return `${d}d`
}

// A line in a tax bucket. `signed` colors the value like a gain/loss (green/red)
// and shows an explicit sign; otherwise it's a plain neutral income amount.
interface BucketRow {
  label: string
  value: number
  sub?: string // small detail under the label (e.g. the long/short split)
  note?: string // inline qualifier after the label (e.g. "all ordinary")
  signed?: boolean
}

// BucketCard renders one jurisdiction (Federal or State): its taxable buckets
// and the taxable total. Federal and State intentionally differ in shape -
// Federal breaks gains down by rate character, State collapses them and instead
// lists what's excluded.
function BucketCard({
  title,
  subtitle,
  rows,
  total,
  excluded,
}: {
  title: string
  subtitle: string
  rows: BucketRow[]
  total: number
  excluded?: { label: string; value: number }
}) {
  return (
    <div className="bg-surface-container-lowest rounded-xl shadow-card p-6">
      <div className="flex items-start justify-between mb-3">
        <div>
          <h3 className="text-label-caps text-on-surface-variant uppercase">{title}</h3>
          <p className="text-label-sm text-on-surface-variant">{subtitle}</p>
        </div>
        <div className="text-right">
          <p className="text-label-caps text-on-surface-variant uppercase">Taxable</p>
          <p className="text-headline-md font-bold tabular-nums text-on-surface">{fmtCurrency(total)}</p>
        </div>
      </div>
      <div className="divide-y divide-outline-variant border-t border-outline-variant">
        {rows.map((r) => (
          <div key={r.label} className="flex items-baseline justify-between py-2.5">
            <div>
              <p className="text-body-md text-on-surface">
                {r.label}
                {r.note && <span className="text-label-sm text-on-surface-variant"> · {r.note}</span>}
              </p>
              {r.sub && <p className="text-label-sm text-on-surface-variant tabular-nums">{r.sub}</p>}
            </div>
            <p className={`text-data-tabular font-semibold tabular-nums ${r.signed ? gainColor(r.value) : 'text-on-surface'}`}>
              {r.signed ? fmtSigned(r.value) : fmtCurrency(r.value)}
            </p>
          </div>
        ))}
        {excluded && excluded.value !== 0 && (
          <div className="flex items-baseline justify-between py-2.5">
            <p className="text-body-md text-on-surface-variant">{excluded.label}</p>
            <p className="text-data-tabular font-semibold tabular-nums text-on-surface-variant">
              -{fmtCurrency(Math.abs(excluded.value))}
            </p>
          </div>
        )}
      </div>
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
  const [year, setYear] = useState(now.getUTCFullYear())
  const [page, setPage] = useState<Pagination>({ pageIndex: 0, pageSize: 10 })

  const { data, isLoading, isError } = useQuery({
    queryKey: ['gains', year, page.pageIndex, page.pageSize],
    queryFn: () => getGains(year, page.pageIndex + 1, page.pageSize),
  })

  // Year-scoped income totals for the buckets. Only the summary is needed here,
  // so ask for the smallest page.
  const { data: distData } = useQuery({
    queryKey: ['distributions', 'summary', year],
    queryFn: () => getDistributions(undefined, year, 1, 1),
  })

  const rows = data?.data ?? []
  const gains = data?.summary ?? { total: 0, short_term: 0, long_term: 0 }
  const income = distData?.summary ?? { total: 0, dividend: 0, other_income: 0 }

  // Buckets over the data we have today. Treasury income is still recorded as a
  // capital gain (reclassification deferred), so the state-exempt Treasury line
  // stays dormant at 0 and Federal/State totals match until that lands.
  const capNet = gains.total
  const treasuryExempt = 0
  const fedTaxable = capNet + income.dividend + income.other_income
  const stateTaxable = fedTaxable - treasuryExempt

  return (
    <div className="p-8">
      {/* Page header */}
      <div className="flex items-start justify-between mb-8">
        <div>
          <h1 className="text-headline-lg text-on-surface mb-1">Tax Center</h1>
          <p className="text-body-lg text-on-surface-variant">Realized gains and taxable income for the tax year.</p>
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

      {/* Tax buckets, split by jurisdiction */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        <BucketCard
          title="Federal"
          subtitle={`Taxable income for ${year}`}
          total={fedTaxable}
          rows={[
            {
              label: 'Capital gains — net',
              value: capNet,
              sub: `long ${fmtSigned(gains.long_term)} · short ${fmtSigned(gains.short_term)}`,
              signed: true,
            },
            { label: 'Dividends', value: income.dividend },
            { label: 'Interest & other income', value: income.other_income },
          ]}
        />
        <BucketCard
          title="State"
          subtitle={`Taxable income for ${year}`}
          total={stateTaxable}
          rows={[
            { label: 'Capital gains — net', value: capNet, signed: true },
            { label: 'Dividends', value: income.dividend, note: 'all ordinary' },
            { label: 'Interest & other income', value: income.other_income },
          ]}
          excluded={{ label: 'Excluded — U.S. Treasury', value: treasuryExempt }}
        />
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
              {isLoading && (
                <tr><td colSpan={5} className="px-6 py-10 text-center text-body-md text-on-surface-variant">Loading gains…</td></tr>
              )}
              {isError && (
                <tr><td colSpan={5} className="px-6 py-10 text-center text-body-md text-error">Failed to load gains.</td></tr>
              )}
              {!isLoading && !isError && rows.length === 0 && (
                <tr><td colSpan={5} className="px-6 py-10 text-center text-body-md text-on-surface-variant">No realized gains in {year}.</td></tr>
              )}
              {rows.map((g) => {
                const long = g.term === 'long'
                return (
                  <tr key={g.id} className="border-b border-outline-variant last:border-0 hover:bg-surface-container-low transition-colors">
                    <td className="px-6 py-4 text-body-md font-medium text-on-surface">{g.symbol || '—'}</td>
                    <td className="px-6 py-4">
                      <span className="inline-flex items-center px-2 py-0.5 rounded-full bg-surface-container-high text-on-surface-variant text-label-sm font-semibold">{g.asset_type || '—'}</span>
                    </td>
                    <td className="px-6 py-4 text-body-md text-on-surface-variant tabular-nums whitespace-nowrap">{fmtDate(g.realized_date)}</td>
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-2">
                        <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-label-sm font-semibold ${long ? 'bg-secondary-container text-on-secondary-container' : 'bg-surface-container-high text-on-surface-variant'}`}>
                          {long ? 'Long-term' : 'Short-term'}
                        </span>
                        <span className="text-label-sm text-on-surface-variant tabular-nums">{heldLabel(g.acquired_date, g.realized_date)}</span>
                      </div>
                    </td>
                    <td className={`px-6 py-4 text-right text-data-tabular font-semibold tabular-nums ${gainColor(g.amount)}`}>{fmtSigned(g.amount)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        <TablePagination page={page} setPage={setPage} total={data?.total ?? 0} />
      </div>
    </div>
  )
}
