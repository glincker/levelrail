import { useMemo } from 'react'
import { GlobeIcon } from '@phosphor-icons/react/dist/ssr'
import { MetricChartCard } from './MetricChartCard'
import { Skeleton } from './ui/skeleton'
import { useRequestSeries } from '../queries/requests'
import type { RequestPoint } from '../types/requests'
import type { ChartMarker, ChartRow } from '../lib/metricChart'
import type { ResolvedTimeRange } from '../lib/timeRange'

// Ingress-derived request charts (rate, 4xx/5xx error rate, latency
// percentiles). Zero-config: no app changes, measured at the Caddy ingress.

const COLORS = {
  rate: '#0ea5e9',
  e4xx: '#f59e0b',
  e5xx: '#ef4444',
  p95: '#a855f7',
  p99: '#ef4444',
}

function toRows(
  points: RequestPoint[],
  primary: (p: RequestPoint) => number,
  secondary?: (p: RequestPoint) => number,
): ChartRow[] {
  return points.map((p) => ({
    t: Date.parse(p.timestamp),
    primary: primary(p),
    secondary: secondary ? secondary(p) : undefined,
  }))
}

export function RequestMetricsSection({
  appName,
  range,
  markers,
}: {
  appName: string
  range: ResolvedTimeRange
  markers: ChartMarker[]
}) {
  const { data, isLoading, error } = useRequestSeries(appName, range)
  const points = useMemo(() => data?.points ?? [], [data])

  const rateRows = useMemo(
    () => toRows(points, (p) => p.rate_per_sec),
    [points],
  )
  const errorRows = useMemo(
    () =>
      toRows(
        points,
        (p) => p.error_rate_4xx * 100,
        (p) => p.error_rate_5xx * 100,
      ),
    [points],
  )
  const latencyRows = useMemo(
    () =>
      toRows(
        points,
        (p) => p.p95_ms,
        (p) => p.p99_ms,
      ),
    [points],
  )

  if (isLoading) {
    return <Skeleton className="mt-4 h-24 w-full" />
  }
  if (error) {
    return (
      <p className="mt-4 text-sm text-destructive">
        Request metrics unavailable: {error.message}
      </p>
    )
  }
  if (points.length === 0) {
    return (
      <div className="mt-4 flex items-start gap-2 rounded-lg border border-dashed border-border p-4 text-sm text-muted-foreground">
        <GlobeIcon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
        <p>
          No request traffic in this range. Request rate, error rate and latency
          appear here once the app receives requests through its domain.
        </p>
      </div>
    )
  }

  const summary = data?.summary
  return (
    <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
      <MetricChartCard
        title="Request rate"
        subtitle="Requests per second at the ingress"
        unit="rate"
        primaryLabel="Requests"
        primaryColor={COLORS.rate}
        rows={rateRows}
        range={range}
        markers={markers}
        isLoading={false}
      />
      <MetricChartCard
        title="Error rate"
        subtitle="Share of requests answered with 4xx and 5xx"
        unit="percent"
        primaryLabel="4xx"
        primaryColor={COLORS.e4xx}
        secondaryLabel="5xx"
        secondaryColor={COLORS.e5xx}
        rows={errorRows}
        range={range}
        markers={markers}
        isLoading={false}
      />
      <MetricChartCard
        title="Latency"
        subtitle={
          summary
            ? `p95 ${Math.round(summary.p95_ms)} ms over this range`
            : undefined
        }
        unit="ms"
        primaryLabel="p95"
        primaryColor={COLORS.p95}
        secondaryLabel="p99"
        secondaryColor={COLORS.p99}
        rows={latencyRows}
        range={range}
        markers={markers}
        isLoading={false}
      />
    </div>
  )
}
