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

// Income totals carry no tax-bucket split: how income divides into qualified,
// ordinary, and exempt depends on the jurisdiction asking, so that breakdown
// comes from getTaxSummary instead, per jurisdiction.
export interface DistributionSummary {
  total: number
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

// Creating or updating an income payment by hand. Uploads build the same
// record; this is the other way in. The category is what the payment is —
// 'dividend' from a security, 'interest' from a cash or savings balance.
export interface DistributionPayload {
  account_id: string
  holding_id?: string | null
  category: string
  symbol: string
  asset_type?: string | null
  payment_date: string
  amount: number
}

export async function createDistribution(payload: DistributionPayload): Promise<Distribution> {
  const res = await fetch(`${API_BASE}/distributions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to create distribution')
  }

  return res.json()
}

export async function updateDistribution(id: string, payload: DistributionPayload): Promise<Distribution> {
  const res = await fetch(`${API_BASE}/distributions/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to update distribution')
  }

  return res.json()
}

export async function deleteDistribution(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/distributions/${id}`, { method: 'DELETE' })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to delete distribution')
  }
}
