import type { TaxLot } from './holdings'
import type { Transaction } from './transactions'

const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

export interface CreateTaxLotPayload {
  holding_id: string
  account_id: string
  purchase_date: string
  purchase_quantity: number
  purchase_price: number
  commission?: number
  fees?: number
}

// Adding a lot also records the buy transaction that produced it; the endpoint
// returns both.
export interface TaxLotWithTransaction {
  tax_lot: TaxLot
  transaction: Transaction
}

export interface PaginatedTaxLots {
  data: TaxLot[]
  total: number
  page: number
  page_size: number
}

export async function getTaxLots(holdingId?: string, page = 1, pageSize = 100): Promise<PaginatedTaxLots> {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
  if (holdingId) params.set('holding_id', holdingId)
  const res = await fetch(`${API_BASE}/tax-lots?${params}`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch tax lots')
  }

  return res.json()
}

export async function createTaxLot(payload: CreateTaxLotPayload): Promise<TaxLotWithTransaction> {
  const res = await fetch(`${API_BASE}/tax-lots`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to add tax lot')
  }

  return res.json()
}

export interface UpdateTaxLotPayload {
  account_id: string
  purchase_date: string
  purchase_quantity: number
  purchase_price: number
}

export async function updateTaxLot(id: string, payload: UpdateTaxLotPayload): Promise<TaxLot> {
  const res = await fetch(`${API_BASE}/tax-lots/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to update tax lot')
  }

  return res.json()
}
