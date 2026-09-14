const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

export interface TaxJurisdiction {
  code: string
  name: string
  level: string // 'national' | 'regional'
  parent_code: string | null
  currency_code: string
}

// One taxable bucket within a jurisdiction. The label comes from the API so a
// jurisdiction added as data arrives readable, without a client-side mapping to
// keep in sync.
export interface TaxBucket {
  character: string
  label: string
  amount: number
}

// Income a jurisdiction did not tax, and why.
export interface TaxExclusion {
  reason: string
  label: string
  amount: number
}

export interface JurisdictionSummary {
  code: string
  name: string
  level: string
  taxable_total: number
  buckets: TaxBucket[]
  excluded: TaxExclusion[]
}

// The year's taxable income per jurisdiction. Jurisdictions differ in shape -
// one may split capital gains by holding period while another folds everything
// into ordinary income - so the client renders whatever buckets it's given
// rather than assuming a fixed set.
export interface TaxSummary {
  year: number
  jurisdictions: JurisdictionSummary[]
}

export async function getTaxSummary(year: number): Promise<TaxSummary> {
  const res = await fetch(`${API_BASE}/tax/summary?year=${year}`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch tax summary')
  }

  return res.json()
}

export async function getJurisdictions(): Promise<TaxJurisdiction[]> {
  const res = await fetch(`${API_BASE}/tax/jurisdictions`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch jurisdictions')
  }

  return res.json()
}

// How one jurisdiction taxes a realized event, recorded when it was realized.
export interface TaxTreatment {
  jurisdiction_code: string
  taxable: boolean
  taxable_amount: number
  excluded_amount: number
  character: string
  reason: string
}

// One realized taxable event. Disposals and income share a single ledger, so
// this is one table's row rather than a union of two - the lot fields are set
// only for a disposal and null for income, which has no lot behind it.
export interface RealizedEvent {
  id: string
  category: string // 'capital_gain' | 'interest' | 'dividend'
  origin: string // 'disposal' | 'distribution'
  symbol: string
  asset_type: string
  event_date: string
  amount: number
  // Source records this was derived from; which are set follows the origin.
  transaction_id: string | null
  lot_transaction_id: string | null
  distribution_id: string | null
  // Lot detail, disposals only.
  acquired_date: string | null
  term: string
  quantity: number | null
  cost_basis: number | null
  proceeds: number | null
  holding_id: string | null
  account_id: string
  treatments: TaxTreatment[]
}

// Filters accepted by the realized-events list. They compose, and each is
// optional - a plain query over one table now that the ledgers are merged.
export interface RealizedEventFilters {
  category?: string
  origin?: string
  symbol?: string
  holdingId?: string
  accountId?: string
}

// Totals by what was realized. How it's taxed differs per jurisdiction and
// comes from getTaxSummary instead.
export interface RealizedEventSummary {
  total: number
  capital_gains: number
  interest: number
  dividends: number
}

export interface PaginatedRealizedEvents {
  data: RealizedEvent[]
  total: number
  page: number
  page_size: number
  summary: RealizedEventSummary
}

export async function getRealizedEvents(
  year: number,
  page = 1,
  pageSize = 20,
  filters: RealizedEventFilters = {},
): Promise<PaginatedRealizedEvents> {
  const params = new URLSearchParams({ year: String(year), page: String(page), page_size: String(pageSize) })
  if (filters.category) params.set('category', filters.category)
  if (filters.origin) params.set('origin', filters.origin)
  if (filters.symbol) params.set('symbol', filters.symbol)
  if (filters.holdingId) params.set('holding_id', filters.holdingId)
  if (filters.accountId) params.set('account_id', filters.accountId)

  const res = await fetch(`${API_BASE}/tax/events?${params}`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch realized events')
  }

  return res.json()
}
