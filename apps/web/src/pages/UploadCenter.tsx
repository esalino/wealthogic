import { useState } from 'react'

type UploadStatus = 'Completed' | 'Failed' | 'Processing'

interface UploadRow {
  date: string
  fileName: string
  status: UploadStatus
  size: string
}

const uploadHistory: UploadRow[] = [
  { date: 'Oct 24 2023', fileName: 'Q3_Brokerage_Export.csv', status: 'Completed', size: '1.2 MB' },
  { date: 'Oct 21 2023', fileName: 'Manual_Additions_V2.xlsx', status: 'Completed', size: '840 KB' },
  { date: 'Oct 15 2023', fileName: 'Legacy_Portfolio_2019.csv', status: 'Failed', size: '4.1 MB' },
  { date: 'Oct 02 2023', fileName: 'Chase_Sept_Statement.pdf', status: 'Processing', size: '2.3 MB' },
  { date: 'Sep 28 2023', fileName: 'Wealthfront_Dividend_Log.csv', status: 'Completed', size: '45 KB' },
]

function statusChip(status: UploadStatus) {
  const base = 'inline-flex items-center px-2 py-0.5 rounded-full text-label-sm font-semibold'
  switch (status) {
    case 'Completed':
      return <span className={`${base} bg-secondary-container text-on-secondary-container`}>{status}</span>
    case 'Failed':
      return <span className={`${base} bg-error-container text-on-error-container`}>{status}</span>
    case 'Processing':
      return <span className={`${base} bg-surface-container-high text-on-surface-variant`}>{status}</span>
  }
}

export default function UploadCenter() {
  const [isDragging, setIsDragging] = useState(false)

  function handleDragOver(e: React.DragEvent<HTMLDivElement>) {
    e.preventDefault()
    setIsDragging(true)
  }

  function handleDragLeave() {
    setIsDragging(false)
  }

  function handleDrop(e: React.DragEvent<HTMLDivElement>) {
    e.preventDefault()
    setIsDragging(false)
    // handle dropped files here
  }

  return (
    <div className="p-8">
      {/* Page header */}
      <div className="mb-8">
        <h1 className="text-headline-lg text-on-surface mb-2">Upload Center</h1>
        <p className="text-body-lg text-on-surface-variant max-w-2xl">
          Import your financial data from CSV exports, Excel files, or PDF statements. Our AI will automatically parse and categorize your transactions.
        </p>
      </div>

      {/* Drag & Drop zone */}
      <div
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        className={`upload-dashed-border p-12 flex flex-col items-center justify-center mb-8 transition-colors ${
          isDragging ? 'bg-secondary-container/20' : 'bg-surface-container-lowest'
        }`}
      >
        <div className="w-16 h-16 rounded-xl bg-primary-fixed flex items-center justify-center mb-4">
          <span className="material-symbols-outlined text-primary text-4xl">upload_file</span>
        </div>
        <h2 className="text-headline-sm text-on-surface mb-2">Drag &amp; Drop CSV</h2>
        <p className="text-body-md text-on-surface-variant mb-6">
          or select files from your computer. Supports CSV, XLSX, and PDF formats.
        </p>
        <div className="flex gap-3">
          <label className="flex items-center gap-2 px-5 py-2.5 bg-primary text-on-primary rounded-lg text-body-md font-semibold cursor-pointer hover:opacity-90 transition-opacity">
            <span className="material-symbols-outlined text-xl">folder_open</span>
            Select Files
            <input type="file" className="hidden" multiple accept=".csv,.xlsx,.pdf" />
          </label>
          <button className="flex items-center gap-2 px-5 py-2.5 border border-outline-variant rounded-lg text-body-md text-on-surface hover:bg-surface-container-high transition-colors">
            <span className="material-symbols-outlined text-xl">description</span>
            View Templates
          </button>
        </div>
      </div>

      {/* Bento grid */}
      <div className="grid grid-cols-12 gap-6">
        {/* Guidelines column */}
        <div className="col-span-12 lg:col-span-4 space-y-4">
          {/* Supported Providers */}
          <div className="bg-surface-container-lowest rounded-xl shadow-card p-6">
            <h3 className="text-headline-sm text-on-surface mb-1">Supported Providers</h3>
            <p className="text-body-md text-on-surface-variant mb-4">Import directly from these institutions.</p>
            <div className="flex gap-3">
              {[
                { icon: 'account_balance', label: 'Banks' },
                { icon: 'finance', label: 'Brokers' },
                { icon: 'currency_bitcoin', label: 'Crypto' },
              ].map((p) => (
                <div key={p.label} className="flex-1 flex flex-col items-center gap-2 p-3 bg-surface-container-low rounded-lg">
                  <span className="material-symbols-outlined text-on-surface-variant text-2xl">{p.icon}</span>
                  <span className="text-label-sm text-on-surface-variant">{p.label}</span>
                </div>
              ))}
            </div>
            <div className="mt-4 space-y-2">
              {['Chase', 'Fidelity', 'Vanguard', 'Coinbase', 'Schwab'].map((name) => (
                <div key={name} className="flex items-center gap-2 text-body-md text-on-surface-variant">
                  <span className="material-symbols-outlined text-secondary text-base">check_circle</span>
                  {name}
                </div>
              ))}
            </div>
          </div>

          {/* Auto-Sync card */}
          <div className="bg-primary-container rounded-xl shadow-card p-6">
            <div className="w-10 h-10 rounded-lg bg-white/10 flex items-center justify-center mb-4">
              <span className="material-symbols-outlined text-on-primary text-xl">sync</span>
            </div>
            <h3 className="text-headline-sm text-on-primary mb-2">Auto-Sync</h3>
            <p className="text-body-md text-on-primary-container/80 mb-4">
              Connect your accounts via API for real-time automatic syncing. No more manual uploads.
            </p>
            <button className="flex items-center gap-2 px-4 py-2 bg-secondary text-on-secondary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity">
              <span className="material-symbols-outlined text-xl">settings_ethernet</span>
              Configure API
            </button>
          </div>
        </div>

        {/* Upload History table */}
        <div className="col-span-12 lg:col-span-8 bg-surface-container-lowest rounded-xl shadow-card">
          <div className="flex items-center justify-between px-6 py-4 border-b border-outline-variant">
            <h2 className="text-headline-sm text-on-surface">Upload History</h2>
            <button className="text-body-md text-secondary font-semibold hover:opacity-80 transition-opacity">Clear History</button>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-outline-variant">
                  <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Date</th>
                  <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">File Name</th>
                  <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Status</th>
                  <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Size</th>
                </tr>
              </thead>
              <tbody>
                {uploadHistory.map((row, i) => (
                  <tr key={i} className="border-b border-outline-variant last:border-0 hover:bg-surface-container-low transition-colors">
                    <td className="px-6 py-4 text-body-md text-on-surface-variant tabular-nums whitespace-nowrap">{row.date}</td>
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-2">
                        <span className="material-symbols-outlined text-on-surface-variant text-base">description</span>
                        <span className="text-body-md text-on-surface">{row.fileName}</span>
                      </div>
                    </td>
                    <td className="px-6 py-4">{statusChip(row.status)}</td>
                    <td className="px-6 py-4 text-right text-body-md text-on-surface-variant tabular-nums">{row.size}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="px-6 py-4 border-t border-outline-variant">
            <button className="w-full py-2 text-body-md text-secondary font-semibold hover:bg-surface-container-low rounded-lg transition-colors">
              Load Older History
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
