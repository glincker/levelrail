import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { GlobeIcon } from '@phosphor-icons/react/dist/ssr'
import { MetricChartCard } from './MetricChartCard'
import { Skeleton } from './ui/skeleton'
import { useRequestSeries } from '../queries/requests'
import type { RequestPoint } from '../types/requests'
import type { ChartMarker, ChartRow } from '../lib/metricChart'
import { attachPrevious, capRows, MAX_CHART_POINTS } from '../lib/chartSeries'
import { chartPalette } from '../lib/chartPalette'
import type { ResolvedTimeRange } from '../lib/timeRange'

// Ingress-derived request charts (rate, 4xx/5xx error rate, latency
// percentiles). Zero-config: measured at the Caddy ingress.

type Pick = (p: RequestPoint) => number

function toRows(
  points: RequestPoint[],
  previous: RequestPoint[] | undefined,
  primary: Pick,
  secondary?: Pick,
  tertiary?: Pick,
  previousPick?: Pick,
): ChartRow[] {
  const rows: ChartRow[] = points.map((p) => ({
    t: Date.parse(p.timestamp),
    primary: primary(p),
    secondary: secondary ? secondary(p) : undefined,
    tertiary: tertiary ? tertiary(p) : undefined,
  }))
  const pick = previousPick ?? primary
  const prev = previous?.map((p) => ({
    timestamp: p.timestamp,
    value: pick(p),
  }))
  return capRows(attachPrevious(rows, prev), MAX_CHART_POINTS)
}

export function RequestMetricsSection({
  appName,
  range,
  markers,
  compare = false,
  selectedAt,
  onSelectTime,
  onMarkerClick,
}: {
  appName: string
  range: ResolvedTimeRange
  markers: ChartMarker[]
  compare?: boolean
  selectedAt?: number
  onSelectTime?: (t: number) => void
  onMarkerClick?: (marker: ChartMarker) => void
}) {
  const { t } = useTranslation('observability')
  const { data, isLoading, error } = useRequestSeries(appName, {
    from: range.from,
    to: range.to,
    maxPoints: MAX_CHART_POINTS,
    compare,
  })
  const points = useMemo(() => data?.points ?? [], [data])
  const previous = data?.previous_points

  const rateRows = useMemo(
    () => toRows(points, previous, (p) => p.rate_per_sec),
    [points, previous],
  )
  const errorRows = useMemo(
    () =>
      toRows(
        points,
        previous,
        (p) => p.error_rate_4xx * 100,
        (p) => p.error_rate_5xx * 100,
        undefined,
        (p) => p.error_rate_5xx * 100,
      ),
    [points, previous],
  )
  const latencyRows = useMemo(
    () =>
      toRows(
        points,
        previous,
        (p) => p.p50_ms,
        (p) => p.p95_ms,
        (p) => p.p99_ms,
        (p) => p.p95_ms,
      ),
    [points, previous],
  )

  if (isLoading) {
    return <Skeleton className="mt-4 h-24 w-full" />
  }
  if (error) {
    return (
      <p className="mt-4 text-sm text-destructive">
        {t('requests.unavailable', { message: error.message })}
      </p>
    )
  }
  if (points.length === 0) {
    return (
      <div className="mt-4 flex items-start gap-2 rounded-lg border border-dashed border-border p-4 text-sm text-muted-foreground">
        <GlobeIcon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
        <p>{t('requests.empty')}</p>
      </div>
    )
  }

  const summary = data?.summary
  const common = {
    range,
    markers,
    isLoading: false,
    showPrevious: compare && Boolean(previous?.length),
    previousLabel: t('compare.previousPeriod'),
    selectedAt,
    onSelectTime,
    onMarkerClick,
  }
  return (
    <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
      <MetricChartCard
        {...common}
        title={t('charts.requestRate')}
        subtitle={t('charts.requestRateHint')}
        unit="rate"
        primaryLabel={t('charts.requests')}
        primaryColor={chartPalette.primary}
        rows={rateRows}
      />
      <MetricChartCard
        {...common}
        title={t('charts.errorRate')}
        subtitle={t('charts.errorRateHint')}
        unit="percent"
        primaryLabel="4xx"
        primaryColor={chartPalette.warning}
        secondaryLabel="5xx"
        secondaryColor={chartPalette.danger}
        rows={errorRows}
        previousLabel={t('compare.previous5xx')}
      />
      <MetricChartCard
        {...common}
        title={t('charts.latency')}
        subtitle={
          summary
            ? t('charts.latencyHint', { p95: Math.round(summary.p95_ms) })
            : undefined
        }
        unit="ms"
        primaryLabel="p50"
        primaryColor={chartPalette.primary}
        secondaryLabel="p95"
        secondaryColor={chartPalette.warning}
        tertiaryLabel="p99"
        tertiaryColor={chartPalette.danger}
        rows={latencyRows}
        previousLabel={t('compare.previousP95')}
      />
    </div>
  )
}
