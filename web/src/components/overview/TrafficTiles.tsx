import {
  ActivityGlassIcon,
  CheckGlassIcon,
  CircleAlertGlassIcon,
  ClockGlassIcon,
  CopyGlassIcon,
} from '@thesvg/ui-react'
import { MetricTile, SkeletonTile } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import {
  percentChange,
  useAppTraffic,
  type TrafficStats,
} from '../../queries/appTraffic'
import { formatRate } from './trafficFormat'
import {
  ERROR_RATE_DANGER,
  MIN_SAMPLE_REQUESTS,
  TRAFFIC_WINDOW_MINUTES,
} from './config'

const MIN_DELTA_PERCENT = 1

function delta(
  current: number,
  previous: number,
  goodWhen: 'up' | 'down',
  stats: TrafficStats,
) {
  const change = percentChange(current, previous, {
    current: stats.current.requests,
    previous: stats.previous.requests,
    minSample: MIN_SAMPLE_REQUESTS,
  })
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
  const rate = formatRate(current.ratePerSec)
  const count = (
    <span className="text-xs text-muted-foreground">
      {current.requests.toLocaleString()}{' '}
      {current.requests === 1 ? 'request' : 'requests'} in{' '}
      {TRAFFIC_WINDOW_MINUTES} min
      {current.requests < MIN_SAMPLE_REQUESTS ? ', low traffic' : ''}
    </span>
  )
  return (
    <>
      <MetricTile
        label="Requests"
        icon={<ActivityGlassIcon className="size-5" />}
        value={rate.value}
        unit={rate.unit}
        delta={delta(current.ratePerSec, previous.ratePerSec, 'up', stats)}
        note={count}
        series={stats.rateSeries}
        tone="accent"
        info={`Average requests per second through the ingress over ${windowLabel}.`}
      />
      <MetricTile
        label="Errors (4xx + 5xx)"
        icon={<CircleAlertGlassIcon className="size-5 text-tone-danger" />}
        value={(current.errorRate * 100).toFixed(1)}
        unit="%"
        delta={delta(current.errorRate, previous.errorRate, 'down', stats)}
        series={stats.errorSeries}
        tone={current.errorRate >= ERROR_RATE_DANGER ? 'danger' : 'success'}
        info={`Share of responses with a 4xx or 5xx status over ${windowLabel}. The Apps list counts 5xx only.`}
      />
      <MetricTile
        label={`p95 latency, last ${TRAFFIC_WINDOW_MINUTES} min`}
        icon={<ClockGlassIcon className="size-5" />}
        value={Math.round(current.p95Ms)}
        unit="ms"
        delta={delta(current.p95Ms, previous.p95Ms, 'down', stats)}
        series={stats.p95Series}
        tone="info"
        info={`95% of requests finished faster than this over ${windowLabel}, from the server's merged histogram. The Apps list uses a longer window.`}
      />
    </>
  )
}

export function TrafficTiles({
  appName,
  url,
  hideEmpty = false,
}: {
  appName: string
  url: string | null
  /** Skip the no-traffic strip when another card already says so. */
  hideEmpty?: boolean
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
  if (!data.hasTraffic && hideEmpty) return null
  if (!data.hasTraffic) {
    return (
      <div className="col-span-full flex items-center justify-between gap-3 rounded-xl border border-dashed border-border px-4 py-2 text-sm text-muted-foreground">
        <span className="flex items-center gap-2">
          <ActivityGlassIcon className="size-5 shrink-0" aria-hidden="true" />
          <span className="font-medium text-foreground">No traffic yet</span>
          <span>Send a request to see live numbers.</span>
        </span>
        {url ? (
          <Button size="sm" variant="ghost" onClick={() => copy(url)}>
            {copied ? (
              <CheckGlassIcon size="1.4em" aria-hidden="true" />
            ) : (
              <CopyGlassIcon size="1.4em" aria-hidden="true" />
            )}
            {copied ? 'Copied' : 'Copy app URL'}
          </Button>
        ) : null}
      </div>
    )
  }

  return <TrafficTilesView stats={data} />
}
