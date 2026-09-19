const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

export interface TaxLot {
  id: string
  asset_type: string
  symbol: string
  asset_description: string
  purchase_date: string
  purchase_quantity: number
  purchase_price: number
  remaining_quantity: number
  holding_id: string | null
  account_id: string
  created_at: string
  updated_at: string

  // 'long' | 'short'. A written option's lot took premium in rather than paying
  // it out, so its cost basis and value carry the opposite sign.
  direction: string
  // Shares one unit covers: 100 for an option contract, 1 otherwise.
  contract_multiplier: number
  // From the holding, which is the one place a current price lives.
  last_price: number
  // The open part of the lot at its opening price, fees included.
  cost_basis: number
  // Null when the holding has no price - an unpriced lot is unknown, not zero.
  market_value: number | null
  gain_unrealized_amount: number | null
  gain_unrealized_percent: number | null
}

export interface Holding {
  id: string
  asset_type: string
  symbol: string
  description: string

  // Reference data from a market-data provider. Descriptive only, and empty
  // when the provider has no record of the symbol (a Treasury CUSIP, an option
  // contract) or when no API key is configured.
  company_name: string
  sector: string
  industry: string
  exchange: string
  country: string
  website: string
  logo_url: string
  profile_fetched_at: string | null
  status: string
  last_price: number
  purchase_quantity: number
  current_value: number
  average_cost_basis: number
  cost_basis_total: number
  gain_unrealized_percent: number
  gain_unrealized_amount: number
  gain_realized_percent: number
  gain_realized_amount: number
  dividend_income: number
  // Tax-class override: null = auto (derive from asset type). Describes what the
  // asset pays, not how any jurisdiction taxes it.
  tax_class_override: string | null
  // Which government issued the asset's debt, for the bond classes.
  issuer_jurisdiction: string | null
  // Resolved class (override, else asset-type default). Read-only.
  tax_class: string
  // The allocation slice the tax class implies: Equity, Fixed Income, Cash,
  // Derivatives, Other. Read-only.
  asset_class: string
  created_at: string
  updated_at: string
}

export interface PaginatedHoldings {
  data: Holding[]
  total: number
  page: number
  page_size: number
}

export async function getHoldings(page = 1, pageSize = 20): Promise<PaginatedHoldings> {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
  const res = await fetch(`${API_BASE}/holdings?${params}`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch holdings')
  }

  return res.json()
}

export interface CreateHoldingPayload {
  asset_type: string
  symbol: string
  description: string
  status?: string
  last_price: number
  purchase_quantity: number
  current_value: number
  average_cost_basis: number
  cost_basis_total: number
  dividend_income: number
  tax_class_override?: string | null
  issuer_jurisdiction?: string | null
}

export async function createHolding(payload: CreateHoldingPayload): Promise<Holding> {
  const res = await fetch(`${API_BASE}/holdings`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to create holding')
  }

  return res.json()
}

export type UpdateHoldingPayload = CreateHoldingPayload

export async function updateHolding(id: string, payload: UpdateHoldingPayload): Promise<Holding> {
  const res = await fetch(`${API_BASE}/holdings/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to update holding')
  }

  return res.json()
}

// One wedge of an allocation breakdown.
export interface AllocationSlice {
  label: string
  value: number
  percent: number
}

// Computed over every holding, not a page of them: an allocation drawn from
// whatever rows a table happens to be showing describes the page rather than
// the portfolio.
export interface AllocationSummary {
  total_value: number
  asset_classes: AllocationSlice[]
  equity_value: number
  equity_percent: number
  // Weighted within equities, since a sector describes a company and means
  // nothing for cash or a government bond.
  equity_sectors: AllocationSlice[]
}

export async function getAllocation(): Promise<AllocationSummary> {
  const res = await fetch(`${API_BASE}/holdings/allocation`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch allocation')
  }

  return res.json()
}
