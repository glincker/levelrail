import { useTranslation } from 'react-i18next'
import { EllipsisGlassIcon } from '@thesvg/ui-react'
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
import { adminActions, unsupportedHint } from './adminActions'
import { HealthHistoryStrip } from './HealthHistoryStrip'
import { LbTrafficCell } from './LbTrafficCell'
import { StateGlyphIcon } from './StateGlyphIcon'
import {
  changedAt,
  formatShare,
  sharesAreEstimated,
  upstreamShares,
  upstreamView,
} from './rollup'

interface Props {
  upstreams: LiveUpstream[]
  algorithm: LoadBalancerAlgorithm
  history: Map<string, UpstreamHistory>
  historySupported: boolean
  onOpen: (id: string) => void
  onAdmin: (id: string, state: AdminState) => void
}

const COLS =
  'md:grid md:grid-cols-[minmax(0,1.3fr)_minmax(0,1.3fr)_120px_150px_52px_52px_168px_36px] md:items-center md:gap-3'

function LatencyCell({
  u,
  h,
  supported,
}: {
  u: LiveUpstream
  h?: UpstreamHistory
  supported: boolean
}) {
  const { t } = useTranslation('statePlaceholders')
  const values = (h?.series.latency_ms ?? []).map((p) => p.value)
  if (values.length >= 2) {
    return (
      <span className="flex items-center gap-2">
        <Sparkline
          values={values}
          tone="info"
          width={64}
          height={20}
          ariaLabel={t('lb.table.latencyTrend', { dial: u.dial })}
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
      {supported ? t('lb.table.noData') : '-'}
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
  const { t } = useTranslation('statePlaceholders')
  const shares = upstreamShares(upstreams, algorithm)
  return (
    <div className="space-y-1">
      <div
        aria-hidden="true"
        className={`hidden px-3 text-xs text-muted-foreground ${COLS}`}
      >
        <span>{t('lb.table.upstream')}</span>
        <span>{t('lb.table.status')}</span>
        <span>{t('lb.table.latency')}</span>
        <span>{t('lb.table.traffic')}</span>
        <span>{t('lb.table.conns')}</span>
        <span>{t('lb.table.share')}</span>
        <span>{t('lb.table.lastChecks')}</span>
        <span />
      </div>
      <ul
        role="list"
        aria-label={t('lb.table.label')}
        className="divide-y divide-border rounded-xl border"
      >
        {upstreams.map((u) => {
          const view = upstreamView(u)
          const name = u.dial || t('lb.table.replica', { replica: u.replica })
          const h = history.get(u.id)
          const changed = changedAt(u, h)
          const menu = adminActions(u, historySupported).map((a) => ({
            id: a.state,
            label: a.label,
            description: historySupported ? a.meaning : unsupportedHint(),
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
                  {name}
                </button>
                <div className="text-xs text-muted-foreground">
                  {u.last_check ? (
                    <>
                      {t('lb.table.checked')}{' '}
                      <RelativeTime at={u.last_check} live />
                    </>
                  ) : (
                    t('lb.table.notChecked')
                  )}
                  {changed ? (
                    <>
                      {` · ${t('lb.table.changed')} `}
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
              <LbTrafficCell u={u} />
              <span
                className="text-sm tabular-nums"
                title={t('lb.table.openConnections')}
              >
                <span className="text-xs text-muted-foreground md:hidden">
                  {t('lb.table.conns')}{' '}
                </span>
                <AnimatedNumber value={u.active_connections} />
              </span>
              <span
                className="text-sm tabular-nums"
                title={
                  sharesAreEstimated(algorithm)
                    ? t('lb.table.shareEstimated')
                    : t('lb.table.shareMeasured')
                }
              >
                {formatShare(shares.get(u.id) ?? 0, algorithm)}
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
                    aria-label={t('lb.table.actionsFor', { name })}
                  >
                    <EllipsisGlassIcon size="1.4em" />
                  </Button>
                }
                items={[
                  {
                    id: 'details',
                    label: t('lb.table.viewDetails'),
                    description: t('lb.table.viewDetailsHint'),
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
