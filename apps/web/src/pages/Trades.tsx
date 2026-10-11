import { Fragment, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  EXIT_REASONS,
  addTradeFill,
  deleteTrade,
  getTradeCandidates,
  getTrades,
  removeTradeFill,
  updateRuleChecks,
  updateTrade,
  updateTradeFill,
  type Trade,
  type TradeFill,
} from '../api/trades'
import {
  RULE_KINDS,
  createStrategy,
  deleteStrategy,
  getStrategies,
  getStrategyPerformance,
  updateStrategy,
  type RuleKind,
  type Strategy,
  type StrategyPayload,
  type TradeStats,
} from '../api/strategies'
import { getAccounts } from '../api/accounts'
import { formatDate } from '../lib/datetime'

const tabs = ['Trades', 'Strategies', 'Performance'] as const
type Tab = (typeof tabs)[number]

// fieldCls is an input without a width, for fields sized by a flex row.
const fieldCls = 'px-3 py-2.5 bg-surface-container-low border border-outline-variant rounded-lg text-body-md text-on-surface placeholder:text-on-surface-variant focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors'
const inputCls = `w-full ${fieldCls}`
const numInputCls = `${inputCls} tabular-nums`
const filterCls = 'px-2 py-1 bg-surface-container-low border border-outline-variant rounded-lg text-label-sm text-on-surface focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors'
const thCls = 'px-4 py-3 text-label-caps text-on-surface-variant uppercase'

const fmtCurrency = (n: number) => (n ?? 0).toLocaleString('en-US', { style: 'currency', currency: 'USD' })
const fmtSigned = (n: number) => `${(n ?? 0) < 0 ? '-' : '+'}${fmtCurrency(Math.abs(n ?? 0))}`
const fmtNumber = (n: number) => (n ?? 0).toLocaleString('en-US', { maximumFractionDigits: 4 })
const fmtR = (r: number | null) => (r == null ? '—' : `${r >= 0 ? '+' : ''}${r.toFixed(2)}R`)
const fmtPct = (p: number | null) => (p == null ? '—' : `${p >= 0 ? '+' : ''}${p.toFixed(2)}%`)

function gainColor(n: number | null) {
  if (n == null || n === 0) return 'text-on-surface'
  return n < 0 ? 'text-error' : 'text-secondary'
}

function parseOptional(v: string): number | null {
  const n = parseFloat(v)
  return Number.isFinite(n) ? n : null
}

const optionalString = (n: number | null) => (n == null ? '' : String(n))

// The tabs touched by any change to a trade.
function useInvalidateTrades() {
  const queryClient = useQueryClient()
  return () => {
    queryClient.invalidateQueries({ queryKey: ['trades'] })
    queryClient.invalidateQueries({ queryKey: ['strategy-performance'] })
  }
}

