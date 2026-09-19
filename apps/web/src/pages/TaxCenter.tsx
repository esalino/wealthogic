import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  getRealizedEvents,
  getTaxSummary,
  type JurisdictionSummary,
  type TaxTreatment,
} from '../api/tax'

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

// A line in a card. `signed` colors and signs the value like a gain/loss;
// `muted` is for figures that inform without adding to the total.
function Line({ label, value, signed, muted, indent, note }: {
  label: string
  value: number
  signed?: boolean
  muted?: boolean
  indent?: boolean
  note?: string
}) {
  const color = muted ? 'text-on-surface-variant' : signed ? gainColor(value) : 'text-on-surface'
  return (
    <div className="flex items-baseline justify-between py-2">
      <p className={`text-body-md ${muted ? 'text-on-surface-variant' : 'text-on-surface'} ${indent ? 'pl-4' : ''}`}>
        {label}
        {note && <span className="text-label-sm text-on-surface-variant"> · {note}</span>}
      </p>
      <p className={`text-data-tabular font-semibold tabular-nums ${color}`}>
        {signed ? fmtSigned(value) : fmtCurrency(value)}
      </p>
    </div>
  )
}

function SectionHeading({ title, total }: { title: string; total: number }) {
  return (
    <div className="flex items-baseline justify-between pt-3 pb-1">
      <p className="text-label-caps text-on-surface-variant uppercase">{title}</p>
      <p className="text-data-tabular font-bold tabular-nums text-on-surface">{fmtCurrency(total)}</p>
    </div>
  )
}

// BucketCard renders one jurisdiction: its capital gains after netting, its
// ordinary income, and what it excluded.
//
// The two are shown apart because they are taxed apart - capital losses net
// against capital gains only, and never reduce interest or dividends. Showing
// one combined total was hiding a capital loss eating into interest income.
function BucketCard({ jurisdiction, year }: { jurisdiction: JurisdictionSummary; year: number }) {
  const cap = jurisdiction.capital
  const hasCapital = cap.short_term !== 0 || cap.long_term !== 0
  const hasOrdinary = jurisdiction.ordinary.length > 0
  const carried = cap.loss_carryforward > 0

  return (
    <div className="bg-surface-container-lowest rounded-xl shadow-card p-6">
      <div className="flex items-start justify-between mb-2">
        <div>
          <h3 className="text-label-caps text-on-surface-variant uppercase">{jurisdiction.name}</h3>
          <p className="text-label-sm text-on-surface-variant">Taxable income for {year}</p>
        </div>
        <div className="text-right">
          <p className="text-label-caps text-on-surface-variant uppercase">Taxable</p>
          <p className="text-headline-md font-bold tabular-nums text-on-surface">
            {fmtCurrency(jurisdiction.taxable_total)}
          </p>
        </div>
      </div>

      <div className="divide-y divide-outline-variant border-t border-outline-variant">
        {hasCapital && (
          <div>
            <SectionHeading title="Capital gains" total={cap.taxable} />
            <Line label="Short-term" value={cap.short_term} signed indent />
            <Line label="Long-term" value={cap.long_term} signed indent />
            {cap.offset > 0 && (
              <Line
                label="Offset between periods"
                value={-cap.offset}
                muted
                indent
                note="loss applied to the other period's gain"
              />
            )}
            <div className="border-t border-outline-variant/60">
              <Line label="Net capital" value={cap.net} signed indent />
            </div>
            {carried && (
              <Line
                label="Carried forward"
                value={cap.loss_carryforward}
                muted
                indent
                note="net loss, not deductible against income below"
              />
            )}
          </div>
        )}

        {hasOrdinary && (
          <div>
            <SectionHeading title="Ordinary income" total={jurisdiction.ordinary_total} />
            {jurisdiction.ordinary.map((b) => (
              <Line key={b.character} label={b.label} value={b.amount} indent />
            ))}
          </div>
        )}

        {jurisdiction.excluded.length > 0 && (
          <div className="pt-1">
            {jurisdiction.excluded.map((e) => (
              <div key={e.reason} className="flex items-baseline justify-between py-2">
                <p className="text-body-md text-on-surface-variant">Excluded — {e.label}</p>
                <p className="text-data-tabular font-semibold tabular-nums text-on-surface-variant">
                  -{fmtCurrency(Math.abs(e.amount))}
                </p>
              </div>
            ))}
          </div>
        )}

        {!hasCapital && !hasOrdinary && jurisdiction.excluded.length === 0 && (
          <p className="py-2.5 text-body-md text-on-surface-variant">No taxable activity.</p>
        )}
      </div>
    </div>
  )
}

