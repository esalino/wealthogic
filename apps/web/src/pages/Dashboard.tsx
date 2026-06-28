type StatusType = 'Processed' | 'Active' | 'Pending' | 'Error'

interface ActivityRow {
  type: string
  entity: string
  date: string
  status: StatusType
  amount: string
}

const activityData: ActivityRow[] = [
  {
    type: 'Monthly Statement',
    entity: 'Chase Sapphire Preferred',
    date: 'Oct 24 2023',
    status: 'Processed',
    amount: '-$1,420.50',
  },
  {
    type: 'New Account',
    entity: 'Vanguard Brokerage',
    date: 'Oct 22 2023',
    status: 'Active',
    amount: '$12,400.00',
  },
  {
    type: 'Asset Revaluation',
    entity: '123 Maple Street (Real Estate)',
    date: 'Oct 20 2023',
    status: 'Pending',
    amount: '+$45,000.00',
  },
  {
    type: 'Failed Upload',
    entity: 'Coinbase Wallet API',
    date: 'Oct 19 2023',
    status: 'Error',
    amount: '--',
  },
]

function statusChip(status: StatusType) {
  const base = 'inline-flex items-center px-2 py-0.5 rounded-full text-label-sm font-semibold'
  switch (status) {
    case 'Processed':
    case 'Active':
      return <span className={`${base} bg-secondary-container text-on-secondary-container`}>{status}</span>
    case 'Pending':
      return <span className={`${base} bg-surface-container-high text-on-surface-variant`}>{status}</span>
    case 'Error':
      return <span className={`${base} bg-error-container text-on-error-container`}>{status}</span>
  }
}

const barHeights = [40, 55, 35, 65, 50, 70, 85]

