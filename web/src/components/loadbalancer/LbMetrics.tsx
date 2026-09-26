import { MetricTile } from '@/components/kit'
import type {
  LiveUpstream,
  UpstreamHistory,
} from '../../queries/loadBalancerLive'
import { medianLatency, type PoolRollup } from './rollup'

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
  const first = [...history.values()].find(
    (h) => h.series.latency_ms.length > 1,
  )
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
        series={first?.series.latency_ms.map((p) => p.value)}
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
