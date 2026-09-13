const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

// How one jurisdiction taxes a realized event, recorded when it was realized.
export interface TaxTreatment {
  jurisdiction_code: string
  taxable: boolean
  taxable_amount: number
  excluded_amount: number
  character: string
  reason: string
}

export interface Gain {
  id: string
  category: string
  holding_id: string | null
  account_id: string
  symbol: string
  asset_type: string
  transaction_id: string
  lot_transaction_id: string | null
  acquired_date: string
  realized_date: string
  quantity: number
  cost_basis: number
  proceeds: number
  term: string // 'short' | 'long'
  amount: number
  // Per-jurisdiction treatment of this gain.
  treatments?: TaxTreatment[]
  created_at: string
  updated_at: string
}

export interface GainSummary {
  total: number
  short_term: number
  long_term: number
}

export interface PaginatedGains {
  data: Gain[]
  total: number
  page: number
  page_size: number
  summary: GainSummary
}

export async function getGains(year: number, page = 1, pageSize = 20): Promise<PaginatedGains> {
  const params = new URLSearchParams({ year: String(year), page: String(page), page_size: String(pageSize) })
  const res = await fetch(`${API_BASE}/gains?${params}`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch gains')
  }

  return res.json()
}
