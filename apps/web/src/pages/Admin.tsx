import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { UseMutationResult } from '@tanstack/react-query'
import {
  backfillProfiles,
  getLatestAdminLogs,
  recalculateHoldings,
  refreshPrices,
  UTILITY_BACKFILL_PROFILES,
  UTILITY_RECALCULATE,
  UTILITY_REFRESH_PRICES,
  type AdminLog,
  type PriceRefreshResult,
  type QuotedSymbol,
} from '../api/admin'
import { formatDateTime, formatRelative } from '../lib/datetime'

const fmtCurrency = (n: number) => (n ?? 0).toLocaleString('en-US', { style: 'currency', currency: 'USD' })

const fmtDuration = (ms: number) => {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  const mins = Math.floor(ms / 60000)
  return `${mins}m ${Math.round((ms % 60000) / 1000)}s`
}

// The last-run line under a tool's title. Absent until the tool has run once,
// which is itself the useful signal - it means "never, as far as we know".
function LastRun({ log }: { log?: AdminLog }) {
  if (!log) {
    return <p className="text-label-sm text-on-surface-variant mt-2">Never run.</p>
  }

  const errorCount = log.errors ? log.errors.split('\n').filter((l) => l.trim() !== '').length : 0

  return (
    <p className="text-label-sm text-on-surface-variant mt-2 flex items-center gap-1.5 flex-wrap">
      {/* Relative reads faster, absolute is precise - so the exact time is the
          tooltip rather than a second line. */}
      <span title={formatDateTime(log.started_at)}>Last run {formatRelative(log.started_at)}</span>
      <span aria-hidden>·</span>
      <span>took {fmtDuration(log.duration_ms)}</span>
      {errorCount > 0 && (
        <>
          <span aria-hidden>·</span>
          <span className="text-error font-semibold" title={log.errors ?? undefined}>
            {errorCount} {errorCount === 1 ? 'error' : 'errors'}
          </span>
        </>
      )}
    </p>
  )
}

// A count worth reporting back from a utility run.
interface Stat {
  label: string
  value: number
  // 'good' and 'bad' tint the number; most counts are neither.
  tone?: 'good' | 'bad'
}

function statColor(stat: Stat) {
  if (stat.tone === 'bad' && stat.value > 0) return 'text-error'
  if (stat.tone === 'good' && stat.value > 0) return 'text-secondary'
  return 'text-on-surface'
}

function StatRow({ stats }: { stats: Stat[] }) {
  return (
    <div className="flex flex-wrap gap-x-8 gap-y-3">
      {stats.map((stat) => (
        <div key={stat.label}>
          <p className="text-label-caps text-on-surface-variant uppercase">{stat.label}</p>
          <p className={`text-headline-sm tabular-nums ${statColor(stat)}`}>{stat.value.toLocaleString('en-US')}</p>
        </div>
      ))}
    </div>
  )
}

interface UtilityCardProps<T> {
  icon: string
  title: string
  description: string
  action: string
  pendingLabel: string
  mutation: UseMutationResult<T, Error, void, unknown>
  // The tool's most recent recorded run, if it has ever run.
  lastRun?: AdminLog
  // Rendered once a run has returned something.
  children?: (result: T) => React.ReactNode
}

