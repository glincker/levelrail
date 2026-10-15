import { useEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import {
  BAR_CLASS,
  barTone,
  type Lane,
  type TimelineAlert,
  type TimelineDeploy,
  type TimelineZoom,
} from '../../lib/deployTimeline'
import {
  adaptiveTicks,
  cardPlacement,
  layoutLane,
  spreadOffset,
  type AxisTick,
  type MarkerItem,
} from '../../lib/timelineLayout'
import { TimelineHoverCard, type CardAnchor } from './TimelineHoverCard'
import {
  AlertMarker,
  BarMarker,
  ClusterChip,
  ClusterCollapse,
  type ActiveHandler,
} from './TimelineMarkers'

const ROW_HEIGHT = 40
const OVERSCAN = 8
const EDGE_PCT = 6

function tickLabel(tick: AxisTick, zoom: TimelineZoom, first: boolean): string {
  const d = new Date(tick.at)
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  if (zoom === '1h') return time
  const date = d.toLocaleDateString([], { month: 'short', day: 'numeric' })
  if (tick.major) return date
  return first ? `${date} ${time}` : time
}

function tickShift(left: number): string {
  if (left < EDGE_PCT) return 'translate-x-0'
  if (left > 100 - EDGE_PCT) return '-translate-x-full'
  return '-translate-x-1/2'
}

interface Props {
  lanes: Lane[]
  alerts: TimelineAlert[]
  zoom: TimelineZoom
  windowStart: number
  windowEnd: number
  projectNames?: ReadonlyMap<string, string>
}

export function TimelineLanes({
  lanes,
  alerts,
  zoom,
  windowStart,
  windowEnd,
  projectNames,
}: Props) {
  const { t } = useTranslation('deployTimeline')
  const scrollRef = useRef<HTMLDivElement>(null)
  const [anchor, setAnchor] = useState<CardAnchor | null>(null)
  const [expanded, setExpanded] = useState<string | null>(null)
  const virtualizer = useVirtualizer({
    count: lanes.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: OVERSCAN,
  })
  const ticks = useMemo(
    () => adaptiveTicks(windowStart, windowEnd),
    [windowStart, windowEnd],
  )
  const alertsByApp = useMemo(() => {
    const m = new Map<string, TimelineAlert[]>()
    for (const a of alerts) m.set(a.app, [...(m.get(a.app) ?? []), a])
    return m
  }, [alerts])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      setAnchor(null)
      setExpanded(null)
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
    }
  }, [])

  const onActive: ActiveHandler = (d, el) => {
    setAnchor(
      d && el
        ? {
            deploy: d,
            placement: cardPlacement(el.getBoundingClientRect(), {
              width: window.innerWidth,
              height: window.innerHeight,
            }),
          }
        : null,
    )
  }

  return (
    <div
      role="group"
      aria-label={t('chart.label')}
      className="overflow-hidden rounded-lg border border-border bg-card"
    >
      <div
        ref={scrollRef}
        className="max-h-[60vh] overflow-auto"
        onScroll={() => {
          setAnchor(null)
        }}
      >
        <div className="min-w-[56rem]">
          <div className="sticky top-0 z-20 flex border-b border-border bg-card text-[11px] text-muted-foreground">
            <div className="sticky left-0 z-10 w-48 shrink-0 bg-card px-3 py-2">
              {t('chart.axisLabel')}
            </div>
            <div aria-hidden="true" className="relative h-8 flex-1">
              {ticks.map((tick, i) => (
                <span
                  key={tick.at}
                  data-tick={tick.major ? 'major' : 'minor'}
                  className={cn(
                    'absolute top-2 whitespace-nowrap',
                    tickShift(tick.left),
                    tick.major && 'font-semibold text-foreground',
                  )}
                  style={{ left: `${String(tick.left)}%` }}
                >
                  {tickLabel(tick, zoom, i === 0)}
                </span>
              ))}
              <span
                className="absolute top-2 right-0 rounded-sm bg-tone-info-solid px-1 text-[10px] font-medium text-white"
                data-testid="now-label"
              >
                {t('chart.now')}
              </span>
            </div>
          </div>
          <div
            className="relative w-full"
            style={{ height: virtualizer.getTotalSize() }}
          >
            {virtualizer.getVirtualItems().map((row) => {
              const lane = lanes[row.index]
              if (!lane) return null
              return (
                <LaneRow
                  key={lane.app}
                  lane={lane}
                  index={row.index}
                  start={row.start}
                  ticks={ticks}
                  alerts={alertsByApp.get(lane.app) ?? []}
                  windowStart={windowStart}
                  windowEnd={windowEnd}
                  projectName={projectNames}
                  activeId={anchor?.deploy.id ?? null}
                  expanded={expanded}
                  onExpand={setExpanded}
                  onActive={onActive}
                />
              )
            })}
          </div>
        </div>
      </div>
      <TimelineHoverCard anchor={anchor} />
    </div>
  )
}

