// Admin utilities: one-off maintenance chores run by hand from the Admin page,
// rather than anything the app does on its own. They all act on the whole book,
// so none of them take arguments.
const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

async function post<T>(path: string, failure: string): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, { method: 'POST' })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? failure)
  }

  return res.json()
}

// What a price refresh managed for one symbol.
export type QuoteOutcome = 'priced' | 'not_found' | 'failed'

export interface QuotedSymbol {
  symbol: string
  outcome: QuoteOutcome
  // Set only for a symbol that was priced.
  price: number
  // What the price is as of, which is not when we asked: a quote pulled over a
  // weekend is Friday's close.
  as_of: string | null
  previous_price: number
}

export interface PriceRefreshResult {
  considered: number
  priced: number
  not_found: number
  failed: number
  fetched_at: string
  symbols: QuotedSymbol[]
}

// Re-quotes every open stock position. Requests are paced server-side to stay
// inside the provider's rate limit, so a large book takes a while.
export function refreshPrices(): Promise<PriceRefreshResult> {
  return post<PriceRefreshResult>('/holdings/refresh-prices', 'Failed to refresh prices')
}

export interface BackfillProfilesResult {
  considered: number
  enriched: number
  not_found: number
  failed: number
}

// Fills in sector, industry and company name for holdings that have none.
export function backfillProfiles(): Promise<BackfillProfilesResult> {
  return post<BackfillProfilesResult>('/holdings/backfill-profiles', 'Failed to backfill profiles')
}

export interface RecalculateResult {
  holdings: number
  open: number
  closed: number
}

// Replays the position derivation over every holding, for holdings last written
// before the derivation changed.
export function recalculateHoldings(): Promise<RecalculateResult> {
  return post<RecalculateResult>('/holdings/recalculate', 'Failed to recalculate holdings')
}