function StatusPill({ status }: { status: string }) {
  const open = status === 'open'
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-bold uppercase ${
      open ? 'bg-secondary/10 text-secondary' : 'bg-surface-container-high text-on-surface-variant'
    }`}>
      {status}
    </span>
  )
}

// ── Trades tab ───────────────────────────────────────────────────────────────

function TradesTab({ focusId }: { focusId: string | null }) {
  const [status, setStatus] = useState<'' | 'open' | 'closed'>('')
  const [strategyId, setStrategyId] = useState('')
  const [expandedId, setExpandedId] = useState<string | null>(focusId)

  const { data: trades = [], isLoading, isError, error } = useQuery({
    queryKey: ['trades', status, strategyId],
    queryFn: () => getTrades({ status: status || undefined, strategyId: strategyId || undefined }),
  })
  const { data: strategies = [] } = useQuery({ queryKey: ['strategies'], queryFn: () => getStrategies() })
  const { data: accountsData } = useQuery({ queryKey: ['accounts', 'select'], queryFn: () => getAccounts(1, 100) })
  const accountName = (id: string) => accountsData?.data.find((a) => a.id === id)?.account_name ?? ''

  return (
    <div className="bg-surface-container-lowest rounded-xl shadow-card">
      <div className="flex items-center justify-between px-6 py-4 border-b border-outline-variant">
        <div>
          <h2 className="text-headline-sm text-on-surface">Trades</h2>
          <p className="text-label-sm text-on-surface-variant">Start a trade from a buy in Portfolio → Transactions or Tax Lots.</p>
        </div>
        <div className="flex items-center gap-2">
          <select value={status} onChange={(e) => setStatus(e.target.value as typeof status)} className={filterCls}>
            <option value="">All</option>
            <option value="open">Open</option>
            <option value="closed">Closed</option>
          </select>
          <select value={strategyId} onChange={(e) => setStrategyId(e.target.value)} className={filterCls}>
            <option value="">Any strategy</option>
            {strategies.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select>
        </div>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="border-b border-outline-variant">
              <th className={`${thCls} text-left`}>Symbol</th>
              <th className={`${thCls} text-left`}>Strategy</th>
              <th className={`${thCls} text-left`}>Opened</th>
              <th className={`${thCls} text-left`}>Closed</th>
              <th className={`${thCls} text-right`}>Avg Entry</th>
              <th className={`${thCls} text-right`}>Avg Exit</th>
              <th className={`${thCls} text-right`}>Net P&amp;L</th>
              <th className={`${thCls} text-right`}>Return</th>
              <th className={`${thCls} text-right`}>R</th>
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr><td colSpan={9} className="px-4 py-10 text-center text-body-md text-on-surface-variant">Loading trades…</td></tr>
            )}
            {isError && (
              <tr><td colSpan={9} className="px-4 py-10 text-center text-body-md text-error">{(error as Error).message}</td></tr>
            )}
            {!isLoading && !isError && trades.length === 0 && (
              <tr><td colSpan={9} className="px-4 py-10 text-center text-body-md text-on-surface-variant">No trades yet.</td></tr>
            )}
            {trades.map((t) => {
              const expanded = expandedId === t.id
              return (
                <Fragment key={t.id}>
                  <tr
                    onClick={() => setExpandedId(expanded ? null : t.id)}
                    className="group border-b border-outline-variant hover:bg-surface-container-low transition-colors cursor-pointer text-data-tabular tabular-nums text-on-surface"
                  >
                    <td className="px-4 py-4">
                      <div className="flex items-center gap-3">
                        <span className={`material-symbols-outlined text-lg text-on-surface-variant transition-transform group-hover:text-primary ${expanded ? 'rotate-90' : ''}`}>
                          chevron_right
                        </span>
                        <div>
                          <div className="flex items-center gap-2">
                            <p className="text-body-md font-semibold">{t.underlying}</p>
                            <StatusPill status={t.status} />
                            {t.direction === 'short' && <span className="text-[10px] font-bold uppercase text-on-surface-variant">Short</span>}
                          </div>
                          <p className="text-label-sm text-on-surface-variant">{accountName(t.account_id)}</p>
                        </div>
                      </div>
                    </td>
                    <td className="px-4 py-4 text-body-md">{t.strategy?.name ?? <span className="text-on-surface-variant">—</span>}</td>
                    <td className="px-4 py-4">{t.opened_on ? formatDate(t.opened_on) : '—'}</td>
                    <td className="px-4 py-4">
                      {t.closed_on ? formatDate(t.closed_on) : (
                        <span className="text-on-surface-variant">{fmtNumber(t.open_quantity)} open</span>
                      )}
                    </td>
                    <td className="px-4 py-4 text-right">{fmtCurrency(t.avg_entry_price)}</td>
                    <td className="px-4 py-4 text-right">{t.avg_exit_price ? fmtCurrency(t.avg_exit_price) : '—'}</td>
                    <td className={`px-4 py-4 text-right font-semibold ${gainColor(t.net_pnl)}`}>
                      {fmtSigned(t.net_pnl)}
                      {t.unrealized_pnl != null && (
                        <div className={`text-label-sm font-normal ${gainColor(t.unrealized_pnl)}`}>{fmtSigned(t.unrealized_pnl)} open</div>
                      )}
                    </td>
                    <td className={`px-4 py-4 text-right ${gainColor(t.return_percent)}`}>{fmtPct(t.return_percent)}</td>
                    <td className={`px-4 py-4 text-right font-semibold ${gainColor(t.r_multiple)}`}>{fmtR(t.r_multiple)}</td>
                  </tr>
                  {expanded && (
                    <tr className="border-b border-outline-variant">
                      <td colSpan={9} className="p-0">
                        <TradeDetail key={t.id} trade={t} strategies={strategies} onDeleted={() => setExpandedId(null)} />
                      </td>
                    </tr>
                  )}
                </Fragment>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function Stat({ label, value, cls }: { label: string; value: string; cls?: string }) {
  return (
    <div>
      <p className="text-label-caps text-on-surface-variant uppercase">{label}</p>
      <p className={`text-body-lg font-semibold tabular-nums ${cls ?? 'text-on-surface'}`}>{value}</p>
    </div>
  )
}

function TradeDetail({ trade, strategies, onDeleted }: { trade: Trade; strategies: Strategy[]; onDeleted: () => void }) {
  const invalidate = useInvalidateTrades()

  // The form is seeded from the trade when it's opened. Saving writes back the
  // planned risk the server derived, the one field it may have filled in.
  const [strategyId, setStrategyId] = useState(trade.strategy_id ?? '')
  const [initialStop, setInitialStop] = useState(optionalString(trade.initial_stop))
  const [currentStop, setCurrentStop] = useState(optionalString(trade.current_stop))
  const [target, setTarget] = useState(optionalString(trade.target))
  const [risk, setRisk] = useState(optionalString(trade.planned_risk))
  const [rating, setRating] = useState<number | null>(trade.rating)
  const [notes, setNotes] = useState(trade.notes)
  const [adding, setAdding] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const save = useMutation({
    mutationFn: () => updateTrade(trade.id, {
      strategy_id: strategyId || null,
      structure: trade.structure,
      initial_stop: parseOptional(initialStop),
      current_stop: parseOptional(currentStop),
      target: parseOptional(target),
      // A risk the user didn't change is re-derived from the stop, so moving
      // the stop moves the risk with it.
      planned_risk: risk === optionalString(trade.planned_risk) && initialStop !== optionalString(trade.initial_stop)
        ? null
        : parseOptional(risk),
      notes,
      rating,
    }),
    onSuccess: (saved) => {
      setRisk(optionalString(saved.planned_risk))
      invalidate()
    },
  })
  const remove = useMutation({ mutationFn: () => deleteTrade(trade.id), onSuccess: () => { invalidate(); onDeleted() } })
  const checks = useMutation({
    mutationFn: (c: { id: string; met: boolean | null }) => updateRuleChecks(trade.id, [c]),
    onSuccess: invalidate,
  })

  const strategyChanged = (strategyId || null) !== trade.strategy_id
  const allStrategies = trade.strategy && !strategies.some((s) => s.id === trade.strategy!.id)
    ? [...strategies, trade.strategy]
    : strategies

  return (
    <div className="p-6 space-y-6 bg-surface-container-lowest">
      <div className="grid grid-cols-2 md:grid-cols-6 gap-4">
        <Stat label="Net P&L" value={fmtSigned(trade.net_pnl)} cls={gainColor(trade.net_pnl)} />
        <Stat label="Unrealized" value={trade.unrealized_pnl == null ? '—' : fmtSigned(trade.unrealized_pnl)} cls={gainColor(trade.unrealized_pnl)} />
        <Stat label="R Multiple" value={fmtR(trade.r_multiple)} cls={gainColor(trade.r_multiple)} />
        <Stat label="Return" value={fmtPct(trade.return_percent)} cls={gainColor(trade.return_percent)} />
        <Stat label="Fees" value={fmtCurrency(trade.fees)} />
        <Stat label="Held" value={trade.holding_days == null ? '—' : `${trade.holding_days}d`} />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Plan */}
        <div className="space-y-3">
          <h3 className="text-label-caps text-on-surface-variant uppercase">Plan</h3>
          <div>
            <label className="block text-label-sm font-semibold text-on-surface mb-1">Strategy</label>
            <select value={strategyId} onChange={(e) => setStrategyId(e.target.value)} className={inputCls}>
              <option value="">No strategy</option>
              {allStrategies.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
            {strategyChanged && trade.rule_checks.length > 0 && (
              <p className="mt-1 text-label-sm text-on-surface-variant">Saving replaces this trade's checklist with the new strategy's.</p>
            )}
          </div>
          <div className="grid grid-cols-2 gap-3">
            {([
              ['Initial stop', initialStop, setInitialStop],
              ['Current stop', currentStop, setCurrentStop],
              ['Target', target, setTarget],
              ['Planned risk', risk, setRisk],
            ] as const).map(([label, value, set]) => (
              <div key={label}>
                <label className="block text-label-sm font-semibold text-on-surface mb-1">{label}</label>
                <div className="relative">
                  <span className="absolute left-3 top-1/2 -translate-y-1/2 text-body-md text-on-surface-variant">$</span>
                  <input type="number" value={value} onChange={(e) => set(e.target.value)} className={`${numInputCls} pl-7`} />
                </div>
              </div>
            ))}
          </div>
          <div>
            <label className="block text-label-sm font-semibold text-on-surface mb-1">Execution rating</label>
            <div className="flex gap-1">
              {[1, 2, 3, 4, 5].map((n) => (
                <button
                  key={n}
                  onClick={() => setRating(rating === n ? null : n)}
                  className={`w-9 h-9 rounded-lg border text-body-md font-semibold transition-colors ${
                    rating != null && n <= rating ? 'bg-primary text-on-primary border-primary' : 'border-outline-variant text-on-surface-variant hover:bg-surface-container-high'
                  }`}
                >
                  {n}
                </button>
              ))}
            </div>
          </div>
          <div>
            <label className="block text-label-sm font-semibold text-on-surface mb-1">Notes</label>
            <textarea value={notes} onChange={(e) => setNotes(e.target.value)} rows={4} className={inputCls} />
          </div>
          {save.error && <p className="text-body-sm text-error">{(save.error as Error).message}</p>}
          <div className="flex items-center gap-3">
            <button
              onClick={() => save.mutate()}
              disabled={save.isPending}
              className="px-4 py-2 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity disabled:opacity-50"
            >
              {save.isPending ? 'Saving…' : 'Save plan'}
            </button>
            <button
              onClick={() => (confirmDelete ? remove.mutate() : setConfirmDelete(true))}
              className="px-4 py-2 rounded-lg text-body-md text-error hover:bg-error-container transition-colors"
            >
              {confirmDelete ? 'Confirm delete trade' : 'Delete trade'}
            </button>
          </div>
        </div>

        {/* Checklist */}
        <div className="space-y-3">
          <h3 className="text-label-caps text-on-surface-variant uppercase">
            Checklist {trade.strategy && <span className="normal-case">· {trade.strategy.name}</span>}
          </h3>
          {trade.rule_checks.length === 0 && (
            <p className="text-body-md text-on-surface-variant">{trade.strategy_id ? 'This strategy has no rules.' : 'Link a strategy to judge this trade against its rules.'}</p>
          )}
          {trade.rule_checks.map((c) => (
            <div key={c.id} className="flex items-center justify-between gap-3 py-1.5 border-b border-outline-variant last:border-0">
              <p className="text-body-md text-on-surface">
                <span className="text-[10px] font-bold uppercase text-on-surface-variant mr-1.5">{c.kind}</span>
                {c.text}
              </p>
              <div className="flex gap-1 flex-shrink-0">
                {([[true, 'check', 'text-secondary'], [false, 'close', 'text-error']] as const).map(([value, icon, color]) => (
                  <button
                    key={icon}
                    onClick={() => checks.mutate({ id: c.id, met: c.met === value ? null : value })}
                    title={value ? 'Met' : 'Not met'}
                    className={`w-8 h-8 flex items-center justify-center rounded-lg border transition-colors ${
                      c.met === value ? `border-current ${color}` : 'border-outline-variant text-on-surface-variant hover:bg-surface-container-high'
                    }`}
                  >
                    <span className="material-symbols-outlined text-base">{icon}</span>
                  </button>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Fills */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-label-caps text-on-surface-variant uppercase">Fills</h3>
          <button onClick={() => setAdding((v) => !v)} className="flex items-center gap-1 text-label-sm font-semibold text-secondary hover:opacity-80">
            <span className="material-symbols-outlined text-base">{adding ? 'expand_less' : 'add'}</span>
            Add transaction
          </button>
        </div>
        {adding && <Candidates tradeId={trade.id} />}
        <div className="overflow-hidden rounded-lg border border-outline-variant">
          <table className="w-full text-left border-collapse">
            <thead className="bg-surface-container-low">
              <tr className="text-[10px] text-label-caps text-on-surface-variant uppercase">
                <th className="px-4 py-2">Date</th>
                <th className="px-4 py-2">Symbol</th>
                <th className="px-4 py-2">Fill</th>
                <th className="px-4 py-2 text-right">Quantity</th>
                <th className="px-4 py-2 text-right">Price</th>
                <th className="px-4 py-2">Exit reason</th>
                <th className="px-4 py-2 w-10" />
              </tr>
            </thead>
            <tbody className="divide-y divide-outline-variant">
              {trade.fills.map((f) => <FillRow key={f.id} tradeId={trade.id} fill={f} />)}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

function FillRow({ tradeId, fill }: { tradeId: string; fill: TradeFill }) {
  const invalidate = useInvalidateTrades()
  const txn = fill.transaction
  const exit = txn?.effect === 'close'
  const reason = useMutation({
    mutationFn: (exitReason: string) => updateTradeFill(tradeId, fill.id, { exit_reason: exitReason }),
    onSuccess: invalidate,
  })
  const remove = useMutation({ mutationFn: () => removeTradeFill(tradeId, fill.id), onSuccess: invalidate })
  const reattach = useMutation({ mutationFn: () => addTradeFill(tradeId, fill.transaction_id), onSuccess: invalidate })
  const error = reason.error ?? remove.error ?? reattach.error

  return (
    <tr className={`text-data-tabular text-on-surface tabular-nums ${fill.detached ? 'opacity-50' : ''}`}>
      <td className="px-4 py-3">{txn ? formatDate(txn.date) : '—'}</td>
      <td className="px-4 py-3">{txn?.symbol}</td>
      <td className="px-4 py-3">
        <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-bold ${exit ? 'bg-surface-container-high text-on-surface-variant' : 'bg-secondary/10 text-secondary'}`}>
          {exit ? 'Exit' : 'Entry'} · {txn?.action}
        </span>
        {fill.source === 'auto' && !fill.detached && (
          <span className="ml-2 text-[10px] font-bold uppercase text-on-surface-variant" title="Attached automatically: it closed shares this trade had open">Auto</span>
        )}
        {fill.detached && <span className="ml-2 text-[10px] font-bold uppercase text-on-surface-variant">Detached</span>}
        {error && <p className="text-label-sm text-error">{(error as Error).message}</p>}
      </td>
      <td className="px-4 py-3 text-right">
        {fmtNumber(fill.quantity)}
        {txn?.quantity != null && Math.abs(txn.quantity) - fill.quantity > 1e-9 && (
          <span className="text-label-sm text-on-surface-variant"> of {fmtNumber(Math.abs(txn.quantity))}</span>
        )}
      </td>
      <td className="px-4 py-3 text-right">{txn?.price != null ? fmtCurrency(txn.price) : '—'}</td>
      <td className="px-4 py-3">
        {exit && !fill.detached && (
          <select value={fill.exit_reason} onChange={(e) => reason.mutate(e.target.value)} className={filterCls}>
            <option value="">—</option>
            {EXIT_REASONS.map((r) => <option key={r.value} value={r.value}>{r.label}</option>)}
          </select>
        )}
      </td>
      <td className="px-4 py-3 text-right w-10">
        {fill.detached ? (
          <button onClick={() => reattach.mutate()} title="Re-attach" className="text-on-surface-variant hover:text-primary">
            <span className="material-symbols-outlined text-lg align-middle">undo</span>
          </button>
        ) : (
          <button onClick={() => remove.mutate()} title={fill.source === 'auto' ? 'Detach from this trade' : 'Remove from this trade'} className="text-on-surface-variant hover:text-error">
            <span className="material-symbols-outlined text-lg align-middle">link_off</span>
          </button>
        )}
      </td>
    </tr>
  )
}

