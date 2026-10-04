import { useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react'
import {
  ArrowsSplitIcon,
  DatabaseIcon,
  HardDrivesIcon,
  StackIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { NetworkTopologyResponse } from '../../types/networkTopology'
import { groupIntoZones } from '../../lib/networkTopologyLayout'
import { TopologyZonePanel } from './TopologyZonePanel'

interface LineSpec {
  id: string
  x1: number
  y1: number
  x2: number
  y2: number
  label?: string
}

// The connector-line overlay: an absolutely-positioned <svg> the same
// size as the zone grid, drawn from measured DOM anchor points rather
// than computed graph coordinates. No graph library dependency (none was
// already in package.json, and the resource counts this page deals with
// don't warrant adding one): the zones lay out with an ordinary flex-wrap
// grid, and this overlay just connects whatever anchors land where after
// that natural layout, recomputing on resize.
export function NetworkTopologyView({
  topology,
}: {
  topology: NetworkTopologyResponse
}) {
  const zones = useMemo(() => groupIntoZones(topology), [topology])
  const containerRef = useRef<HTMLDivElement>(null)
  const anchors = useRef(new Map<string, HTMLAnchorElement>())
  const [lines, setLines] = useState<LineSpec[]>([])
  const [canvasSize, setCanvasSize] = useState({ width: 0, height: 0 })

  const registerAnchor = useCallback(
    (id: string) => (el: HTMLAnchorElement | null) => {
      if (el) {
        anchors.current.set(id, el)
      } else {
        anchors.current.delete(id)
      }
    },
    [],
  )

  const recomputeLines = useCallback(() => {
    const container = containerRef.current
    if (!container) return
    const bounds = container.getBoundingClientRect()
    setCanvasSize({
      width: container.scrollWidth,
      height: container.scrollHeight,
    })

    const next: LineSpec[] = []
    for (const c of topology.connections) {
      const from = anchors.current.get(`app:${c.app}`)
      const to = anchors.current.get(`db:${c.database}`)
      if (!from || !to) continue
      const a = from.getBoundingClientRect()
      const b = to.getBoundingClientRect()
      next.push({
        id: `${c.app}->${c.database}`,
        x1: a.left + a.width / 2 - bounds.left,
        y1: a.top + a.height / 2 - bounds.top,
        x2: b.left + b.width / 2 - bounds.left,
        y2: b.top + b.height / 2 - bounds.top,
        label: c.env_var,
      })
    }
    setLines(next)
  }, [topology.connections])

  useLayoutEffect(() => {
    recomputeLines()
    const container = containerRef.current
    if (!container || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(() => recomputeLines())
    observer.observe(container)
    window.addEventListener('resize', recomputeLines)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', recomputeLines)
    }
  }, [recomputeLines, zones])

  return (
    <div className="space-y-3">
      <TopologyLegend />
      <div ref={containerRef} className="relative">
        <svg
          className="pointer-events-none absolute top-0 left-0 z-0"
          width={canvasSize.width}
          height={canvasSize.height}
          aria-hidden="true"
        >
          {lines.map((line) => (
            <g key={line.id}>
              <path
                d={`M ${line.x1} ${line.y1} C ${(line.x1 + line.x2) / 2} ${line.y1}, ${(line.x1 + line.x2) / 2} ${line.y2}, ${line.x2} ${line.y2}`}
                fill="none"
                className="stroke-tone-accent-border"
                strokeWidth={1.5}
                strokeDasharray="4 3"
              />
            </g>
          ))}
        </svg>
        <div className="relative z-10 flex flex-wrap gap-3">
          {zones.map((zone) => (
            <TopologyZonePanel
              key={zone.key}
              zone={zone}
              registerAnchor={registerAnchor}
            />
          ))}
        </div>
      </div>
    </div>
  )
}

function TopologyLegend() {
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-lg border border-border bg-card/50 px-3 py-2 text-xs text-muted-foreground">
      <span className="flex items-center gap-1">
        <HardDrivesIcon className="size-3.5" aria-hidden="true" /> Node / zone
      </span>
      <span className="flex items-center gap-1">
        <StackIcon className="size-3.5" aria-hidden="true" /> App
      </span>
      <span className="flex items-center gap-1">
        <DatabaseIcon className="size-3.5" aria-hidden="true" /> Database
      </span>
      <span className="flex items-center gap-1">
        <ArrowsSplitIcon className="size-3.5" aria-hidden="true" /> Load
        balanced
      </span>
      <span className="flex items-center gap-1">
        <svg width="16" height="10" aria-hidden="true">
          <line
            x1="0"
            y1="5"
            x2="16"
            y2="5"
            className="stroke-tone-accent-border"
            strokeWidth={1.5}
            strokeDasharray="4 3"
          />
        </svg>
        App &rarr; database connection
      </span>
      <span className="flex items-center gap-1">
        <WarningCircleIcon
          className="size-3.5 text-amber-600 dark:text-amber-400"
          aria-hidden="true"
        />
        No mesh address (unreachable)
      </span>
    </div>
  )
}
