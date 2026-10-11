import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createTrade } from '../api/trades'
import { getStrategies } from '../api/strategies'
import type { Transaction } from '../api/transactions'
import { formatDate } from '../lib/datetime'

const inputCls = 'w-full px-3 py-2.5 bg-surface-container-low border border-outline-variant rounded-lg text-body-md text-on-surface placeholder:text-on-surface-variant focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors'
const numInputCls = `${inputCls} tabular-nums`

const fmtCurrency = (n: number) => (n ?? 0).toLocaleString('en-US', { style: 'currency', currency: 'USD' })

function parseOptional(v: string): number | null {
  const n = parseFloat(v)
  return Number.isFinite(n) ? n : null
}

// StartTradeTarget is the opening transaction a trade starts from - all the
// modal needs of it, so a tax lot can supply it from its own fields.
export type StartTradeTarget = Pick<Transaction, 'id' | 'symbol' | 'action' | 'date' | 'quantity' | 'price' | 'direction' | 'asset_type'>

// StartTradeModal journals an opening transaction as the first entry of a
// trade: the strategy it was taken under, the stop that sets its risk, and
// which of the strategy's entry and risk rules the setup met. Later sells on the
// holding attach to it on their own.
export default function StartTradeModal({ txn, onClose, onStarted }: {
  txn: StartTradeTarget | null
  onClose: () => void
  onStarted?: (tradeId: string) => void
}) {
  const queryClient = useQueryClient()
  const { data: strategies = [] } = useQuery({
    queryKey: ['strategies', 'active'],
    queryFn: () => getStrategies('active'),
    enabled: txn !== null,
  })

  // Seeded from the transaction; the parent remounts this per transaction via a
  // `key`.
  const [quantity, setQuantity] = useState(txn?.quantity != null ? String(Math.abs(txn.quantity)) : '')
  const [strategyId, setStrategyId] = useState('')
  const [stop, setStop] = useState('')
  const [target, setTarget] = useState('')
  const [risk, setRisk] = useState('')
  const [notes, setNotes] = useState('')
  const [met, setMet] = useState<Set<string>>(new Set())

  const { mutate, isPending, error } = useMutation({
    mutationFn: createTrade,
    onSuccess: (trade) => {
      queryClient.invalidateQueries({ queryKey: ['trades'] })
      queryClient.invalidateQueries({ queryKey: ['strategy-performance'] })
      onStarted?.(trade.id)
      onClose()
    },
  })

  if (!txn) return null

  const strategy = strategies.find((s) => s.id === strategyId)
  // Exit rules can't be judged until the trade is out, so only entry and risk
  // rules are asked about now.
  const askedRules = strategy?.rules.filter((r) => r.kind !== 'exit') ?? []

  const qty = parseOptional(quantity)
  const stopValue = parseOptional(stop)
  const short = txn.direction === 'short'
  // The stop belongs on the losing side of the entry; a long's stop above its
  // entry is almost always a typo. The server accepts either.
  const stopWrongSide = stopValue != null && txn.price != null && (short ? stopValue <= txn.price : stopValue >= txn.price)
  const derivedRisk = stopValue != null && txn.price != null && qty != null && txn.asset_type !== 'Option'
    ? Math.abs(txn.price - stopValue) * qty
    : null

  const canSubmit = qty != null && qty > 0

  function toggleRule(id: string) {
    setMet((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function submit() {
    mutate({
      transaction_id: txn!.id,
      quantity: qty,
      strategy_id: strategyId || null,
      initial_stop: stopValue,
      target: parseOptional(target),
      planned_risk: parseOptional(risk),
      notes,
      met_rule_ids: strategy ? [...met] : undefined,
    })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center p-4" onClick={onClose}>
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" />
      <div
        className="relative bg-surface-container-lowest rounded-xl shadow-card w-full max-w-lg p-6 max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
        style={{ animation: 'slideUp 0.25s ease-out' }}
      >
        <div className="flex items-start justify-between mb-6">
          <div>
            <h2 className="text-headline-sm text-on-surface">Start Trade</h2>
            <p className="text-label-sm text-on-surface-variant">
              {txn.symbol} · {txn.action} {txn.quantity != null ? Math.abs(txn.quantity) : ''}
              {txn.price != null && ` @ ${fmtCurrency(txn.price)}`} · {formatDate(txn.date)}
            </p>
          </div>
          <button onClick={onClose} className="w-8 h-8 flex items-center justify-center rounded-lg text-on-surface-variant hover:bg-surface-container-high transition-colors">
            <span className="material-symbols-outlined text-xl">close</span>
          </button>
        </div>

        <div className="space-y-4">
          <div>
            <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Strategy</label>
            <select value={strategyId} onChange={(e) => { setStrategyId(e.target.value); setMet(new Set()) }} className={inputCls}>
              <option value="">No strategy</option>
              {strategies.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
          </div>

          {askedRules.length > 0 && (
            <div>
              <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Setup checklist</label>
              <div className="space-y-1.5 rounded-lg border border-outline-variant p-3">
                {askedRules.map((r) => (
                  <label key={r.id} className="flex items-start gap-2 text-body-md text-on-surface cursor-pointer">
                    <input type="checkbox" checked={met.has(r.id)} onChange={() => toggleRule(r.id)} className="mt-1 accent-secondary" />
                    <span>
                      <span className="text-[10px] font-bold uppercase text-on-surface-variant mr-1.5">{r.kind}</span>
                      {r.text}
                    </span>
                  </label>
                ))}
              </div>
              <p className="mt-1 text-label-sm text-on-surface-variant">Unticked rules are recorded as not met. Exit rules are checked when you're out.</p>
            </div>
          )}

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Quantity</label>
              <input type="number" value={quantity} onChange={(e) => setQuantity(e.target.value)} className={numInputCls} />
            </div>
            <div>
              <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Initial Stop</label>
              <div className="relative">
                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-body-md text-on-surface-variant">$</span>
                <input type="number" value={stop} onChange={(e) => setStop(e.target.value)} placeholder="0.00" className={`${numInputCls} pl-7`} />
              </div>
            </div>
            <div>
              <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Target</label>
              <div className="relative">
                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-body-md text-on-surface-variant">$</span>
                <input type="number" value={target} onChange={(e) => setTarget(e.target.value)} placeholder="Optional" className={`${numInputCls} pl-7`} />
              </div>
            </div>
            <div>
              <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Planned Risk</label>
              <div className="relative">
                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-body-md text-on-surface-variant">$</span>
                <input
                  type="number"
                  value={risk}
                  onChange={(e) => setRisk(e.target.value)}
                  placeholder={derivedRisk != null ? derivedRisk.toFixed(2) : 'From stop'}
                  className={`${numInputCls} pl-7`}
                />
              </div>
            </div>
          </div>
          {stopWrongSide && (
            <p className="text-label-sm text-error">The stop is {short ? 'below' : 'above'} the entry for a {short ? 'short' : 'long'} - check it.</p>
          )}
          <p className="text-label-sm text-on-surface-variant">
            Planned risk is what you lose if the initial stop is hit, and what R is measured against. Leave it blank to derive it from the stop.
          </p>

          <div>
            <label className="block text-label-sm font-semibold text-on-surface mb-1.5">Notes</label>
            <textarea value={notes} onChange={(e) => setNotes(e.target.value)} rows={3} placeholder="Why this trade, why now" className={inputCls} />
          </div>
        </div>

        {error && <p className="mt-4 text-body-sm text-error">{(error as Error).message}</p>}

        <div className="flex gap-3 mt-6">
          <button onClick={onClose} disabled={isPending} className="flex-1 px-4 py-2.5 border border-outline-variant rounded-lg text-body-md text-on-surface hover:bg-surface-container-high transition-colors disabled:opacity-50">
            Cancel
          </button>
          <button
            onClick={submit}
            disabled={isPending || !canSubmit}
            className="flex-1 px-4 py-2.5 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {isPending ? 'Starting…' : 'Start Trade'}
          </button>
        </div>
      </div>
    </div>
  )
}
