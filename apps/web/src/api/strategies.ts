const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

export type RuleKind = 'entry' | 'exit' | 'risk'

export const RULE_KINDS: { value: RuleKind; label: string }[] = [
  { value: 'entry', label: 'Entry' },
  { value: 'exit', label: 'Exit' },
  { value: 'risk', label: 'Risk' },
]

export interface StrategyRule {
  id: string
  strategy_id: string
  kind: RuleKind
  text: string
  sort_order: number
}

// A named way of trading, written down as a checklist. A trade copies the rules
// when it starts, so editing them never changes how past trades were judged.
export interface Strategy {
  id: string
  name: string
  description: string
  status: 'active' | 'retired'
  direction: '' | 'long' | 'short' | 'both'
  timeframe: string
  rules: StrategyRule[]
  created_at: string
  updated_at: string
}

export interface StrategyPayload {
  name: string
  description: string
  status: 'active' | 'retired'
  direction: string
  timeframe: string
  rules: { kind: RuleKind; text: string }[]
}

// How a set of closed trades performed. Ratios that would divide by zero come
// back null - "no losses yet" isn't a profit factor of zero.
export interface TradeStats {
  strategy_id: string | null
  strategy_name: string
  trades: number
  wins: number
  losses: number
  win_rate: number | null
  net_pnl: number
  avg_win: number | null
  avg_loss: number | null
  largest_win: number
  largest_loss: number
  profit_factor: number | null
  expectancy: number | null
  avg_r: number | null
  r_trades: number
  avg_holding_days: number | null
  max_drawdown: number
  rule_adherence: number | null
}

export interface StrategyPerformance {
  strategies: TradeStats[]
  total: TradeStats
}

async function check<T>(res: Response, fallback: string): Promise<T> {
  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? fallback)
  }
  return res.status === 204 ? (undefined as T) : res.json()
}

export async function getStrategies(status?: 'active' | 'retired'): Promise<Strategy[]> {
  const params = status ? `?status=${status}` : ''
  return check(await fetch(`${API_BASE}/strategies${params}`), 'Failed to fetch strategies')
}

export async function createStrategy(payload: StrategyPayload): Promise<Strategy> {
  return check(await fetch(`${API_BASE}/strategies`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  }), 'Failed to create strategy')
}

export async function updateStrategy(id: string, payload: StrategyPayload): Promise<Strategy> {
  return check(await fetch(`${API_BASE}/strategies/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  }), 'Failed to update strategy')
}

export async function deleteStrategy(id: string): Promise<void> {
  return check(await fetch(`${API_BASE}/strategies/${id}`, { method: 'DELETE' }), 'Failed to delete strategy')
}

export async function getStrategyPerformance(accountId?: string): Promise<StrategyPerformance> {
  const params = accountId ? `?account_id=${accountId}` : ''
  return check(await fetch(`${API_BASE}/strategies/performance${params}`), 'Failed to fetch performance')
}
