import { RelativeTime, Sparkline, type Tone } from '@/components/kit'
import { cn } from '@/lib/utils'
import { errorTone, latencyTone } from '../../lib/fleetThresholds'
import type { AppRowMetrics } from './useAppRowMetrics'

const NUM_TONE: Record<Tone, string> = {
  neutral: 'text-muted-foreground',
  success: 'text-emerald-600 dark:text-emerald-400',
  warning: 'text-amber-600 dark:text-amber-400',
  danger: 'text-destructive',
  info: 'text-sky-600 dark:text-sky-400',
  accent: 'text-primary',
}

export function TrafficSpark({
  metrics,
  name,
  width = 88,
}: {
  metrics: AppRowMetrics
  name: string
  width?: number
}) {
  if (metrics.loading) {
    return (
      <span
        className="h-4 w-20 animate-pulse rounded bg-muted"
        aria-hidden="true"
      />
    )
  }
  if (!metrics.hasTraffic || metrics.spark.length < 2) {
    return <span className="text-xs text-muted-foreground/60">No traffic</span>
  }
  return (
    <Sparkline
      values={metrics.spark}
      tone="accent"
      width={width}
      height={24}
      ariaLabel={`Request rate for ${name}, last hour`}
    />
  )
}

export function P95Cell({ metrics }: { metrics: AppRowMetrics }) {
  if (!metrics.hasTraffic) {
    return <span className="text-xs text-muted-foreground/60">-</span>
  }
  return (
    <span
      title="p95 latency"
      className={cn(
        'text-xs tabular-nums',
        NUM_TONE[latencyTone(metrics.p95Ms)],
      )}
    >
      {Math.round(metrics.p95Ms)} ms
    </span>
  )
}

export function ErrorCell({ metrics }: { metrics: AppRowMetrics }) {
  if (!metrics.hasTraffic) {
    return <span className="text-xs text-muted-foreground/60">-</span>
  }
  return (
    <span
      title="5xx error rate"
      className={cn(
        'text-xs tabular-nums',
        NUM_TONE[errorTone(metrics.errorPct)],
      )}
    >
      {metrics.errorPct.toFixed(metrics.errorPct < 10 ? 1 : 0)}%
    </span>
  )
}

export function LastDeployCell({ metrics }: { metrics: AppRowMetrics }) {
  return metrics.lastDeployAt ? (
    <span className="text-xs text-muted-foreground">
      <RelativeTime at={metrics.lastDeployAt} />
    </span>
  ) : (
    <span className="text-xs text-muted-foreground/60">Never</span>
  )
}
