import { useTranslation } from 'react-i18next'
import { MetricTile } from '@/components/kit'
import type {
  LiveUpstream,
  UpstreamHistory,
} from '../../queries/loadBalancerLive'
import { medianLatency, type PoolRollup } from './rollup'

function aggregateLatency(
  history: Map<string, UpstreamHistory>,
): number[] | undefined {
  const all = [...history.values()].map((h) => h.series.latency_ms)
  const len = Math.min(...all.map((s) => s.length))
  if (all.length === 0 || len < 2) return undefined
  return Array.from({ length: len }, (_, i) => {
    const vals = all.map((s) => s[s.length - len + i]?.value ?? 0)
    return Math.round(vals.reduce((a, b) => a + b, 0) / vals.length)
  })
}

export function LbMetrics({
  upstreams,
  rollup,
  history,
}: {
  upstreams: LiveUpstream[]
  rollup: PoolRollup
  history: Map<string, UpstreamHistory>
}) {
  const { t } = useTranslation('statePlaceholders')
  const latency = medianLatency(upstreams)
  const conns = upstreams.reduce((s, u) => s + u.active_connections, 0)
  const trend = aggregateLatency(history)
  return (
    <div className="grid grid-cols-3 gap-3 lg:grid-cols-1">
      <MetricTile
        label={t('lb.metrics.healthy')}
        value={`${rollup.healthy}/${rollup.total}`}
        tone={rollup.tone}
      />
      <MetricTile
        label={t('lb.metrics.latency')}
        value={latency ?? '-'}
        unit={latency === null ? undefined : 'ms'}
        series={trend}
        tone="info"
        info={t('lb.metrics.latencyInfo')}
      />
      <MetricTile
        label={t('lb.metrics.connections')}
        value={conns}
        info={t('lb.metrics.connectionsInfo')}
      />
    </div>
  )
}