function LaneRow({
  lane,
  index,
  start,
  ticks,
  alerts,
  windowStart,
  windowEnd,
  projectName,
  activeId,
  expanded,
  onExpand,
  onActive,
}: {
  lane: Lane
  index: number
  start: number
  ticks: AxisTick[]
  alerts: TimelineAlert[]
  windowStart: number
  windowEnd: number
  projectName?: ReadonlyMap<string, string>
  activeId: string | null
  expanded: string | null
  onExpand: (key: string | null) => void
  onActive: ActiveHandler
}) {
  const { t } = useTranslation('deployTimeline')
  const items = useMemo(
    () => layoutLane(lane.deploys, windowStart, windowEnd),
    [lane.deploys, windowStart, windowEnd],
  )
  const latest = lane.deploys[lane.deploys.length - 1]
  const project = latest?.project_id
    ? (projectName?.get(latest.project_id) ?? '')
    : ''
  const latestStatus = latest
    ? t(`status.${latest.status}` as 'status.ready', {
        defaultValue: latest.status,
      })
    : ''
  return (
    <div
      role="group"
      aria-label={t('chart.laneLabel', {
        app: lane.app,
        count: lane.deploys.length,
      })}
      className="absolute inset-x-0 top-0 flex items-center border-b border-border/60"
      style={{
        height: ROW_HEIGHT,
        transform: `translateY(${String(start)}px)`,
      }}
    >
      <div
        title={lane.app}
        className="sticky left-0 z-10 flex h-full w-48 shrink-0 items-center gap-2 border-r border-border/60 bg-card px-3"
      >
        {latest ? (
          <span
            role="img"
            aria-label={t('chart.latest', { status: latestStatus })}
            title={t('chart.latest', { status: latestStatus })}
            className={cn(
              'size-2 shrink-0 rounded-full',
              BAR_CLASS[barTone(latest)],
            )}
          />
        ) : null}
        <div className="min-w-0">
          <div className="truncate text-sm font-medium">{lane.app}</div>
          <div className="truncate text-[11px] text-muted-foreground">
            {project || t('chart.unknownProject')}
          </div>
        </div>
      </div>
      <div
        className={cn(
          'relative h-full flex-1',
          index % 2 === 1 && 'bg-muted/30',
        )}
      >
        {ticks.map((tick) => (
          <span
            key={tick.at}
            aria-hidden="true"
            className={cn(
              'absolute inset-y-0 w-px',
              tick.major ? 'bg-border' : 'bg-border/40',
            )}
            style={{ left: `${String(tick.left)}%` }}
          />
        ))}
        <span
          aria-hidden="true"
          className="absolute inset-y-0 right-0 w-px bg-tone-info-solid"
        />
        {items.map((item) => (
          <Marker
            key={item.kind === 'bar' ? item.deploy.id : item.key}
            item={item}
            activeId={activeId}
            expanded={expanded}
            onExpand={onExpand}
            onActive={onActive}
          />
        ))}
        {alerts.map((a) => (
          <AlertMarker
            key={a.id}
            alert={a}
            windowStart={windowStart}
            windowEnd={windowEnd}
          />
        ))}
      </div>
    </div>
  )
}

function Marker({
  item,
  activeId,
  expanded,
  onExpand,
  onActive,
}: {
  item: MarkerItem
  activeId: string | null
  expanded: string | null
  onExpand: (key: string | null) => void
  onActive: ActiveHandler
}) {
  if (item.kind === 'bar') {
    return (
      <BarMarker
        d={item.deploy}
        geo={item.geo}
        dot={item.dot}
        active={item.deploy.id === activeId}
        onActive={onActive}
      />
    )
  }
  const app = item.deploys[0]?.app ?? ''
  if (expanded !== item.key) {
    return (
      <ClusterChip
        app={app}
        count={item.deploys.length}
        center={item.center}
        hasFailure={item.deploys.some(
          (d: TimelineDeploy) => d.status === 'failed',
        )}
        expanded={false}
        onExpand={() => {
          onExpand(item.key)
        }}
      />
    )
  }
  const total = item.deploys.length + 1
  return (
    <>
      {item.deploys.map((d, i) => (
        <BarMarker
          key={d.id}
          d={d}
          geo={{ left: item.center, width: 0 }}
          dot
          centerPct={item.center}
          offsetPx={spreadOffset(i, total)}
          active={d.id === activeId}
          onActive={onActive}
        />
      ))}
      <ClusterCollapse
        center={item.center}
        index={item.deploys.length}
        total={total}
        onCollapse={() => {
          onExpand(null)
        }}
      />
    </>
  )
}
