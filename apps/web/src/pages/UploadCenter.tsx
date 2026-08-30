import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getUploads, getUploadTransactions } from '../api/uploads'

const PAGE_SIZES = [10, 20, 50]

interface Pagination {
  pageIndex: number
  pageSize: number
}

// Date-only columns are stored at UTC midnight; format in UTC so they don't slip
// a day in western timezones.
const fmtDate = (iso: string) =>
  new Date(iso).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: '2-digit', timeZone: 'UTC' })

function fmtRange(start: string | null, end: string | null) {
  if (!start && !end) return '—'
  if (start && end) return `${fmtDate(start)} – ${fmtDate(end)}`
  return fmtDate((start ?? end)!)
}

const fmtCurrency = (n: number) => (n ?? 0).toLocaleString('en-US', { style: 'currency', currency: 'USD' })
const fmtNumber = (n: number) => (n ?? 0).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const fmtSignedCurrency = (n: number) => `${(n ?? 0) < 0 ? '-' : '+'}${fmtCurrency(Math.abs(n ?? 0))}`

function amountColor(n: number) {
  if (n < 0) return 'text-error'
  if (n > 0) return 'text-secondary'
  return 'text-on-surface'
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
        <p className="text-label-sm text-on-surface-variant">
          Page {page.pageIndex + 1} of {pageCount}
        </p>
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

function StateRow({ colSpan, children }: { colSpan: number; children: React.ReactNode }) {
  return (
    <tr>
      <td colSpan={colSpan} className="px-6 py-10 text-center text-body-md text-on-surface-variant">{children}</td>
    </tr>
  )
}

export default function UploadCenter() {
  const [historyPage, setHistoryPage] = useState<Pagination>({ pageIndex: 0, pageSize: 10 })
  const [txnPage, setTxnPage] = useState<Pagination>({ pageIndex: 0, pageSize: 10 })

  const uploadsQuery = useQuery({
    queryKey: ['uploads', historyPage.pageIndex, historyPage.pageSize],
    queryFn: () => getUploads(historyPage.pageIndex + 1, historyPage.pageSize),
  })
  const txnsQuery = useQuery({
    queryKey: ['upload-transactions', txnPage.pageIndex, txnPage.pageSize],
    queryFn: () => getUploadTransactions(txnPage.pageIndex + 1, txnPage.pageSize),
  })

  const uploads = uploadsQuery.data?.data ?? []
  const txns = txnsQuery.data?.data ?? []

  return (
    <div className="p-8">
      {/* Page header */}
      <div className="mb-8">
        <h1 className="text-headline-lg text-on-surface mb-2">Upload Center</h1>
        <p className="text-body-lg text-on-surface-variant max-w-2xl">
          Review your imported files and the transactions they contained.
        </p>
      </div>

      <div className="space-y-6">
        {/* Upload History table */}
        <div className="bg-surface-container-lowest rounded-xl shadow-card">
          <div className="px-6 py-4 border-b border-outline-variant">
            <h2 className="text-headline-sm text-on-surface">Upload History</h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-outline-variant">
                  <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Uploaded</th>
                  <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">File Name</th>
                  <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Date Range</th>
                </tr>
              </thead>
              <tbody>
                {uploadsQuery.isLoading && <StateRow colSpan={3}>Loading uploads…</StateRow>}
                {uploadsQuery.isError && <StateRow colSpan={3}><span className="text-error">Failed to load uploads.</span></StateRow>}
                {!uploadsQuery.isLoading && !uploadsQuery.isError && uploads.length === 0 && <StateRow colSpan={3}>No uploads yet.</StateRow>}
                {uploads.map((row) => (
                  <tr key={row.id} className="border-b border-outline-variant last:border-0 hover:bg-surface-container-low transition-colors">
                    <td className="px-6 py-4 text-body-md text-on-surface-variant tabular-nums whitespace-nowrap">{fmtDate(row.created_at)}</td>
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-2">
                        <span className="material-symbols-outlined text-on-surface-variant text-base">description</span>
                        <span className="text-body-md text-on-surface">{row.file_name}</span>
                      </div>
                    </td>
                    <td className="px-6 py-4 text-right text-body-md text-on-surface-variant tabular-nums whitespace-nowrap">{fmtRange(row.start_date, row.end_date)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <TablePagination page={historyPage} setPage={setHistoryPage} total={uploadsQuery.data?.total ?? 0} />
        </div>

        {/* Upload Transactions table */}
        <div className="bg-surface-container-lowest rounded-xl shadow-card">
          <div className="flex items-center justify-between px-6 py-4 border-b border-outline-variant">
            <h2 className="text-headline-sm text-on-surface">Upload Transactions</h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-outline-variant">
                  <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Date</th>
                  <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Symbol</th>
                  <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Action</th>
                  <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Quantity</th>
                  <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Price</th>
                  <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Amount</th>
                </tr>
              </thead>
              <tbody>
                {txnsQuery.isLoading && <StateRow colSpan={6}>Loading transactions…</StateRow>}
                {txnsQuery.isError && <StateRow colSpan={6}><span className="text-error">Failed to load transactions.</span></StateRow>}
                {!txnsQuery.isLoading && !txnsQuery.isError && txns.length === 0 && <StateRow colSpan={6}>No upload transactions yet.</StateRow>}
                {txns.map((row) => (
                  <tr key={row.id} className="border-b border-outline-variant last:border-0 hover:bg-surface-container-low transition-colors">
                    <td className="px-6 py-4 text-body-md text-on-surface-variant tabular-nums whitespace-nowrap">{fmtDate(row.date)}</td>
                    <td className="px-6 py-4 text-body-md font-medium text-on-surface">{row.symbol || '—'}</td>
                    <td className="px-6 py-4">
                      <span className="text-body-md text-on-surface-variant block max-w-[22rem] truncate" title={row.action}>{row.action}</span>
                    </td>
                    <td className="px-6 py-4 text-right text-data-tabular text-on-surface tabular-nums">{row.quantity != null ? fmtNumber(row.quantity) : '—'}</td>
                    <td className="px-6 py-4 text-right text-data-tabular text-on-surface tabular-nums">{row.price != null ? fmtCurrency(row.price) : '—'}</td>
                    <td className={`px-6 py-4 text-right text-data-tabular font-semibold tabular-nums ${amountColor(row.amount)}`}>{fmtSignedCurrency(row.amount)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <TablePagination page={txnPage} setPage={setTxnPage} total={txnsQuery.data?.total ?? 0} />
        </div>
      </div>
    </div>
  )
}
