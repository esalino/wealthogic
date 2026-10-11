import type { Transaction } from './transactions'
import type { Strategy } from './strategies'

const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

export const EXIT_REASONS: { value: string; label: string }[] = [
  { value: 'target', label: 'Target' },
  { value: 'stop', label: 'Stop' },
  { value: 'trailing_stop', label: 'Trailing stop' },
  { value: 'time', label: 'Time' },
  { value: 'rule', label: 'Exit rule' },
  { value: 'discretionary', label: 'Discretionary' },
  { value: 'expired', label: 'Expired' },
  { value: 'assigned', label: 'Assigned' },
]

// A transaction, or part of one, assigned to a trade. Whether it's an entry or
// an exit is the transaction's own effect. Quantity is in the transaction's own
// shares. An 'auto' fill was attached because it closed shares the trade had
// open; a detached one was taken off by the user and counts for nothing.
export interface TradeFill {
  id: string
  trade_id: string
  transaction_id: string
  transaction: Transaction | null
  quantity: number
  source: 'manual' | 'auto'
  detached: boolean
  exit_reason: string
}

export interface TradeRuleCheck {
  id: string
  trade_id: string
  kind: 'entry' | 'exit' | 'risk'
  text: string
  sort_order: number
  met: boolean | null
}

// A journaled position. The figures are realized only, except unrealized_pnl,
// which values what's still open at the holding's last price.
export interface Trade {
  id: string
  account_id: string
  underlying: string
  structure: string
  direction: 'long' | 'short'
  status: 'open' | 'closed'
  opened_on: string | null
  closed_on: string | null
  strategy_id: string | null
  strategy?: Strategy | null
  initial_stop: number | null
  current_stop: number | null
  target: number | null
  planned_risk: number | null
  notes: string
  rating: number | null
  open_quantity: number
  avg_entry_price: number
  avg_exit_price: number
  entry_value: number
  gross_pnl: number
  fees: number
  net_pnl: number
  return_percent: number | null
  r_multiple: number | null
  holding_days: number | null
  unrealized_pnl: number | null
  fills: TradeFill[]
  rule_checks: TradeRuleCheck[]
  created_at: string
  updated_at: string
}

export interface TradeFilters {
  status?: 'open' | 'closed'
  strategyId?: string
  accountId?: string
  holdingId?: string
}

export interface CreateTradePayload {
  transaction_id: string
  quantity?: number | null
  strategy_id?: string | null
  initial_stop?: number | null
  target?: number | null
  planned_risk?: number | null
  notes?: string
  met_rule_ids?: string[]
}

// The whole plan: fields left out are cleared. planned_risk left null is
// derived from the initial stop.
export interface UpdateTradePayload {
  strategy_id: string | null
  structure: string
  initial_stop: number | null
  current_stop: number | null
  target: number | null
  planned_risk: number | null
  notes: string
  rating: number | null
}

export interface TradeCandidate {
  transaction: Transaction
  available_quantity: number
  detached: boolean
}

async function check<T>(res: Response, fallback: string): Promise<T> {
  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? fallback)
  }
  return res.status === 204 ? (undefined as T) : res.json()
}

function send(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

export async function getTrades(filters: TradeFilters = {}): Promise<Trade[]> {
  const params = new URLSearchParams()
  if (filters.status) params.set('status', filters.status)
  if (filters.strategyId) params.set('strategy_id', filters.strategyId)
  if (filters.accountId) params.set('account_id', filters.accountId)
  if (filters.holdingId) params.set('holding_id', filters.holdingId)
  return check(await fetch(`${API_BASE}/trades?${params}`), 'Failed to fetch trades')
}

export async function getTrade(id: string): Promise<Trade> {
  return check(await fetch(`${API_BASE}/trades/${id}`), 'Failed to fetch trade')
}

export async function createTrade(payload: CreateTradePayload): Promise<Trade> {
  return check(await fetch(`${API_BASE}/trades`, send('POST', payload)), 'Failed to start trade')
}

export async function updateTrade(id: string, payload: UpdateTradePayload): Promise<Trade> {
  return check(await fetch(`${API_BASE}/trades/${id}`, send('PATCH', payload)), 'Failed to update trade')
}

export async function deleteTrade(id: string): Promise<void> {
  return check(await fetch(`${API_BASE}/trades/${id}`, { method: 'DELETE' }), 'Failed to delete trade')
}

export async function getTradeCandidates(id: string): Promise<TradeCandidate[]> {
  return check(await fetch(`${API_BASE}/trades/${id}/candidates`), 'Failed to fetch transactions')
}

// Leave quantity out on an exit to have it sized to what the trade had open.
export async function addTradeFill(id: string, transactionId: string, quantity?: number): Promise<Trade> {
  return check(await fetch(`${API_BASE}/trades/${id}/fills`, send('POST', { transaction_id: transactionId, quantity })), 'Failed to add fill')
}

export async function updateTradeFill(id: string, fillId: string, payload: { exit_reason: string; quantity?: number }): Promise<Trade> {
  return check(await fetch(`${API_BASE}/trades/${id}/fills/${fillId}`, send('PATCH', payload)), 'Failed to update fill')
}

export async function removeTradeFill(id: string, fillId: string): Promise<Trade> {
  return check(await fetch(`${API_BASE}/trades/${id}/fills/${fillId}`, { method: 'DELETE' }), 'Failed to remove fill')
}

export async function updateRuleChecks(id: string, checks: { id: string; met: boolean | null }[]): Promise<Trade> {
  return check(await fetch(`${API_BASE}/trades/${id}/rule-checks`, send('PUT', checks)), 'Failed to update checklist')
}
