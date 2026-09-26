import { DotsThreeIcon } from '@phosphor-icons/react/dist/ssr'
import {
  ActionMenu,
  AnimatedNumber,
  RelativeTime,
  Sparkline,
  StatusPill,
} from '@/components/kit'
import { Button } from '@/components/ui/button'
import type { LoadBalancerAlgorithm } from '../../queries/appLoadBalancer'
import type {
  AdminState,
  LiveUpstream,
  UpstreamHistory,
} from '../../queries/loadBalancerLive'
import { adminActions, UNSUPPORTED_HINT } from './adminActions'
import { HealthHistoryStrip } from './HealthHistoryStrip'
import { StateGlyphIcon } from './StateGlyphIcon'
import { changedAt, upstreamShares, upstreamView } from './rollup'

interface Props {
  upstreams: LiveUpstream[]
  algorithm: LoadBalancerAlgorithm
  history: Map<string, UpstreamHistory>
  historySupported: boolean
  onOpen: (id: string) => void
  onAdmin: (id: string, state: AdminState) => void
}

const COLS =
  'md:grid md:grid-cols-[minmax(0,1.3fr)_minmax(0,1.3fr)_120px_52px_52px_168px_36px] md:items-center md:gap-3'

function LatencyCell({
  u,
  h,
  supported,
}: {
  u: LiveUpstream
  h?: UpstreamHistory
  supported: boolean
}) {
  const values = (h?.series.latency_ms ?? []).map((p) => p.value)
  if (values.length >= 2) {
    return (
      <span className="flex items-center gap-2">
        <Sparkline
          values={values}
          tone="info"
          width={64}
          height={20}
          ariaLabel={`Latency over the last 30 minutes for ${u.dial}`}
        />
        <span className="text-xs tabular-nums">{u.latency_ms ?? '-'}ms</span>
      </span>
    )
  }
  if (u.latency_ms) {
    return <span className="text-xs tabular-nums">{u.latency_ms}ms</span>
  }
  return (
    <span className="text-xs text-muted-foreground">
      {supported ? 'no data yet' : '-'}
    </span>
  )
}

export function LbUpstreamTable({
  upstreams,
  algorithm,
  history,
  historySupported,
  onOpen,
  onAdmin,
}: Props) {
  const shares = upstreamShares(upstreams, algorithm)
  return (
    <div className="space-y-1">
      <div
        aria-hidden="true"
        className={`hidden px-3 text-xs text-muted-foreground ${COLS}`}
      >
        <span>Upstream</span>
        <span>Status</span>
        <span>Latency</span>
        <span>Conns</span>
        <span>Share</span>
        <span>Last 12 checks</span>
        <span />
      </div>
      <ul role="list" className="divide-y divide-border rounded-xl border">
        {upstreams.map((u) => {
          const view = upstreamView(u)
          const h = history.get(u.id)
          const changed = changedAt(u, h)
          const menu = adminActions(u, historySupported).map((a) => ({
            id: a.state,
            label: a.label,
            description: historySupported ? a.meaning : UNSUPPORTED_HINT,
            disabled: a.disabled,
            onSelect: () => onAdmin(u.id, a.state),
          }))
          return (
            <li
              key={u.id}
              data-upstream={u.id}
              className={`grid gap-2 px-3 py-3 ${COLS}`}
            >
              <div className="min-w-0">
                <button
                  type="button"
                  onClick={() => onOpen(u.id)}
                  className="block max-w-full truncate rounded font-mono text-xs font-medium outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring/60"
                >
                  {u.dial || `replica ${u.replica}`}
                </button>
                <div className="text-xs text-muted-foreground">
                  {u.last_check ? (
                    <>
                      checked <RelativeTime at={u.last_check} live />
                    </>
                  ) : (
                    'not checked yet'
                  )}
                  {changed ? (
                    <>
                      {' · changed '}
                      <RelativeTime at={changed} live />
                    </>
                  ) : null}
                </div>
              </div>
              <div className="min-w-0 space-y-1">
                <StatusPill
                  tone={view.tone}
                  label={view.label}
                  size="sm"
                  icon={<StateGlyphIcon glyph={view.glyph} />}
                />
                {view.glyph !== 'ok' ? (
                  <p
                    className="truncate text-xs text-muted-foreground"
                    title={view.reason}
                  >
                    {view.reason}
                  </p>
                ) : null}
              </div>
              <LatencyCell u={u} h={h} supported={historySupported} />
              <span className="text-sm tabular-nums" title="Open connections">
                <span className="text-xs text-muted-foreground md:hidden">
                  conns{' '}
                </span>
                <AnimatedNumber value={u.active_connections} />
              </span>
              <span className="text-sm tabular-nums" title="Share of traffic">
                {shares.get(u.id) ?? 0}%
              </span>
              {historySupported ? (
                <HealthHistoryStrip checks={h?.checks ?? []} />
              ) : (
                <span />
              )}
              <ActionMenu
                trigger={
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={`Actions for ${u.dial || `replica ${u.replica}`}`}
                  >
                    <DotsThreeIcon />
                  </Button>
                }
                items={[
                  {
                    id: 'details',
                    label: 'View details',
                    description: 'Checks, changes and actions',
                    onSelect: () => onOpen(u.id),
                  },
                  ...menu,
                ]}
              />
            </li>
          )
        })}
      </ul>
    </div>
  )
}
