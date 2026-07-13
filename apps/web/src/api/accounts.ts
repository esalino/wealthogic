const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

export interface CreateAccountPayload {
  account_name: string
  account_type: string
  tax_type: string
  balance: number
  owner_ids: string[]
}

export interface Account {
  id: string
  account_name: string
  account_type: string
  tax_type: string
  balance: number
  created_at: string
  updated_at: string
  owners?: { id: string; first_name: string; last_name: string }[]
}

export interface PaginatedAccounts {
  data: Account[]
  total: number
  page: number
  page_size: number
}

export async function getAccounts(page = 1, pageSize = 20): Promise<PaginatedAccounts> {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
  const res = await fetch(`${API_BASE}/accounts?${params}`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch accounts')
  }

  return res.json()
}

export interface UpdateAccountPayload {
  account_name: string
  account_type: string
  tax_type: string
  balance: number
  owner_ids: string[]
}

export async function updateAccount(id: string, payload: UpdateAccountPayload): Promise<Account> {
  const res = await fetch(`${API_BASE}/accounts/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to update account')
  }

  return res.json()
}

export async function createAccount(payload: CreateAccountPayload): Promise<Account> {
  const res = await fetch(`${API_BASE}/accounts`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to create account')
  }

  return res.json()
}
