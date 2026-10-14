import { memo } from 'react'
import type { ProjectTopologyEdge } from '../../types/projectTopology'
import {
  type CanvasPositions,
  NODE_HEIGHT,
  NODE_WIDTH,
} from '../../lib/infraCanvasLayout'

const DASH: Record<ProjectTopologyEdge['kind'], string | undefined> = {
  database_binding: undefined,
  depends_on: '6 4',
  volume_attachment: '2 4',
  egress_allow: '3 3',
  routing: undefined,
}

const STROKE: Record<ProjectTopologyEdge['kind'], string> = {
  database_binding: 'stroke-foreground/50',
  depends_on: 'stroke-muted-foreground',
  volume_attachment: 'stroke-muted-foreground/70',
  egress_allow: 'stroke-muted-foreground/70',
  routing: 'stroke-ring',
}

interface Props {
  edges: ProjectTopologyEdge[]
  positions: CanvasPositions
  visible: Set<string>
  highlighted: string | null
}

// One SVG inside the transformed layer. Edges with both ends offscreen
// are skipped, which is what keeps a 200 node project cheap to repaint.
function InfraCanvasEdgesBase({
  edges,
  positions,
  visible,
  highlighted,
}: Props) {
  return (
    <svg
      className="pointer-events-none absolute left-0 top-0 overflow-visible"
      width="1"
      height="1"
      aria-hidden="true"
    >
      {edges.map((e) => {
        if (!visible.has(e.from) && !visible.has(e.to)) return null
        const a = positions[e.from]
        const b = positions[e.to]
        if (!a || !b) return null
        const ltr = a.x <= b.x
        const x1 = a.x + (ltr ? NODE_WIDTH : 0)
        const x2 = b.x + (ltr ? 0 : NODE_WIDTH)
        const y1 = a.y + NODE_HEIGHT / 2
        const y2 = b.y + NODE_HEIGHT / 2
        const bend = Math.max(40, Math.abs(x2 - x1) / 2)
        const dir = ltr ? 1 : -1
        const d = `M${x1},${y1} C${x1 + bend * dir},${y1} ${x2 - bend * dir},${y2} ${x2},${y2}`
        const active = highlighted === e.from || highlighted === e.to
        return (
          <path
            key={`${e.from}>${e.to}:${e.kind}`}
            d={d}
            fill="none"
            strokeWidth={active ? 2.5 : 1.5}
            strokeDasharray={DASH[e.kind]}
            className={active ? 'stroke-ring' : STROKE[e.kind]}
          />
        )
      })}
    </svg>
  )
}

export const InfraCanvasEdges = memo(InfraCanvasEdgesBase)