// One admin chore: what it does, a button to run it, and whatever the run
// reported. Every utility here is a manual sweep over the whole book, so the
// card needs no inputs - only a result.
function UtilityCard<T>({
  icon,
  title,
  description,
  action,
  pendingLabel,
  mutation,
  lastRun,
  children,
}: UtilityCardProps<T>) {
  return (
    <div className="bg-surface-container-lowest rounded-xl shadow-card">
      <div className="flex items-start justify-between gap-6 px-6 py-5">
        <div className="flex items-start gap-4 min-w-0">
          <div className="w-10 h-10 rounded-lg bg-surface-container-high flex items-center justify-center flex-shrink-0">
            <span className="material-symbols-outlined text-on-surface-variant text-xl">{icon}</span>
          </div>
          <div className="min-w-0">
            <h2 className="text-headline-sm text-on-surface">{title}</h2>
            <p className="text-body-md text-on-surface-variant mt-1 max-w-xl">{description}</p>
            <LastRun log={lastRun} />
          </div>
        </div>
        <button
          onClick={() => mutation.mutate()}
          disabled={mutation.isPending}
          className="px-4 py-2.5 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity disabled:opacity-50 whitespace-nowrap flex-shrink-0"
        >
          {mutation.isPending ? pendingLabel : action}
        </button>
      </div>

      {(mutation.isError || mutation.data !== undefined) && (
        <div className="px-6 py-5 border-t border-outline-variant">
          {mutation.isError ? (
            <p className="text-body-md text-error">{mutation.error.message}</p>
          ) : (
            children?.(mutation.data as T)
          )}
        </div>
      )}
    </div>
  )
}

const OUTCOME_LABELS: Record<QuotedSymbol['outcome'], string> = {
  priced: 'Priced',
  not_found: 'No quote',
  failed: 'Failed',
}

