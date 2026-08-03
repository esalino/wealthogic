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
}

export interface Holding {
  id: string
  asset_type: string
  symbol: string
  description: string
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
  // Sub-records are not returned by the list endpoint yet; lazy-loaded later.
  tax_lots?: TaxLot[]
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
