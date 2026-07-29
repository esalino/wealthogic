import { Fragment, useState, useEffect } from 'react'

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
  symbol: string
  name: string
  assetClass: string
  price: string
  quantity: string
  marketValue: string
  change: string
  changeDir: ChangeDirection
  allocation: string
  taxLots: TaxLot[]
  transactions: Transaction[]
  dividends: Dividend[]
}

const holdings: Holding[] = [
  {
    symbol: 'AAPL',
    name: 'Apple Inc.',
    assetClass: 'Technology',
    price: '$189.43',
    quantity: '1,240.00',
    marketValue: '$234,893.20',
    change: '+1.24%',
    changeDir: 'positive',
    allocation: '9.46%',
    taxLots: [
      { dateAcquired: 'Oct 12, 2023', quantity: '500.00', costBasis: '$178.20', currentValue: '$94,715.00' },
      { dateAcquired: 'Jan 15, 2024', quantity: '740.00', costBasis: '$185.50', currentValue: '$140,178.20' },
    ],
    transactions: [
      { date: 'Jan 15, 2024', action: 'Buy', quantity: '740.00', price: '$185.50', amount: '-$137,270.00', amountDir: 'negative' },
      { date: 'Oct 12, 2023', action: 'Buy', quantity: '500.00', price: '$178.20', amount: '-$89,100.00', amountDir: 'negative' },
    ],
    dividends: [
      { date: 'May 16, 2024', perShare: '$0.25', shares: '1,240.00', total: '$310.00' },
      { date: 'Feb 15, 2024', perShare: '$0.24', shares: '1,240.00', total: '$297.60' },
    ],
  },
  {
    symbol: 'MSFT',
    name: 'Microsoft Corp.',
    assetClass: 'Technology',
    price: '$415.10',
    quantity: '480.00',
    marketValue: '$199,248.00',
    change: '-0.82%',
    changeDir: 'negative',
    allocation: '8.02%',
    taxLots: [
      { dateAcquired: 'Mar 03, 2023', quantity: '280.00', costBasis: '$255.40', currentValue: '$116,228.00' },
      { dateAcquired: 'Aug 21, 2023', quantity: '200.00', costBasis: '$327.15', currentValue: '$83,020.00' },
    ],
    transactions: [
      { date: 'Aug 21, 2023', action: 'Buy', quantity: '200.00', price: '$327.15', amount: '-$65,430.00', amountDir: 'negative' },
      { date: 'Mar 03, 2023', action: 'Buy', quantity: '280.00', price: '$255.40', amount: '-$71,512.00', amountDir: 'negative' },
    ],
    dividends: [
      { date: 'Jun 13, 2024', perShare: '$0.75', shares: '480.00', total: '$360.00' },
      { date: 'Mar 14, 2024', perShare: '$0.75', shares: '480.00', total: '$360.00' },
    ],
  },
  {
    symbol: 'VTI',
    name: 'Vanguard Total Stock',
    assetClass: 'ETF • Diversified',
    price: '$252.18',
    quantity: '3,500.00',
    marketValue: '$882,630.00',
    change: '+0.45%',
    changeDir: 'positive',
    allocation: '35.56%',
    taxLots: [
      { dateAcquired: 'Jan 04, 2022', quantity: '2,000.00', costBasis: '$221.05', currentValue: '$504,360.00' },
      { dateAcquired: 'Jul 19, 2023', quantity: '1,500.00', costBasis: '$213.80', currentValue: '$378,270.00' },
    ],
    transactions: [
      { date: 'Jul 19, 2023', action: 'Buy', quantity: '1,500.00', price: '$213.80', amount: '-$320,700.00', amountDir: 'negative' },
      { date: 'Jan 04, 2022', action: 'Buy', quantity: '2,000.00', price: '$221.05', amount: '-$442,100.00', amountDir: 'negative' },
    ],
    dividends: [
      { date: 'Jun 27, 2024', perShare: '$0.88', shares: '3,500.00', total: '$3,080.00' },
      { date: 'Mar 26, 2024', perShare: '$0.82', shares: '3,500.00', total: '$2,870.00' },
    ],
  },
  {
    symbol: 'BND',
    name: 'Vanguard Total Bond',
    assetClass: 'Fixed Income',
    price: '$72.45',
    quantity: '6,845.00',
    marketValue: '$495,920.25',
    change: '0.00%',
    changeDir: 'neutral',
    allocation: '19.98%',
    taxLots: [
      { dateAcquired: 'Feb 11, 2022', quantity: '6,845.00', costBasis: '$78.90', currentValue: '$495,920.25' },
    ],
    transactions: [
      { date: 'Feb 11, 2022', action: 'Buy', quantity: '6,845.00', price: '$78.90', amount: '-$540,070.50', amountDir: 'negative' },
    ],
    dividends: [
      { date: 'Jul 01, 2024', perShare: '$0.19', shares: '6,845.00', total: '$1,300.55' },
      { date: 'Jun 03, 2024', perShare: '$0.19', shares: '6,845.00', total: '$1,300.55' },
    ],
  },
]