// Candidates lists transactions on the trade's underlying that could join it:
// a scale-in, another leg, or a sell that wasn't attached on its own.
function Candidates({ tradeId }: { tradeId: string }) {
  const invalidate = useInvalidateTrades()
  const queryClient = useQueryClient()
  const { data: candidates = [], isLoading } = useQuery({
    queryKey: ['trades', tradeId, 'candidates'],
    queryFn: () => getTradeCandidates(tradeId),
  })
  const add = useMutation({
    mutationFn: (txnId: string) => addTradeFill(tradeId, txnId),
    onSuccess: () => { invalidate(); queryClient.invalidateQueries({ queryKey: ['trades', tradeId, 'candidates'] }) },
  })

  return (
    <div className="rounded-lg border border-dashed border-outline-variant p-3 space-y-1">
      {isLoading && <p className="text-body-sm text-on-surface-variant">Loading…</p>}
      {!isLoading && candidates.length === 0 && (
        <p className="text-body-sm text-on-surface-variant">No unassigned transactions on this underlying since the trade opened.</p>
      )}
      {candidates.map((c) => (
        <div key={c.transaction.id} className="flex items-center justify-between text-data-tabular tabular-nums text-on-surface">
          <span>
            {formatDate(c.transaction.date)} · {c.transaction.symbol} · {c.transaction.action} {fmtNumber(c.available_quantity)}
            {c.transaction.price != null && ` @ ${fmtCurrency(c.transaction.price)}`}
            {c.detached && <span className="ml-2 text-[10px] font-bold uppercase text-on-surface-variant">Detached</span>}
          </span>
          <button
            onClick={() => add.mutate(c.transaction.id)}
            disabled={add.isPending}
            className="text-label-sm font-semibold text-secondary hover:opacity-80 disabled:opacity-50"
          >
            {c.transaction.effect === 'open' ? 'Add entry' : 'Add exit'}
          </button>
        </div>
      ))}
      {add.error && <p className="text-label-sm text-error">{(add.error as Error).message}</p>}
    </div>
  )
}