function outcomeChip(outcome: QuotedSymbol['outcome']) {
  const tone =
    outcome === 'priced'
      ? 'bg-secondary-container text-on-secondary-container'
      : outcome === 'failed'
        ? 'bg-error-container text-on-error-container'
        : 'bg-surface-container-high text-on-surface-variant'
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-label-sm font-semibold ${tone}`}>
      {OUTCOME_LABELS[outcome]}
    </span>
  )
}

function priceChange(row: QuotedSymbol) {
  if (row.outcome !== 'priced') return <span className="text-on-surface-variant">—</span>
  const delta = row.price - row.previous_price
  if (row.previous_price === 0) return <span className="text-on-surface-variant">new</span>
  const color = delta > 0 ? 'text-secondary' : delta < 0 ? 'text-error' : 'text-on-surface-variant'
  return <span className={color}>{`${delta >= 0 ? '+' : '-'}${fmtCurrency(Math.abs(delta))}`}</span>
}

// The per-symbol log of a price run: what each quote came back as, against the
// price it replaced, so an obviously wrong quote shows up as a jump.
function QuoteLog({ result }: { result: PriceRefreshResult }) {
  if (result.symbols.length === 0) {
    return <p className="text-body-md text-on-surface-variant">No open stock positions to price.</p>
  }

  return (
    <>
      <div className="flex items-baseline justify-between mb-4">
        <h3 className="text-body-md font-semibold text-on-surface">Quotes</h3>
        <p className="text-label-sm text-on-surface-variant">
          {result.provider && <>via {result.provider} · </>}Run at {formatDateTime(result.fetched_at)}
        </p>
      </div>
      <div className="overflow-x-auto -mx-6">
        <table className="w-full">
          <thead>
            <tr className="border-y border-outline-variant">
              <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Symbol</th>
              <th className="text-left px-6 py-3 text-label-caps text-on-surface-variant uppercase">Result</th>
              <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Previous</th>
              <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">New Price</th>
              <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Change</th>
              <th className="text-right px-6 py-3 text-label-caps text-on-surface-variant uppercase">Price As Of</th>
            </tr>
          </thead>
          <tbody>
            {result.symbols.map((row) => (
              <tr key={row.symbol} className="border-b border-outline-variant last:border-0">
                <td className="px-6 py-3 text-body-md font-medium text-on-surface">{row.symbol}</td>
                <td className="px-6 py-3">{outcomeChip(row.outcome)}</td>
                <td className="px-6 py-3 text-right text-data-tabular text-on-surface-variant tabular-nums">
                  {row.previous_price ? fmtCurrency(row.previous_price) : '—'}
                </td>
                <td className="px-6 py-3 text-right text-data-tabular text-on-surface tabular-nums">
                  {row.outcome === 'priced' ? fmtCurrency(row.price) : '—'}
                </td>
                <td className="px-6 py-3 text-right text-data-tabular font-semibold tabular-nums">{priceChange(row)}</td>
                <td className="px-6 py-3 text-right text-body-md text-on-surface-variant whitespace-nowrap">
                  {row.as_of ? formatDateTime(row.as_of) : '—'}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  )
}

export default function Admin() {
  const queryClient = useQueryClient()

  // The most recent run of each tool, for the last-run line in its header. One
  // query for the whole page, keyed by utility below.
  const logsQuery = useQuery({ queryKey: ['admin', 'logs'], queryFn: getLatestAdminLogs })
  const logs = new Map((logsQuery.data ?? []).map((l) => [l.utility, l]))

  // Every utility here rewrites holdings, so anything drawn from them is stale
  // the moment a run finishes. The holdings key is a prefix, which takes the
  // allocation query (['holdings', 'allocation']) with it; the logs refetch
  // because the run that just finished is now the latest one.
  const onRunFinished = () => {
    queryClient.invalidateQueries({ queryKey: ['holdings'] })
    queryClient.invalidateQueries({ queryKey: ['admin', 'logs'] })
  }

  const prices = useMutation({ mutationFn: refreshPrices, onSuccess: onRunFinished })
  const profiles = useMutation({ mutationFn: backfillProfiles, onSuccess: onRunFinished })
  const recalc = useMutation({ mutationFn: recalculateHoldings, onSuccess: onRunFinished })

  return (
    <div className="p-8">
      <div className="mb-8">
        <h1 className="text-headline-lg text-on-surface mb-2">Admin</h1>
        <p className="text-body-lg text-on-surface-variant max-w-2xl">
          Maintenance utilities you run by hand. Each one sweeps the whole portfolio and reports what it touched.
        </p>
      </div>

      <div className="space-y-6">
        <UtilityCard
          icon="trending_up"
          title="Refresh Stock Prices"
          description="Quotes every open stock position and stamps what the new price is as of. Requests are paced to stay inside the provider's rate limit, so a large portfolio takes a moment. Which vendor quotes is set by QUOTE_PROVIDER."
          action="Refresh Prices"
          pendingLabel="Quoting…"
          mutation={prices}
          lastRun={logs.get(UTILITY_REFRESH_PRICES)}
        >
          {(result) => (
            <div className="space-y-5">
              <StatRow
                stats={[
                  { label: 'Considered', value: result.considered },
                  { label: 'Priced', value: result.priced, tone: 'good' },
                  { label: 'No Quote', value: result.not_found },
                  { label: 'Failed', value: result.failed, tone: 'bad' },
                ]}
              />
              <QuoteLog result={result} />
            </div>
          )}
        </UtilityCard>

        <UtilityCard
          icon="business"
          title="Backfill Company Profiles"
          description="Looks up sector, industry and company name for holdings that don't have them yet — ones added before a market-data key was configured, or while the provider was unreachable."
          action="Backfill Profiles"
          pendingLabel="Looking up…"
          mutation={profiles}
          lastRun={logs.get(UTILITY_BACKFILL_PROFILES)}
        >
          {(result) => (
            <StatRow
              stats={[
                { label: 'Considered', value: result.considered },
                { label: 'Enriched', value: result.enriched, tone: 'good' },
                { label: 'Not Found', value: result.not_found },
                { label: 'Failed', value: result.failed, tone: 'bad' },
              ]}
            />
          )}
        </UtilityCard>

        <UtilityCard
          icon="calculate"
          title="Recalculate Holdings"
          description="Replays the position derivation over every holding from its transaction ledger — quantity, cost basis, gains and open/closed status. Use it after an import, or when a figure looks stale."
          action="Recalculate"
          pendingLabel="Recalculating…"
          mutation={recalc}
          lastRun={logs.get(UTILITY_RECALCULATE)}
        >
          {(result) => (
            <StatRow
              stats={[
                { label: 'Holdings', value: result.holdings },
                { label: 'Open', value: result.open },
                { label: 'Closed', value: result.closed },
              ]}
            />
          )}
        </UtilityCard>
      </div>
    </div>
  )
}