export default function Dashboard() {
  return (
    <div className="p-8">
      {/* Page header */}
      <div className="flex items-start justify-between mb-8">
        <div>
          <h1 className="text-headline-lg text-on-surface mb-1">Overview</h1>
          <p className="text-body-md text-on-surface-variant">Last updated: Today, 09:42 AM</p>
        </div>
        <div className="flex gap-3">
          <button className="flex items-center gap-2 px-4 py-2 border border-outline-variant rounded-lg text-body-md text-on-surface hover:bg-surface-container-high transition-colors">
            <span className="material-symbols-outlined text-xl">download</span>
            Export Report
          </button>
          <button className="flex items-center gap-2 px-4 py-2 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity">
            <span className="material-symbols-outlined text-xl">add</span>
            New Entry
          </button>
        </div>
      </div>

      {/* Bento grid row 1 */}
      <div className="grid grid-cols-12 gap-6 mb-6">
        {/* Total Net Worth card */}
        <div className="col-span-12 lg:col-span-7 bg-surface-container-lowest rounded-xl shadow-card p-6">
          <p className="text-label-caps text-on-surface-variant uppercase mb-1">Total Net Worth</p>
          <div className="flex items-baseline gap-3 mb-1">
            <span className="text-headline-lg text-on-surface tabular-nums">$2,845,920.42</span>
            <span className="text-label-sm font-semibold text-secondary">+4.2% vs last month</span>
          </div>
          <p className="text-body-md text-on-surface-variant mb-6">Across all connected accounts and assets</p>

          {/* Bar chart placeholder */}
          <div className="flex items-end gap-2 h-24">
            {barHeights.map((h, i) => (
              <div
                key={i}
                className={`flex-1 rounded-sm ${i === barHeights.length - 1 ? 'bg-secondary' : 'bg-primary-fixed-dim'}`}
                style={{ height: `${h}%` }}
              />
            ))}
          </div>
          <div className="flex justify-between mt-2">
            {['Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct'].map((m) => (
              <span key={m} className="text-label-sm text-on-surface-variant flex-1 text-center">{m}</span>
            ))}
          </div>
        </div>

        {/* Asset breakdown 2x2 */}
        <div className="col-span-12 lg:col-span-5 grid grid-cols-2 gap-4">
          {/* Cash */}
          <div className="bg-surface-container-lowest rounded-xl shadow-card p-4">
            <div className="w-9 h-9 rounded-lg bg-primary-fixed flex items-center justify-center mb-3">
              <span className="material-symbols-outlined text-on-primary-fixed text-xl">payments</span>
            </div>
            <p className="text-label-caps text-on-surface-variant uppercase mb-1">Cash</p>
            <p className="text-headline-sm text-on-surface tabular-nums mb-1">$142,500.00</p>
            <p className="text-label-sm text-on-surface-variant">3 Accounts Connected</p>
          </div>

          {/* Investments */}
          <div className="bg-surface-container-lowest rounded-xl shadow-card p-4">
            <div className="w-9 h-9 rounded-lg bg-secondary-container flex items-center justify-center mb-3">
              <span className="material-symbols-outlined text-on-secondary-container text-xl">trending_up</span>
            </div>
            <p className="text-label-caps text-on-surface-variant uppercase mb-1">Investments</p>
            <p className="text-headline-sm text-on-surface tabular-nums mb-1">$2,103,420.42</p>
            <p className="text-label-sm text-on-surface-variant">Brokerage, 401k, Crypto</p>
          </div>

          {/* Liabilities */}
          <div className="bg-surface-container-lowest rounded-xl shadow-card p-4">
            <div className="w-9 h-9 rounded-lg bg-error-container flex items-center justify-center mb-3">
              <span className="material-symbols-outlined text-on-tertiary-container text-xl">account_balance_wallet</span>
            </div>
            <p className="text-label-caps text-on-surface-variant uppercase mb-1">Liabilities</p>
            <p className="text-headline-sm text-on-surface tabular-nums mb-1">$400,000.00</p>
            <p className="text-label-sm text-on-surface-variant">Mortgage, Auto Loan</p>
          </div>

          {/* Quick action */}
          <div className="bg-primary rounded-xl shadow-card p-4 flex flex-col justify-between cursor-pointer hover:opacity-90 transition-opacity">
            <div className="w-9 h-9 rounded-lg bg-white/10 flex items-center justify-center mb-3">
              <span className="material-symbols-outlined text-on-primary text-xl">add_circle</span>
            </div>
            <div>
              <p className="text-label-caps text-on-primary/70 uppercase mb-1">Quick Action</p>
              <p className="text-headline-sm text-on-primary">New Asset</p>
            </div>
          </div>
        </div>
      </div>

      {/* Recent Activity table */}
      <div className="col-span-12 bg-surface-container-lowest rounded-xl shadow-card mb-6">
        <div className="flex items-center justify-between px-6 py-4 border-b border-outline-variant">
          <h2 className="text-headline-sm text-on-surface">Recent Activity</h2>
          <button className="text-body-md text-secondary font-semibold hover:opacity-80 transition-opacity">View All</button>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="border-b border-outline-variant">
                <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Type</th>
                <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Entity</th>
                <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Date</th>
                <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Status</th>
                <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Amount</th>
              </tr>
            </thead>
            <tbody>
              {activityData.map((row, i) => (
                <tr key={i} className="border-b border-outline-variant last:border-0 hover:bg-surface-container-low transition-colors">
                  <td className="px-6 py-4 text-body-md font-medium text-on-surface">{row.type}</td>
                  <td className="px-6 py-4 text-body-md text-on-surface-variant">{row.entity}</td>
                  <td className="px-6 py-4 text-body-md text-on-surface-variant tabular-nums">{row.date}</td>
                  <td className="px-6 py-4">{statusChip(row.status)}</td>
                  <td className="px-6 py-4 text-right text-data-tabular font-medium text-on-surface tabular-nums">{row.amount}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Footer row */}
      <div className="grid grid-cols-12 gap-6">
        {/* Financial Health card */}
        <div className="col-span-12 lg:col-span-4 bg-primary-container rounded-xl shadow-card p-6">
          <p className="text-label-caps text-on-primary-container uppercase mb-1">Financial Health Score</p>
          <div className="flex items-baseline gap-2 mb-4">
            <span className="text-headline-lg text-on-primary font-bold">82%</span>
            <span className="text-label-sm text-on-primary-container">Excellent</span>
          </div>
          <div className="w-full bg-white/20 rounded-full h-2 mb-3">
            <div className="bg-secondary h-2 rounded-full" style={{ width: '82%' }} />
          </div>
          <p className="text-body-md text-on-primary-container/80">Your portfolio is well diversified with strong growth potential.</p>
        </div>

        {/* Intelligence Hub card */}
        <div className="col-span-12 lg:col-span-8 bg-surface-container-lowest rounded-xl shadow-card p-6 flex gap-6">
          {/* Placeholder image */}
          <div className="w-48 h-32 rounded-lg bg-surface-container-high flex-shrink-0 flex items-center justify-center">
            <span className="material-symbols-outlined text-on-surface-variant text-4xl">auto_awesome</span>
          </div>
          <div className="flex flex-col justify-between">
            <div>
              <p className="text-label-caps text-on-surface-variant uppercase mb-2">Intelligence Hub</p>
              <h3 className="text-headline-sm text-on-surface mb-2">AI-Powered Financial Insights</h3>
              <p className="text-body-md text-on-surface-variant">
                Our AI analyzes your portfolio to surface personalized recommendations, risk alerts, and growth opportunities tailored to your goals.
              </p>
            </div>
            <button className="mt-4 self-start flex items-center gap-2 px-4 py-2 border border-outline-variant rounded-lg text-body-md text-on-surface hover:bg-surface-container-high transition-colors">
              Explore Recommendations
              <span className="material-symbols-outlined text-xl">arrow_forward</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
