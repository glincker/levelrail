import { useRef } from 'react'
import { Link } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import { ArrowUUpLeftIcon, XCircleIcon } from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import { deployLogsPath } from '../../queries/deployments'
import {
  BAR_CLASS,
  axisTicks,
  barGeometry,
  barTone,
  isInFlight,
  type Lane,
  type TimelineDeploy,
  type TimelineZoom,
} from '../../lib/deployTimeline'

const ROW_HEIGHT = 40
const OVERSCAN = 8

function tickLabel(at: number, zoom: TimelineZoom): string {
  const d = new Date(at)
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  if (zoom !== '7d') return time
  return `${d.toLocaleDateString([], { month: 'short', day: 'numeric' })} ${time}`
}

interface Props {
  lanes: Lane[]
  zoom: TimelineZoom
  windowStart: number
  windowEnd: number
  activeId: string | null
  onActive: (d: TimelineDeploy | null) => void
}

export function TimelineLanes({
  lanes,
  zoom,
  windowStart,
  windowEnd,
  activeId,
  onActive,
}: Props) {
  const { t } = useTranslation('deployTimeline')
  const scrollRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: lanes.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: OVERSCAN,
  })
  const ticks = axisTicks(windowStart, windowEnd)

  return (
    <div
      role="group"
      aria-label={t('chart.label')}
      className="overflow-hidden rounded-lg border border-border bg-card"
    >
      <div
        aria-hidden="true"
        className="flex border-b border-border text-[11px] text-muted-foreground"
      >
        <div className="w-40 shrink-0 px-3 py-2">{t('chart.axisLabel')}</div>
        <div className="relative h-8 flex-1">
          {ticks.map((tick) => (
            <span
              key={tick.left}
              className="absolute top-2 -translate-x-1/2 whitespace-nowrap first:translate-x-0 last:-translate-x-full"
              style={{ left: `${String(tick.left)}%` }}
            >
              {tickLabel(tick.at, zoom)}
            </span>
          ))}
        </div>
      </div>
      <div ref={scrollRef} className="h-[60vh] overflow-auto">
        <div
          className="relative w-full"
          style={{ height: virtualizer.getTotalSize() }}
        >
          {virtualizer.getVirtualItems().map((row) => {
            const lane = lanes[row.index]
            if (!lane) return null
            return (
              <div
                key={lane.app}
                role="group"
                aria-label={t('chart.laneLabel', {
                  app: lane.app,
                  count: lane.deploys.length,
                })}
                className="absolute inset-x-0 top-0 flex items-center border-b border-border/60"
                style={{
                  height: ROW_HEIGHT,
                  transform: `translateY(${String(row.start)}px)`,
                }}
              >
                <div className="w-40 shrink-0 truncate px-3 text-sm font-medium">
                  {lane.app}
                </div>
                <div className="relative h-full flex-1">
                  {ticks.slice(1, -1).map((tick) => (
                    <span
                      key={tick.left}
                      aria-hidden="true"
                      className="absolute inset-y-0 w-px bg-border/50"
                      style={{ left: `${String(tick.left)}%` }}
                    />
                  ))}
                  {lane.deploys.map((d) => (
                    <TimelineBarLink
                      key={d.id}
                      d={d}
                      windowStart={windowStart}
                      windowEnd={windowEnd}
                      active={d.id === activeId}
                      onActive={onActive}
                    />
                  ))}
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

function TimelineBarLink({
  d,
  windowStart,
  windowEnd,
  active,
  onActive,
}: {
  d: TimelineDeploy
  windowStart: number
  windowEnd: number
  active: boolean
  onActive: (d: TimelineDeploy | null) => void
}) {
  const { t } = useTranslation('deployTimeline')
  const geo = barGeometry(d, windowStart, windowEnd)
  if (!geo) return null
  const MarkerIcon =
    d.status === 'failed'
      ? XCircleIcon
      : d.status === 'rolled_back'
        ? ArrowUUpLeftIcon
        : null
  const when = new Date(d.started_at).toLocaleString()
  return (
    <>
      <Link
        {...deployLogsPath(d)}
        aria-label={t('chart.barLabel', {
          app: d.app,
          status: t(`status.${d.status}` as 'status.ready', {
            defaultValue: d.status,
          }),
          when,
        })}
        data-status={d.status}
        onMouseEnter={() => {
          onActive(d)
        }}
        onMouseLeave={() => {
          onActive(null)
        }}
        onFocus={() => {
          onActive(d)
        }}
        onBlur={() => {
          onActive(null)
        }}
        className={cn(
          'absolute top-3 h-4 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring',
          BAR_CLASS[barTone(d)],
          isInFlight(d) && 'motion-safe:animate-pulse',
          active && 'ring-2 ring-ring',
        )}
        style={{
          left: `${String(geo.left)}%`,
          width: `${String(geo.width)}%`,
        }}
      />
      {MarkerIcon ? (
        <MarkerIcon
          aria-hidden="true"
          weight="fill"
          className={cn(
            'pointer-events-none absolute top-0 size-3 -translate-x-1/2',
            d.status === 'failed' ? 'text-tone-danger' : 'text-tone-neutral',
          )}
          style={{ left: `${String(geo.left + geo.width / 2)}%` }}
        />
      ) : null}
    </>
  )
}
