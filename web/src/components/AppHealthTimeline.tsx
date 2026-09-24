import { useMemo, useState, type KeyboardEvent } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { HeartbeatIcon } from '@phosphor-icons/react/dist/ssr'
import { deployAttemptsQueryOptions } from '../queries/deployAttempts'
import { useMetricSeries } from '../queries/metrics'
import { resolveTimeRange, type TimeRangeKey } from '../lib/timeRange'
import {
  buildHealthTimeline,
  formatTimelineTime,
  type HealthTimeline,
} from '../lib/healthTimeline'

const RANGES: { key: TimeRangeKey; label: string }[] = [
  { key: '24h', label: '24h' },
  { key: '7d', label: '7d' },
]

const VIEW_W = 1000
const VIEW_H = 76

const DEPLOY_FILL: Record<string, string> = {
  succeeded: 'fill-emerald-500',
  running: 'fill-sky-500',
  failed: 'fill-destructive',
}

function activateOnKey(action: () => void) {
  return (e: KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      action()
    }
  }
}

interface StripProps {
  timeline: HealthTimeline
  onHover: (text: string | null) => void
  onOpenDeploy: (id: string) => void
  onOpenLogs: () => void
}

function TimelineStrip({
  timeline,
  onHover,
  onOpenDeploy,
  onOpenLogs,
}: StripProps) {
  const x = (pos: number) => pos * VIEW_W
  return (
    <svg
      viewBox={`0 0 ${VIEW_W} ${VIEW_H}`}
      className="h-auto w-full"
      role="group"
      aria-label="Health timeline"
    >
      <line
        x1={0}
        x2={VIEW_W}
        y1={VIEW_H / 2}
        y2={VIEW_H / 2}
        className="stroke-border"
        strokeWidth={1}
      />
      {timeline.windows.map((w) => (
        <rect
          key={w.key}
          x={x(w.startPos)}
          y={4}
          width={Math.max(6, x(w.endPos) - x(w.startPos))}
          height={VIEW_H - 8}
          rx={3}
          className="fill-destructive/15"
          aria-hidden="true"
          onMouseEnter={() => onHover(w.label)}
          onMouseLeave={() => onHover(null)}
        >
          <title>{w.label}</title>
        </rect>
      ))}
      {timeline.restarts.map((r) => (
        <g
          key={r.key}
          role="link"
          tabIndex={0}
          aria-label={`${r.label}. Open logs.`}
          className="group cursor-pointer outline-none"
          onClick={onOpenLogs}
          onKeyDown={activateOnKey(onOpenLogs)}
          onMouseEnter={() => onHover(r.label)}
          onMouseLeave={() => onHover(null)}
          onFocus={() => onHover(r.label)}
          onBlur={() => onHover(null)}
        >
          <title>{r.label}</title>
          <rect
            x={x(r.pos) - 9}
            y={VIEW_H - 26}
            width={18}
            height={18}
            fill="transparent"
          />
          <path
            d={`M ${x(r.pos)} ${VIEW_H - 24} l 6 6 l -6 6 l -6 -6 Z`}
            className="fill-amber-500 stroke-transparent group-focus-visible:stroke-foreground motion-safe:transition-colors"
            strokeWidth={2}
          />
        </g>
      ))}
      {timeline.deploys.map((d) => (
        <g
          key={d.id}
          role="link"
          tabIndex={0}
          aria-label={`${d.label}. Open deploy logs.`}
          className="group cursor-pointer outline-none"
          onClick={() => onOpenDeploy(d.id)}
          onKeyDown={activateOnKey(() => onOpenDeploy(d.id))}
          onMouseEnter={() => onHover(d.label)}
          onMouseLeave={() => onHover(null)}
          onFocus={() => onHover(d.label)}
          onBlur={() => onHover(null)}
        >
          <title>{d.label}</title>
          <circle cx={x(d.pos)} cy={22} r={12} fill="transparent" />
          <circle
            cx={x(d.pos)}
            cy={22}
            r={7}
            className={`${DEPLOY_FILL[d.status] ?? 'fill-muted-foreground'} stroke-transparent group-focus-visible:stroke-foreground motion-safe:transition-colors`}
            strokeWidth={2}
          />
        </g>
      ))}
    </svg>
  )
}

