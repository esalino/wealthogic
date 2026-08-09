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

export async function getTransactions(page = 1, pageSize = 20): Promise<PaginatedTransactions> {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
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
