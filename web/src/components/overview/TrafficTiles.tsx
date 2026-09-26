import {
  CheckIcon,
  CopyIcon,
  PulseIcon,
  TimerIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { EmptyState, MetricTile, SkeletonTile } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import {
  percentChange,
  useAppTraffic,
  type TrafficStats,
} from '../../queries/appTraffic'
import { ERROR_RATE_DANGER, TRAFFIC_WINDOW_MINUTES } from './config'

const MIN_DELTA_PERCENT = 1

function formatRate(rate: number): string {
  return rate < 10 ? rate.toFixed(2) : rate.toFixed(1)
}

function delta(current: number, previous: number, goodWhen: 'up' | 'down') {
  const change = percentChange(current, previous)
  if (change === undefined || Math.abs(change) < MIN_DELTA_PERCENT) {
    return undefined
  }
  return {
    value: Math.abs(change),
    direction: change > 0 ? ('up' as const) : ('down' as const),
    goodWhen,
  }
}

export function TrafficTilesView({ stats }: { stats: TrafficStats }) {
  const { current, previous } = stats
  const windowLabel = `the last ${TRAFFIC_WINDOW_MINUTES} minutes`
  return (
    <>
      <MetricTile
        label="Requests"
        icon={<PulseIcon className="size-3.5" />}
        value={formatRate(current.ratePerSec)}
        unit="req/s"
        delta={delta(current.ratePerSec, previous.ratePerSec, 'up')}
        series={stats.rateSeries}
        tone="accent"
        info={`Average requests per second through the ingress over ${windowLabel}.`}
      />
      <MetricTile
        label="Errors"
        icon={<WarningCircleIcon className="size-3.5" />}
        value={(current.errorRate * 100).toFixed(1)}
        unit="%"
        delta={delta(current.errorRate, previous.errorRate, 'down')}
        series={stats.errorSeries}
        tone={current.errorRate >= ERROR_RATE_DANGER ? 'danger' : 'success'}
        info={`Share of responses with a 4xx or 5xx status over ${windowLabel}.`}
      />
      <MetricTile
        label="p95 latency"
        icon={<TimerIcon className="size-3.5" />}
        value={Math.round(current.p95Ms)}
        unit="ms"
        delta={delta(current.p95Ms, previous.p95Ms, 'down')}
        series={stats.p95Series}
        tone="info"
        info={`95% of requests finished faster than this over ${windowLabel}.`}
      />
    </>
  )
}

export function TrafficTiles({
  appName,
  url,
}: {
  appName: string
  url: string | null
}) {
  const { data, isPending, isError, refetch } = useAppTraffic(appName)
  const { copied, copy } = useCopyToClipboard()

  if (isPending) {
    return (
      <>
        <SkeletonTile />
        <SkeletonTile />
        <SkeletonTile />
      </>
    )
  }
  if (isError) {
    return (
      <div className="col-span-full flex items-center justify-between gap-3 rounded-xl border border-border px-4 py-3 text-sm text-muted-foreground">
        <span>Live traffic numbers are unavailable right now.</span>
        <Button size="sm" variant="ghost" onClick={() => void refetch()}>
          Retry
        </Button>
      </div>
    )
  }
  if (!data.hasTraffic) {
    return (
      <div className="col-span-full rounded-xl border border-dashed border-border">
        <EmptyState
          icon={<PulseIcon />}
          illustration="chart"
          title="No traffic yet"
          description="Send a request to see live numbers."
          action={
            url ? (
              <Button size="sm" variant="outline" onClick={() => copy(url)}>
                {copied ? (
                  <CheckIcon aria-hidden="true" />
                ) : (
                  <CopyIcon aria-hidden="true" />
                )}
                {copied ? 'Copied' : 'Copy app URL'}
              </Button>
            ) : undefined
          }
        />
      </div>
    )
  }
  return <TrafficTilesView stats={data} />
}
