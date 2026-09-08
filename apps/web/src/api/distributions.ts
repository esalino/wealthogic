const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

export interface Distribution {
  id: string
  category: string // 'dividend' | 'interest'
  holding_id: string | null
  account_id: string
  symbol: string
  asset_type: string | null
  payment_date: string
  amount: number
  transaction_id: string | null
  created_at: string
  updated_at: string
}

export interface DistributionSummary {
  total: number
  dividend: number
  interest: number
}

export interface PaginatedDistributions {
  data: Distribution[]
  total: number
  page: number
  page_size: number
  summary: DistributionSummary
}

export async function getDistributions(
  holdingId?: string,
  year?: number,
  page = 1,
  pageSize = 100,
): Promise<PaginatedDistributions> {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
  if (holdingId) params.set('holding_id', holdingId)
  if (year) params.set('year', String(year))
  const res = await fetch(`${API_BASE}/distributions?${params}`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch distributions')
  }

  return res.json()
}