const subTabs = ['Tax Lots', 'Transactions', 'Dividends'] as const
type SubTab = (typeof subTabs)[number]

function changeChip(change: string, dir: ChangeDirection) {
  const base = 'inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-bold tabular-nums'
  if (dir === 'positive') return <span className={`${base} bg-secondary-container text-on-secondary-container`}>{change}</span>
  if (dir === 'negative') return <span className={`${base} bg-error-container text-on-error-container`}>{change}</span>
  return <span className={`${base} bg-surface-container text-on-surface-variant`}>{change}</span>
}

const addLabels: Record<SubTab, string> = {
  'Tax Lots': 'Add Lot',
  Transactions: 'Add Transaction',
  Dividends: 'Add Dividend',
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

export default function Portfolio() {
  const [mounted, setMounted] = useState(false)
  const [showToast, setShowToast] = useState(false)
  const [sortBy, setSortBy] = useState('Market Value High to Low')
  const [expandedIndex, setExpandedIndex] = useState<number | null>(0)
  const [activeTab, setActiveTab] = useState<SubTab>('Tax Lots')

  useEffect(() => {
    const t1 = setTimeout(() => setMounted(true), 100)
    const t2 = setTimeout(() => setShowToast(true), 2000)
    const t3 = setTimeout(() => setShowToast(false), 6000)
    return () => {
      clearTimeout(t1)
      clearTimeout(t2)
      clearTimeout(t3)
    }
  }, [])

  function toggleRow(i: number) {
    if (expandedIndex === i) {
      setExpandedIndex(null)
    } else {
      setExpandedIndex(i)
      setActiveTab('Tax Lots')
    }
  }

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
              <button className="flex items-center gap-1.5 px-4 py-1.5 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity">
                <span className="material-symbols-outlined text-lg">add</span>
                Add Holding
              </button>
            </div>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-outline-variant">
                  <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Symbol</th>
                  <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Price</th>
                  <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Quantity</th>
                  <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Market Value</th>
                  <th className="text-center px-6 py-3 text-label-caps text-on-surface-variant uppercase">24h Change</th>
                  <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Allocation</th>
                </tr>
              </thead>
              <tbody>
                {holdings.map((h, i) => {
                  const expanded = expandedIndex === i
                  return (
                    <Fragment key={h.symbol}>
                      <tr
                        onClick={() => toggleRow(i)}
                        className="group border-b border-outline-variant hover:bg-surface-container-low transition-colors cursor-pointer"
                      >
                        <td className="px-6 py-4">
                          <div className="flex items-center gap-3">
                            <span
                              className={`material-symbols-outlined text-lg text-on-surface-variant transition-transform group-hover:text-primary ${
                                expanded ? 'rotate-90' : ''
                              }`}
                            >
                              chevron_right
                            </span>
                            <div className="w-8 h-8 rounded bg-primary-container flex items-center justify-center text-on-primary font-bold text-[10px]">
                              {h.symbol}
                            </div>
                            <div>
                              <p className="text-body-md font-semibold text-on-surface">{h.name}</p>
                              <p className="text-label-sm text-on-surface-variant">{h.assetClass}</p>
                            </div>
                          </div>
                        </td>
                        <td className="px-6 py-4 text-right text-data-tabular text-on-surface tabular-nums">{h.price}</td>
                        <td className="px-6 py-4 text-right text-data-tabular text-on-surface tabular-nums">{h.quantity}</td>
                        <td className="px-6 py-4 text-right text-data-tabular font-semibold text-on-surface tabular-nums">{h.marketValue}</td>
                        <td className="px-6 py-4 text-center">{changeChip(h.change, h.changeDir)}</td>
                        <td className="px-6 py-4 text-right text-data-tabular text-on-surface-variant tabular-nums">
                          <span className="inline-flex items-center gap-2">
                            {h.allocation}
                            <button
                              onClick={(e) => e.stopPropagation()}
                              className="text-on-surface-variant hover:text-primary transition-colors"
                              aria-label="More actions"
                            >
                              <span className="material-symbols-outlined text-lg align-middle">more_vert</span>
                            </button>
                          </span>
                        </td>
                      </tr>
                      {expanded && (
                        <tr className="bg-surface-container-lowest border-b border-outline-variant">
                          <td className="p-0" colSpan={6}>
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
          <div className="px-6 py-4 border-t border-outline-variant flex justify-center">
            <button className="flex items-center gap-2 py-2 text-body-md text-secondary font-semibold hover:opacity-80 transition-opacity">
              View all 24 holdings
              <span className="material-symbols-outlined text-base">arrow_forward</span>
            </button>
          </div>
        </div>
      </div>

      {/* Toast notification */}
      <div
        className={`fixed bottom-6 right-6 z-50 transition-all duration-500 ${
          showToast ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-4 pointer-events-none'
        }`}
      >
        <div className="flex items-center gap-3 bg-inverse-surface text-inverse-on-surface px-4 py-3 rounded-xl shadow-lg">
          <span className="material-symbols-outlined text-secondary text-xl">check_circle</span>
          <span className="text-body-md">Market data updated in real-time</span>
        </div>
      </div>
    </>
  )
}