// TreatmentBadge shows how one jurisdiction taxes a realized event. Two badges
// on a row that disagree - taxable federally, exempt at the state level - is the
// point: the same event can land differently in each jurisdiction.
function TreatmentBadge({ treatment }: { treatment: TaxTreatment }) {
  const style = treatment.taxable
    ? 'bg-surface-container-high text-on-surface-variant'
    : 'bg-secondary-container text-on-secondary-container'
  return (
    <span
      className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-label-sm font-semibold ${style}`}
      title={treatment.reason}
    >
      <span className="opacity-70">{treatment.jurisdiction_code}</span>
      {CHARACTER_SHORT[treatment.character] ?? treatment.character}
    </span>
  )
}

// Short forms so a row of badges stays readable; an unmapped character falls
// back to its raw value rather than being hidden.
const CHARACTER_SHORT: Record<string, string> = {
  long_term_capital: 'Long-term',
  short_term_capital: 'Short-term',
  qualified_eligible: 'Qualified',
  ordinary: 'Ordinary',
  exempt: 'Exempt',
  deferred: 'Deferred',
}

// What a realized row actually is. Not every disposal is a capital gain:
// redeeming a Treasury realizes accreted discount, which is interest.
const CATEGORY_LABEL: Record<string, string> = {
  capital_gain: 'Capital gain',
  interest: 'Interest',
  dividend: 'Dividend',
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
  const [category, setCategory] = useState('')
  const [page, setPage] = useState<Pagination>({ pageIndex: 0, pageSize: 10 })

  // Everything realized in the year, from both ledgers - capital gains,
  // Treasury interest, and dividends alike. Fetching only gains here is what
  // left income off the list while the jurisdiction cards still counted it.
  const { data, isLoading, isError } = useQuery({
    queryKey: ['tax', 'events', year, category, page.pageIndex, page.pageSize],
    queryFn: () => getRealizedEvents(year, page.pageIndex + 1, page.pageSize, { category }),
  })

  // Taxable income per jurisdiction, already bucketed and labeled by the API
  // from the tax treatments stored when each event was realized. The page no
  // longer decides what's taxable where - the jurisdiction rules do.
  const { data: taxData, isLoading: taxLoading } = useQuery({
    queryKey: ['tax', 'summary', year],
    queryFn: () => getTaxSummary(year),
  })

  const rows = data?.data ?? []
  const summary = data?.summary ?? { total: 0, capital_gains: 0, interest: 0, dividends: 0 }
  const jurisdictions = taxData?.jurisdictions ?? []

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

      {/* One card per jurisdiction the income is subject to. The set and the
          shape of each card come from the API, so adding a jurisdiction's rules
          adds a card here with no change to this page. */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        {taxLoading && (
          <div className="bg-surface-container-lowest rounded-xl shadow-card p-6 text-body-md text-on-surface-variant">
            Loading tax summary…
          </div>
        )}
        {!taxLoading && jurisdictions.length === 0 && (
          <div className="bg-surface-container-lowest rounded-xl shadow-card p-6 text-body-md text-on-surface-variant">
            No taxable activity in {year}.
          </div>
        )}
        {jurisdictions.map((j) => (
          <BucketCard key={j.code} jurisdiction={j} year={year} />
        ))}
      </div>

      {/* The realized ledger: one table now that disposals and income share it,
          so a category filter is a plain query rather than a union. */}
      <div className="bg-surface-container-lowest rounded-xl shadow-card">
        <div className="px-6 py-4 border-b border-outline-variant">
          <div className="flex items-baseline justify-between gap-4 flex-wrap">
            <h2 className="text-headline-sm text-on-surface">Realized Income &amp; Gains</h2>
            <div className="flex items-center gap-2">
              <select
                value={category}
                onChange={(e) => { setCategory(e.target.value); setPage({ pageIndex: 0, pageSize: page.pageSize }) }}
                className="px-2 py-1 bg-surface-container-low border border-outline-variant rounded-lg text-label-sm text-on-surface focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors"
              >
                <option value="">All kinds</option>
                {Object.entries(CATEGORY_LABEL).map(([value, lbl]) => (
                  <option key={value} value={value}>{lbl}</option>
                ))}
              </select>
            </div>
          </div>
          {/* Totals by category, so this list visibly reconciles against the
              jurisdiction cards above rather than appearing to fall short. */}
          <div className="mt-2 flex items-baseline gap-3 text-label-sm text-on-surface-variant flex-wrap">
              <span>Capital gains <span className="tabular-nums text-on-surface">{fmtCurrency(summary.capital_gains)}</span></span>
              <span aria-hidden>·</span>
              <span>Interest <span className="tabular-nums text-on-surface">{fmtCurrency(summary.interest)}</span></span>
              <span aria-hidden>·</span>
              <span>Dividends <span className="tabular-nums text-on-surface">{fmtCurrency(summary.dividends)}</span></span>
              <span aria-hidden>·</span>
              <span className="font-semibold">Total <span className="tabular-nums text-on-surface">{fmtCurrency(summary.total)}</span></span>
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="border-b border-outline-variant">
                <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Asset</th>
                <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Kind</th>
                <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Realized Date</th>
                <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Held</th>
                <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Tax Treatment</th>
                <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Realized</th>
              </tr>
            </thead>
            <tbody>
              {isLoading && (
                <tr><td colSpan={6} className="px-6 py-10 text-center text-body-md text-on-surface-variant">Loading realized income…</td></tr>
              )}
              {isError && (
                <tr><td colSpan={6} className="px-6 py-10 text-center text-body-md text-error">Failed to load realized income.</td></tr>
              )}
              {!isLoading && !isError && rows.length === 0 && (
                <tr><td colSpan={6} className="px-6 py-10 text-center text-body-md text-on-surface-variant">No realized income or gains in {year}.</td></tr>
              )}
              {rows.map((g) => {
                // Holding period only characterizes a capital gain - a Treasury's
                // accreted discount is interest however long it was held, and
                // income has no lot behind it at all.
                const isCapital = g.category === 'capital_gain'
                const long = g.term === 'long'
                return (
                  <tr key={g.id} className="border-b border-outline-variant last:border-0 hover:bg-surface-container-low transition-colors">
                    <td className="px-6 py-4 text-body-md font-medium text-on-surface">{g.symbol || '—'}</td>
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-2">
                        <span className="inline-flex items-center px-2 py-0.5 rounded-full bg-surface-container-high text-on-surface-variant text-label-sm font-semibold">
                          {CATEGORY_LABEL[g.category] ?? g.category}
                        </span>
                        <span className="text-label-sm text-on-surface-variant">{g.asset_type || '—'}</span>
                      </div>
                    </td>
                    <td className="px-6 py-4 text-body-md text-on-surface-variant tabular-nums whitespace-nowrap">{fmtDate(g.event_date)}</td>
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-2">
                        {isCapital && (
                          <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-label-sm font-semibold ${long ? 'bg-secondary-container text-on-secondary-container' : 'bg-surface-container-high text-on-surface-variant'}`}>
                            {long ? 'Long-term' : 'Short-term'}
                          </span>
                        )}
                        {g.acquired_date ? (
                          <span className="text-label-sm text-on-surface-variant tabular-nums">
                            {heldLabel(g.acquired_date, g.event_date)}
                          </span>
                        ) : (
                          <span className="text-label-sm text-on-surface-variant">—</span>
                        )}
                      </div>
                    </td>
                    <td className="px-6 py-4">
                      <div className="flex flex-wrap items-center gap-1.5">
                        {(g.treatments ?? []).map((t) => (
                          <TreatmentBadge key={t.jurisdiction_code} treatment={t} />
                        ))}
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