// ── Strategies tab ───────────────────────────────────────────────────────────

function StrategiesTab() {
  const [editing, setEditing] = useState<Strategy | 'new' | null>(null)
  const { data: strategies = [], isLoading } = useQuery({ queryKey: ['strategies'], queryFn: () => getStrategies() })

  return (
    <>
      <div className="flex justify-end mb-4">
        <button
          onClick={() => setEditing('new')}
          className="flex items-center gap-1.5 px-4 py-1.5 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity"
        >
          <span className="material-symbols-outlined text-lg">add</span>
          New Strategy
        </button>
      </div>
      {isLoading && <p className="text-body-md text-on-surface-variant">Loading strategies…</p>}
      {!isLoading && strategies.length === 0 && (
        <div className="bg-surface-container-lowest rounded-xl shadow-card p-6 text-body-md text-on-surface-variant">
          No strategies yet. A strategy is a checklist of entry, exit and risk rules each trade is judged against.
        </div>
      )}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {strategies.map((s) => (
          <div key={s.id} className={`bg-surface-container-lowest rounded-xl shadow-card p-6 ${s.status === 'retired' ? 'opacity-60' : ''}`}>
            <div className="flex items-start justify-between mb-3">
              <div>
                <div className="flex items-center gap-2">
                  <h2 className="text-headline-sm text-on-surface">{s.name}</h2>
                  {s.status === 'retired' && <span className="text-[10px] font-bold uppercase text-on-surface-variant">Retired</span>}
                </div>
                <p className="text-label-sm text-on-surface-variant capitalize">
                  {[s.direction, s.timeframe].filter(Boolean).join(' · ')}
                </p>
              </div>
              <button onClick={() => setEditing(s)} className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high">
                <span className="material-symbols-outlined text-lg">edit</span>
              </button>
            </div>
            {s.description && <p className="text-body-md text-on-surface mb-3 whitespace-pre-line">{s.description}</p>}
            {RULE_KINDS.map(({ value, label }) => {
              const rules = s.rules.filter((r) => r.kind === value)
              if (rules.length === 0) return null
              return (
                <div key={value} className="mb-2">
                  <p className="text-label-caps text-on-surface-variant uppercase mb-1">{label}</p>
                  <ul className="list-disc pl-5 space-y-0.5">
                    {rules.map((r) => <li key={r.id} className="text-body-md text-on-surface">{r.text}</li>)}
                  </ul>
                </div>
              )
            })}
          </div>
        ))}
      </div>
      {editing && (
        <StrategyModal
          key={editing === 'new' ? 'new' : editing.id}
          strategy={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
        />
      )}
    </>
  )
}

function StrategyModal({ strategy, onClose }: { strategy: Strategy | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(strategy?.name ?? '')
  const [description, setDescription] = useState(strategy?.description ?? '')
  const [direction, setDirection] = useState<string>(strategy?.direction ?? 'long')
  const [timeframe, setTimeframe] = useState(strategy?.timeframe ?? '')
  const [status, setStatus] = useState<'active' | 'retired'>(strategy?.status ?? 'active')
  const [rules, setRules] = useState<{ kind: RuleKind; text: string }[]>(
    strategy?.rules.map((r) => ({ kind: r.kind, text: r.text })) ?? [{ kind: 'entry', text: '' }],
  )
  const [confirmDelete, setConfirmDelete] = useState(false)

  const done = () => {
    queryClient.invalidateQueries({ queryKey: ['strategies'] })
    queryClient.invalidateQueries({ queryKey: ['trades'] })
    queryClient.invalidateQueries({ queryKey: ['strategy-performance'] })
    onClose()
  }
  const save = useMutation({
    mutationFn: (payload: StrategyPayload) => (strategy ? updateStrategy(strategy.id, payload) : createStrategy(payload)),
    onSuccess: done,
  })
  const remove = useMutation({ mutationFn: () => deleteStrategy(strategy!.id), onSuccess: done })

  const setRule = (i: number, patch: Partial<{ kind: RuleKind; text: string }>) =>
    setRules((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  const moveRule = (i: number, by: number) =>
    setRules((rs) => {
      const next = [...rs]
      const [r] = next.splice(i, 1)
      next.splice(Math.max(0, Math.min(next.length, i + by)), 0, r)
      return next
    })

  function submit() {
    save.mutate({
      name,
      description,
      direction,
      timeframe,
      status,
      rules: rules.filter((r) => r.text.trim() !== ''),
    })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center p-4" onClick={onClose}>
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" />
      <div
        className="relative bg-surface-container-lowest rounded-xl shadow-card w-full max-w-2xl p-6 max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
        style={{ animation: 'slideUp 0.25s ease-out' }}
      >
        <div className="flex items-start justify-between mb-6">
          <div>
            <h2 className="text-headline-sm text-on-surface">{strategy ? 'Edit Strategy' : 'New Strategy'}</h2>
            {strategy && <p className="text-label-sm text-on-surface-variant">Trades already started keep the rules they were judged by.</p>}
          </div>
          <button onClick={onClose} className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors">
            <span className="material-symbols-outlined text-xl">close</span>
          </button>
        </div>

        <div className="space-y-4">
          <div>
            <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Name</label>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Base breakout" className={inputCls} />
          </div>
          <div>
            <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Description</label>
            <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} className={inputCls} />
          </div>
          <div className="grid grid-cols-3 gap-4">
            <div>
              <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Direction</label>
              <select value={direction} onChange={(e) => setDirection(e.target.value)} className={inputCls}>
                <option value="long">Long</option>
                <option value="short">Short</option>
                <option value="both">Both</option>
              </select>
            </div>
            <div>
              <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Timeframe</label>
              <input value={timeframe} onChange={(e) => setTimeframe(e.target.value)} placeholder="e.g. swing" className={inputCls} />
            </div>
            <div>
              <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Status</label>
              <select value={status} onChange={(e) => setStatus(e.target.value as 'active' | 'retired')} className={inputCls}>
                <option value="active">Active</option>
                <option value="retired">Retired</option>
              </select>
            </div>
          </div>

          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label className="text-label-sm font-semibold text-on-surface">Rules</label>
              <button onClick={() => setRules((rs) => [...rs, { kind: 'entry', text: '' }])} className="flex items-center gap-1 text-label-sm font-semibold text-secondary hover:opacity-80">
                <span className="material-symbols-outlined text-base">add</span>
                Add rule
              </button>
            </div>
            <div className="space-y-2">
              {rules.map((r, i) => (
                <div key={i} className="flex items-center gap-2">
                  <select value={r.kind} onChange={(e) => setRule(i, { kind: e.target.value as RuleKind })} className={`${fieldCls} w-28 flex-shrink-0`}>
                    {RULE_KINDS.map((k) => <option key={k.value} value={k.value}>{k.label}</option>)}
                  </select>
                  <input value={r.text} onChange={(e) => setRule(i, { text: e.target.value })} placeholder="Rule" className={`${fieldCls} flex-1 min-w-0`} />
                  <button onClick={() => moveRule(i, -1)} disabled={i === 0} className="text-on-surface-variant hover:text-primary disabled:opacity-30" title="Move up">
                    <span className="material-symbols-outlined text-lg align-middle">arrow_upward</span>
                  </button>
                  <button onClick={() => moveRule(i, 1)} disabled={i === rules.length - 1} className="text-on-surface-variant hover:text-primary disabled:opacity-30" title="Move down">
                    <span className="material-symbols-outlined text-lg align-middle">arrow_downward</span>
                  </button>
                  <button onClick={() => setRules((rs) => rs.filter((_, j) => j !== i))} className="text-on-surface-variant hover:text-error" title="Remove">
                    <span className="material-symbols-outlined text-lg align-middle">delete</span>
                  </button>
                </div>
              ))}
            </div>
          </div>
        </div>

        {(save.error ?? remove.error) && <p className="mt-4 text-body-sm text-error">{((save.error ?? remove.error) as Error).message}</p>}

        <div className="flex gap-3 mt-6">
          {strategy && (
            <button
              onClick={() => (confirmDelete ? remove.mutate() : setConfirmDelete(true))}
              className="px-4 py-2.5 rounded-lg text-body-md text-error hover:bg-error-container transition-colors"
            >
              {confirmDelete ? 'Confirm delete' : 'Delete'}
            </button>
          )}
          <div className="flex-1" />
          <button onClick={onClose} className="px-4 py-2.5 border border-outline-variant rounded-lg text-body-md text-on-surface hover:bg-surface-container-high transition-colors">
            Cancel
          </button>
          <button
            onClick={submit}
            disabled={save.isPending || name.trim() === ''}
            className="px-4 py-2.5 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {save.isPending ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Performance tab ──────────────────────────────────────────────────────────

const PERF_COLUMNS: { label: string; title?: string; cell: (s: TradeStats) => { text: string; cls?: string } }[] = [
  { label: 'Trades', cell: (s) => ({ text: String(s.trades) }) },
  { label: 'Win rate', cell: (s) => ({ text: s.win_rate == null ? '—' : `${s.win_rate.toFixed(0)}%` }) },
  { label: 'Net P&L', cell: (s) => ({ text: fmtSigned(s.net_pnl), cls: gainColor(s.net_pnl) }) },
  { label: 'Avg win', cell: (s) => ({ text: s.avg_win == null ? '—' : fmtCurrency(s.avg_win), cls: gainColor(s.avg_win) }) },
  { label: 'Avg loss', cell: (s) => ({ text: s.avg_loss == null ? '—' : fmtSigned(s.avg_loss), cls: gainColor(s.avg_loss) }) },
  { label: 'Profit factor', title: 'Gross winnings over gross losses', cell: (s) => ({ text: s.profit_factor == null ? '—' : s.profit_factor.toFixed(2) }) },
  { label: 'Expectancy', title: 'Average net result per trade', cell: (s) => ({ text: s.expectancy == null ? '—' : fmtSigned(s.expectancy), cls: gainColor(s.expectancy) }) },
  { label: 'Avg R', title: 'Over trades with a planned risk', cell: (s) => ({ text: fmtR(s.avg_r), cls: gainColor(s.avg_r) }) },
  { label: 'Max DD', title: 'Largest fall in cumulative net P&L, in the order trades closed', cell: (s) => ({ text: s.max_drawdown ? `-${fmtCurrency(s.max_drawdown)}` : '—' }) },
  { label: 'Avg hold', cell: (s) => ({ text: s.avg_holding_days == null ? '—' : `${s.avg_holding_days.toFixed(1)}d` }) },
  { label: 'Rules met', title: 'Share of answered checklist items that were met', cell: (s) => ({ text: s.rule_adherence == null ? '—' : `${s.rule_adherence.toFixed(0)}%` }) },
]

function PerformanceTab() {
  const { data, isLoading } = useQuery({ queryKey: ['strategy-performance'], queryFn: () => getStrategyPerformance() })
  const rows = data?.strategies ?? []

  return (
    <div className="bg-surface-container-lowest rounded-xl shadow-card">
      <div className="px-6 py-4 border-b border-outline-variant">
        <h2 className="text-headline-sm text-on-surface">Performance by Strategy</h2>
        <p className="text-label-sm text-on-surface-variant">Closed trades only. Open trades count once they're flat.</p>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="border-b border-outline-variant">
              <th className={`${thCls} text-left`}>Strategy</th>
              {PERF_COLUMNS.map((c) => <th key={c.label} title={c.title} className={`${thCls} text-right`}>{c.label}</th>)}
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr><td colSpan={PERF_COLUMNS.length + 1} className="px-4 py-10 text-center text-body-md text-on-surface-variant">Loading…</td></tr>
            )}
            {!isLoading && rows.length === 0 && (
              <tr><td colSpan={PERF_COLUMNS.length + 1} className="px-4 py-10 text-center text-body-md text-on-surface-variant">No closed trades yet.</td></tr>
            )}
            {[...rows, ...(rows.length > 1 && data ? [{ ...data.total, strategy_name: 'All trades' }] : [])].map((s, i) => (
              <tr key={s.strategy_id ?? `${s.strategy_name}-${i}`} className={`border-b border-outline-variant text-data-tabular tabular-nums text-on-surface ${s.strategy_name === 'All trades' ? 'font-semibold bg-surface-container-low' : ''}`}>
                <td className="px-4 py-3 text-body-md">{s.strategy_name}</td>
                {PERF_COLUMNS.map((c) => {
                  const { text, cls } = c.cell(s)
                  return <td key={c.label} className={`px-4 py-3 text-right ${cls ?? ''}`}>{text}</td>
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ── Page ─────────────────────────────────────────────────────────────────────

export default function Trades() {
  // ?trade=<id> opens that trade, which is how Portfolio links into the journal.
  const [params] = useSearchParams()
  const focusId = params.get('trade')
  const [tab, setTab] = useState<Tab>((params.get('tab') as Tab | null) ?? 'Trades')

  return (
    <div className="p-8">
      <div className="flex items-start justify-between mb-6">
        <div>
          <h1 className="text-headline-lg text-on-surface mb-1">Trading Journal</h1>
          <p className="text-body-lg text-on-surface-variant">Trades, the strategies behind them, and how each one performs.</p>
        </div>
      </div>

      <div className="flex gap-6 border-b border-outline-variant mb-6">
        {tabs.map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`pb-2 text-body-md transition-colors ${
              t === tab ? 'border-b-2 border-primary font-bold text-primary' : 'text-on-surface-variant hover:text-primary'
            }`}
          >
            {t}
          </button>
        ))}
      </div>

      {tab === 'Trades' && <TradesTab focusId={focusId} />}
      {tab === 'Strategies' && <StrategiesTab />}
      {tab === 'Performance' && <PerformanceTab />}
    </div>
  )
}
