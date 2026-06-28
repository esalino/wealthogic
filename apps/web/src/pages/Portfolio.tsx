import { useState, useEffect } from 'react'

type ChangeDirection = 'positive' | 'negative' | 'neutral'

interface Holding {
  symbol: string
  name: string
  price: string
  quantity: string
  marketValue: string
  change: string
  changeDir: ChangeDirection
  allocation: string
}

const holdings: Holding[] = [
  {
    symbol: 'AAPL',
    name: 'Apple Inc.',
    price: '$189.43',
    quantity: '1,240.00',
    marketValue: '$234,893.20',
    change: '+1.24%',
    changeDir: 'positive',
    allocation: '9.46%',
  },
  {
    symbol: 'MSFT',
    name: 'Microsoft Corp.',
    price: '$415.10',
    quantity: '480.00',
    marketValue: '$199,248.00',
    change: '-0.82%',
    changeDir: 'negative',
    allocation: '8.02%',
  },
  {
    symbol: 'VTI',
    name: 'Vanguard Total Stock',
    price: '$252.18',
    quantity: '3,500.00',
    marketValue: '$882,630.00',
    change: '+0.45%',
    changeDir: 'positive',
    allocation: '35.56%',
  },
  {
    symbol: 'BND',
    name: 'Vanguard Total Bond',
    price: '$72.45',
    quantity: '6,845.00',
    marketValue: '$495,920.25',
    change: '0.00%',
    changeDir: 'neutral',
    allocation: '19.98%',
  },
]

function changeChip(change: string, dir: ChangeDirection) {
  const base = 'inline-flex items-center px-2 py-0.5 rounded-full text-label-sm font-semibold tabular-nums'
  if (dir === 'positive') return <span className={`${base} bg-secondary-container text-on-secondary-container`}>{change}</span>
  if (dir === 'negative') return <span className={`${base} bg-error-container text-on-error-container`}>{change}</span>
  return <span className={`${base} bg-surface-container-high text-on-surface-variant`}>{change}</span>
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
            <h2 className="text-headline-sm text-on-surface">Holdings</h2>
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
                {holdings.map((h, i) => (
                  <tr key={i} className="border-b border-outline-variant last:border-0 hover:bg-surface-container-low transition-colors">
                    <td className="px-6 py-4">
                      <div>
                        <p className="text-body-md font-semibold text-on-surface">{h.symbol}</p>
                        <p className="text-label-sm text-on-surface-variant">{h.name}</p>
                      </div>
                    </td>
                    <td className="px-6 py-4 text-right text-data-tabular text-on-surface tabular-nums">{h.price}</td>
                    <td className="px-6 py-4 text-right text-data-tabular text-on-surface tabular-nums">{h.quantity}</td>
                    <td className="px-6 py-4 text-right text-data-tabular font-semibold text-on-surface tabular-nums">{h.marketValue}</td>
                    <td className="px-6 py-4 text-center">{changeChip(h.change, h.changeDir)}</td>
                    <td className="px-6 py-4 text-right text-data-tabular text-on-surface-variant tabular-nums">{h.allocation}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="px-6 py-4 border-t border-outline-variant">
            <button className="w-full py-2 text-body-md text-secondary font-semibold hover:bg-surface-container-low rounded-lg transition-colors">
              View all 24 holdings
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