function Legend() {
  return (
    <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
      <li className="flex items-center gap-1.5">
        <span
          className="size-2.5 rounded-full bg-emerald-500"
          aria-hidden="true"
        />
        Deploy succeeded
      </li>
      <li className="flex items-center gap-1.5">
        <span
          className="size-2.5 rounded-full bg-destructive"
          aria-hidden="true"
        />
        Deploy failed
      </li>
      <li className="flex items-center gap-1.5">
        <span className="size-2.5 rotate-45 bg-amber-500" aria-hidden="true" />
        Container restarts
      </li>
      <li className="flex items-center gap-1.5">
        <span
          className="h-2.5 w-4 rounded-sm bg-destructive/20"
          aria-hidden="true"
        />
        Error window (failed deploy or crashloop)
      </li>
    </ul>
  )
}

export function AppHealthTimeline({ appName }: { appName: string }) {
  const navigate = useNavigate()
  const [rangeKey, setRangeKey] = useState<TimeRangeKey>('24h')
  const [hovered, setHovered] = useState<string | null>(null)
  const range = useMemo(() => resolveTimeRange(rangeKey), [rangeKey])

  const attemptsQuery = useQuery(deployAttemptsQueryOptions(appName))
  const restartsQuery = useMetricSeries(appName, 'container_restart_count', {
    from: range.from,
    to: range.to,
  })

  const timeline = useMemo(
    () =>
      buildHealthTimeline({
        attempts: attemptsQuery.data ?? [],
        restartTimes: (restartsQuery.data?.points ?? []).map((p) =>
          Date.parse(p.timestamp),
        ),
        from: range.from.getTime(),
        to: range.to.getTime(),
      }),
    [attemptsQuery.data, restartsQuery.data, range],
  )

  const isLoading = attemptsQuery.isPending
  const hasError = attemptsQuery.isError
  const isEmpty =
    timeline.deploys.length === 0 &&
    timeline.restarts.length === 0 &&
    timeline.windows.length === 0
  const restartsUnavailable = restartsQuery.isError

  const openDeploy = (deployId: string) =>
    void navigate({
      to: '/apps/$name/deploys/$deployId/logs',
      params: { name: appName, deployId },
    })
  const openLogs = () =>
    void navigate({ to: '/apps/$name/logs', params: { name: appName } })

  let body
  if (isLoading) {
    body = (
      <p className="py-6 text-center text-sm text-muted-foreground">
        Loading timeline...
      </p>
    )
  } else if (hasError) {
    body = (
      <p role="alert" className="py-6 text-center text-sm text-destructive">
        Could not load deploy history for the timeline.
      </p>
    )
  } else if (isEmpty) {
    body = (
      <p className="py-6 text-center text-sm text-muted-foreground">
        No deploys, restarts, or errors in this window.
      </p>
    )
  } else {
    body = (
      <>
        <TimelineStrip
          timeline={timeline}
          onHover={setHovered}
          onOpenDeploy={openDeploy}
          onOpenLogs={openLogs}
        />
        <div className="flex justify-between text-xs text-muted-foreground">
          <span>{formatTimelineTime(range.from.getTime())}</span>
          <span>now</span>
        </div>
        <p
          className="min-h-5 text-xs text-foreground"
          aria-live="polite"
          data-testid="timeline-caption"
        >
          {hovered ?? 'Hover or focus a marker for details. Enter opens it.'}
        </p>
      </>
    )
  }

  return (
    <section
      className="space-y-3 rounded-lg border border-border p-4"
      aria-label="App health timeline"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
          <HeartbeatIcon
            className="size-4 text-muted-foreground"
            aria-hidden="true"
          />
          Health timeline
        </h2>
        <div
          role="group"
          aria-label="Timeline range"
          className="inline-flex rounded-md border border-border"
        >
          {RANGES.map((r) => (
            <button
              key={r.key}
              type="button"
              aria-pressed={rangeKey === r.key}
              onClick={() => setRangeKey(r.key)}
              className={`px-2.5 py-1 text-xs font-medium first:rounded-l-md last:rounded-r-md ${
                rangeKey === r.key
                  ? 'bg-foreground text-background'
                  : 'bg-transparent text-muted-foreground hover:bg-muted hover:text-foreground'
              }`}
            >
              {r.label}
            </button>
          ))}
        </div>
      </div>
      {body}
      <Legend />
      {restartsUnavailable ? (
        <p className="text-xs text-muted-foreground">
          Restart data is unavailable (telemetry may not be configured), so only
          deploys are shown.
        </p>
      ) : null}
    </section>
  )
}
