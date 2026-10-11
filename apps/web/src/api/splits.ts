const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

// A split of a holding's shares: every old_shares held before effective_date
// became new_shares. A 2-for-1 is 1 -> 2; a 1-for-8 reverse split is 8 -> 1.
// Recording one resizes the holding's tax lots; its transactions stay as traded.
export interface StockSplit {
  id: string
  holding_id: string
  effective_date: string
  old_shares: number
  new_shares: number
  created_at: string
  updated_at: string
}

export interface CreateSplitPayload {
  holding_id: string
  effective_date: string
  old_shares: number
  new_shares: number
}

export async function getSplits(holdingId: string): Promise<StockSplit[]> {
  const res = await fetch(`${API_BASE}/splits?holding_id=${holdingId}`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch splits')
  }

  return res.json()
}

export async function createSplit(payload: CreateSplitPayload): Promise<StockSplit> {
  const res = await fetch(`${API_BASE}/splits`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to record split')
  }

  return res.json()
}

export async function deleteSplit(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/splits/${id}`, { method: 'DELETE' })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to delete split')
  }
}

// "2-for-1", "1-for-8": new shares for old.
export function splitLabel(split: Pick<StockSplit, 'old_shares' | 'new_shares'>): string {
  return `${split.new_shares}-for-${split.old_shares}`
}
