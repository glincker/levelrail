import type { CSSProperties } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  BellRingGlassIcon,
  CircleXGlassIcon,
  UndoGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { deployLogsPath } from '../../queries/deployments'
import { formatDurationMs } from '../../lib/deployDuration'
import {
  BAR_CLASS,
  barTone,
  isInFlight,
  type BarGeometry,
  type TimelineAlert,
  type TimelineDeploy,
} from '../../lib/deployTimeline'
import { spreadOffset } from '../../lib/timelineLayout'

export type ActiveHandler = (
  d: TimelineDeploy | null,
  el: HTMLElement | null,
) => void

const FOCUS_RING =
  'outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-card'

function useBarLabel() {
  const { t } = useTranslation('deployTimeline')
  return (d: TimelineDeploy): string => {
    const duration =
      d.duration_ms !== null
        ? (formatDurationMs(d.duration_ms) ?? '')
        : t('card.running')
    return t('chart.barLabel', {
      app: d.app,
      status: t(`status.${d.status}` as 'status.ready', {
        defaultValue: d.status,
      }),
      when: new Date(d.started_at).toLocaleString(),
      duration,
    })
  }
}

function OutcomeIcon({
  status,
  style,
}: {
  status: string
  style: CSSProperties
}) {
  const Icon =
    status === 'failed'
      ? CircleXGlassIcon
      : status === 'rolled_back'
        ? UndoGlassIcon
        : null
  if (!Icon) return null
  return (
    <Icon
      aria-hidden="true"
      className={cn(
        'pointer-events-none absolute top-0 size-3 -translate-x-1/2',
        status === 'failed' ? 'text-tone-danger' : 'text-tone-neutral',
      )}
      style={style}
    />
  )
}

export function BarMarker({
  d,
  geo,
  dot,
  offsetPx = 0,
  centerPct,
  active,
  onActive,
}: {
  d: TimelineDeploy
  geo: BarGeometry
  dot: boolean
  /** Horizontal shift in px, used when a cluster is spread out. */
  offsetPx?: number
  /** Overrides the dot anchor, used when a cluster is spread out. */
  centerPct?: number
  active: boolean
  onActive: ActiveHandler
}) {
  const barLabel = useBarLabel()
  const center = centerPct ?? geo.left + geo.width / 2
  const handlers = {
    onMouseEnter: (e: React.MouseEvent<HTMLElement>) => {
      onActive(d, e.currentTarget)
    },
    onMouseLeave: () => {
      onActive(null, null)
    },
    onFocus: (e: React.FocusEvent<HTMLElement>) => {
      onActive(d, e.currentTarget)
    },
    onBlur: () => {
      onActive(null, null)
    },
  }
  const pulse = isInFlight(d) && 'motion-safe:animate-pulse'
  return (
    <>
      {dot ? (
        <Link
          {...deployLogsPath(d)}
          aria-label={barLabel(d)}
          data-status={d.status}
          data-marker="dot"
          {...handlers}
          className={cn(
            'absolute top-2 flex size-6 -translate-x-1/2 items-center justify-center rounded-full',
            FOCUS_RING,
          )}
          style={{ left: `calc(${String(center)}% + ${String(offsetPx)}px)` }}
        >
          <span
            aria-hidden="true"
            className={cn(
              'size-2.5 rounded-full',
              BAR_CLASS[barTone(d)],
              pulse,
              active && 'ring-2 ring-ring',
            )}
          />
        </Link>
      ) : (
        <Link
          {...deployLogsPath(d)}
          aria-label={barLabel(d)}
          data-status={d.status}
          data-marker="bar"
          {...handlers}
          className={cn(
            'absolute top-3 h-4 min-w-1.5 rounded-sm',
            FOCUS_RING,
            BAR_CLASS[barTone(d)],
            pulse,
            active && 'ring-2 ring-ring',
          )}
          style={{
            left: `${String(geo.left)}%`,
            width: `${String(geo.width)}%`,
          }}
        />
      )}
      <OutcomeIcon
        status={d.status}
        style={{ left: `calc(${String(center)}% + ${String(offsetPx)}px)` }}
      />
    </>
  )
}

export function ClusterChip({
  app,
  count,
  center,
  hasFailure,
  expanded,
  onExpand,
}: {
  app: string
  count: number
  center: number
  hasFailure: boolean
  expanded: boolean
  onExpand: () => void
}) {
  const { t } = useTranslation('deployTimeline')
  return (
    <button
      type="button"
      aria-label={t('chart.clusterLabel', { app, count })}
      aria-expanded={expanded}
      data-marker="cluster"
      onClick={onExpand}
      onFocus={onExpand}
      className={cn(
        'absolute top-2.5 h-5 -translate-x-1/2 rounded-full border px-1.5 text-[11px] font-medium tabular-nums',
        FOCUS_RING,
        hasFailure
          ? 'border-tone-danger bg-tone-danger-solid text-white'
          : 'border-border bg-muted text-foreground',
      )}
      style={{ left: `${String(center)}%` }}
    >
      {t('chart.clusterChip', { count })}
    </button>
  )
}

export function ClusterCollapse({
  center,
  index,
  total,
  onCollapse,
}: {
  center: number
  index: number
  total: number
  onCollapse: () => void
}) {
  const { t } = useTranslation('deployTimeline')
  const offset = spreadOffset(index, total)
  return (
    <button
      type="button"
      aria-label={t('chart.collapse')}
      onClick={onCollapse}
      className={cn(
        'absolute top-3 flex size-4 -translate-x-1/2 items-center justify-center rounded-full bg-muted text-muted-foreground',
        FOCUS_RING,
      )}
      style={{ left: `calc(${String(center)}% + ${String(offset)}px)` }}
    >
      <XGlassIcon aria-hidden="true" className="size-2.5" />
    </button>
  )
}

export function AlertMarker({
  alert,
  windowStart,
  windowEnd,
}: {
  alert: TimelineAlert
  windowStart: number
  windowEnd: number
}) {
  const { t } = useTranslation('deployTimeline')
  const at = new Date(alert.at).getTime()
  if (at < windowStart || at > windowEnd) return null
  const left = ((at - windowStart) / (windowEnd - windowStart)) * 100
  const label = t('chart.alertLabel', {
    rule: alert.rule_name,
    when: new Date(alert.at).toLocaleString(),
  })
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className="absolute bottom-0.5 -translate-x-1/2 text-tone-warning"
      style={{ left: `${String(left)}%` }}
    >
      <BellRingGlassIcon aria-hidden="true" className="size-3.5" />
    </span>
  )
}
