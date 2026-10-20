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
  const latency = medianLatency(upstreams)
  const conns = upstreams.reduce((s, u) => s + u.active_connections, 0)
  const trend = aggregateLatency(history)
  return (
    <div className="grid grid-cols-3 gap-3 lg:grid-cols-1">
      <MetricTile
        label="Healthy"
        value={`${rollup.healthy}/${rollup.total}`}
        tone={rollup.tone}
      />
      <MetricTile
        label="Latency"
        value={latency ?? '-'}
        unit={latency === null ? undefined : 'ms'}
        series={trend}
        tone="info"
        info="Median of the last health check across healthy replicas."
      />
      <MetricTile
        label="Connections"
        value={conns}
        info="Open requests right now, across all replicas."
      />
    </div>
  )
}
