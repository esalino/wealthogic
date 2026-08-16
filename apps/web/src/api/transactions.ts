const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

export interface Transaction {
  id: string
  account_id: string
  holding_id: string | null
  asset_type: string | null
  symbol: string
  asset_description: string | null
  action: string
  date: string
  quantity: number | null
  price: number | null
  amount: number
  commission: number
  fees: number
  settlement_date: string | null
  realized_gains: number
  created_at: string
  updated_at: string
}

export interface PaginatedTransactions {
  data: Transaction[]
  total: number
  page: number
  page_size: number
}

export async function getTransactions(holdingId?: string, page = 1, pageSize = 100): Promise<PaginatedTransactions> {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
  if (holdingId) params.set('holding_id', holdingId)
  const res = await fetch(`${API_BASE}/transactions?${params}`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch transactions')
  }

  return res.json()
}

export interface CreateTransactionPayload {
  account_id: string
  holding_id?: string | null
  asset_type?: string | null
  symbol?: string
  asset_description?: string | null
  action: string
  date: string
  quantity?: number | null
  price?: number | null
  amount: number
  commission?: number
  fees?: number
  settlement_date?: string | null
  realized_gains?: number
}

export async function createTransaction(payload: CreateTransactionPayload): Promise<Transaction> {
  const res = await fetch(`${API_BASE}/transactions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to create transaction')
  }

  return res.json()
}

export interface UpdateTransactionPayload {
  account_id: string
  action: string
  date: string
  quantity?: number | null
  price?: number | null
  amount: number
  commission?: number
  fees?: number
}

export async function updateTransaction(id: string, payload: UpdateTransactionPayload): Promise<Transaction> {
  const res = await fetch(`${API_BASE}/transactions/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to update transaction')
  }

  return res.json()
}

export async function deleteTransaction(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/transactions/${id}`, { method: 'DELETE' })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to delete transaction')
  }
}
