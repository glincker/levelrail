import { useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react'
import {
  DatabaseIcon,
  HardDrivesIcon,
  StackIcon,
} from '@phosphor-icons/react/dist/ssr'
import type {
  ProjectTopologyEdge,
  ProjectTopologyGraph,
} from '../../types/projectTopology'
import {
  EDGE_DASH,
  EDGE_LABEL,
  groupIntoColumns,
} from '../../lib/projectTopologyLayout'
import { ProjectTopologyResourceCard } from './ProjectTopologyResourceCard'

interface LineSpec {
  id: string
  x1: number
  y1: number
  x2: number
  y2: number
  kind: ProjectTopologyEdge['kind']
}

type Anchor = HTMLAnchorElement | HTMLDivElement

// The connector-line overlay: an absolutely-positioned <svg> drawn from
// measured DOM anchor points, not computed graph coordinates. Same
// deliberate choice components/network/NetworkTopologyView.tsx already
// made for the whole-mesh topology page (see that file's own doc
// comment): no graph library dependency, the resource counts a project
// deals with don't warrant one, and this keeps both topology pages
// visually and structurally consistent.
export function ProjectTopologyView({
  graph,
}: {
  graph: ProjectTopologyGraph
}) {
  const columns = useMemo(() => groupIntoColumns(graph), [graph])
  const containerRef = useRef<HTMLDivElement>(null)
  const anchors = useRef(new Map<string, Anchor>())
  const [lines, setLines] = useState<LineSpec[]>([])
  const [canvasSize, setCanvasSize] = useState({ width: 0, height: 0 })

  const registerAnchor = useCallback(
    (id: string) => (el: Anchor | null) => {
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
    for (const edge of graph.edges) {
      const from = anchors.current.get(edge.from)
      const to = anchors.current.get(edge.to)
      if (!from || !to) continue
      const a = from.getBoundingClientRect()
      const b = to.getBoundingClientRect()
      next.push({
        id: `${edge.from}>${edge.to}:${edge.kind}`,
        x1: a.left + a.width / 2 - bounds.left,
        y1: a.top + a.height / 2 - bounds.top,
        x2: b.left + b.width / 2 - bounds.left,
        y2: b.top + b.height / 2 - bounds.top,
        kind: edge.kind,
      })
    }
    setLines(next)
  }, [graph.edges])

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
  }, [recomputeLines, columns])

  return (
    <div className="space-y-3">
      <TopologyLegend
        edgeKinds={[...new Set(graph.edges.map((e) => e.kind))]}
      />
      <div ref={containerRef} className="relative">
        <svg
          className="pointer-events-none absolute top-0 left-0 z-0"
          width={canvasSize.width}
          height={canvasSize.height}
          aria-hidden="true"
        >
          {lines.map((line) => (
            <path
              key={line.id}
              d={`M ${line.x1} ${line.y1} C ${(line.x1 + line.x2) / 2} ${line.y1}, ${(line.x1 + line.x2) / 2} ${line.y2}, ${line.x2} ${line.y2}`}
              fill="none"
              className="stroke-tone-accent-border"
              strokeWidth={1.5}
              strokeDasharray={EDGE_DASH[line.kind]}
            />
          ))}
        </svg>
        <div className="relative z-10 flex flex-wrap items-start gap-6">
          {columns.map((column) => (
            <div
              key={column.kind}
              className="flex min-w-[14rem] flex-1 flex-col gap-3 rounded-xl border border-dashed border-border bg-muted/20 p-3"
            >
              <span className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
                <ColumnIcon kind={column.kind} />
                {column.title}
              </span>
              <div className="flex flex-col gap-2">
                {column.nodes.map((node) => (
                  <ProjectTopologyResourceCard
                    key={node.id}
                    node={node}
                    anchorRef={registerAnchor(node.id)}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function ColumnIcon({ kind }: { kind: 'app' | 'database' | 'volume' }) {
  const className = 'size-4 text-muted-foreground'
  if (kind === 'database')
    return <DatabaseIcon className={className} aria-hidden="true" />
  if (kind === 'volume')
    return <HardDrivesIcon className={className} aria-hidden="true" />
  return <StackIcon className={className} aria-hidden="true" />
}

function TopologyLegend({
  edgeKinds,
}: {
  edgeKinds: ProjectTopologyEdge['kind'][]
}) {
  if (edgeKinds.length === 0) {
    return null
  }
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-lg border border-border bg-card/50 px-3 py-2 text-xs text-muted-foreground">
      {edgeKinds.map((kind) => (
        <span key={kind} className="flex items-center gap-1">
          <svg width="16" height="10" aria-hidden="true">
            <line
              x1="0"
              y1="5"
              x2="16"
              y2="5"
              className="stroke-tone-accent-border"
              strokeWidth={1.5}
              strokeDasharray={EDGE_DASH[kind]}
            />
          </svg>
          {EDGE_LABEL[kind]}
        </span>
      ))}
    </div>
  )
}
