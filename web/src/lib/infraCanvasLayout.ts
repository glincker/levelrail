import type {
  ProjectTopologyEdge,
  ProjectTopologyGraph,
  ProjectTopologyNode,
  ProjectTopologyNodeKind,
} from '../types/projectTopology'

export interface Point {
  x: number
  y: number
}

export interface Rect {
  x: number
  y: number
  width: number
  height: number
}

export interface Viewport {
  x: number
  y: number
  scale: number
}

export type CanvasPositions = Record<string, Point>
export type CanvasHealth = 'healthy' | 'deploying' | 'failed' | 'idle'

export const NODE_WIDTH = 188
export const NODE_HEIGHT = 56
export const MIN_SCALE = 0.15
export const MAX_SCALE = 2.5

const COLUMN_GAP = 96
const ROW_GAP = 28
const ROWS_PER_COLUMN = 12
const COLUMN_ORDER: ProjectTopologyNodeKind[] = [
  'domain',
  'load_balancer',
  'app',
  'database',
  'volume',
  'node',
]

// Stacks each kind into columns of at most ROWS_PER_COLUMN so a 200 node
// project spreads sideways instead of producing one endless column.
export function defaultPositions(graph: ProjectTopologyGraph): CanvasPositions {
  const out: CanvasPositions = {}
  let cursorX = 0
  for (const kind of COLUMN_ORDER) {
    const nodes = graph.nodes.filter((n) => n.kind === kind)
    nodes.forEach((n, i) => {
      const col = Math.floor(i / ROWS_PER_COLUMN)
      const row = i % ROWS_PER_COLUMN
      out[n.id] = {
        x: cursorX + col * (NODE_WIDTH + 32),
        y: row * (NODE_HEIGHT + ROW_GAP),
      }
    })
    if (nodes.length > 0) {
      const cols = Math.ceil(nodes.length / ROWS_PER_COLUMN)
      cursorX += cols * (NODE_WIDTH + 32) + COLUMN_GAP
    }
  }
  return out
}

// Saved positions win per node; nodes added since the last save fall
// back to the computed default so a new app never lands at the origin.
export function mergePositions(
  graph: ProjectTopologyGraph,
  saved: CanvasPositions,
): CanvasPositions {
  const base = defaultPositions(graph)
  for (const n of graph.nodes) {
    const s = saved[n.id]
    if (s) base[n.id] = s
  }
  return base
}

export function clampScale(scale: number): number {
  return Math.min(MAX_SCALE, Math.max(MIN_SCALE, scale))
}

// Zooms around a screen-space anchor so the point under the cursor
// stays put.
export function zoomAt(view: Viewport, nextScale: number, anchor: Point) {
  const scale = clampScale(nextScale)
  const ratio = scale / view.scale
  return {
    scale,
    x: anchor.x - (anchor.x - view.x) * ratio,
    y: anchor.y - (anchor.y - view.y) * ratio,
  }
}

export function fitViewport(
  positions: CanvasPositions,
  size: { width: number; height: number },
): Viewport {
  const pts = Object.values(positions)
  if (pts.length === 0 || size.width === 0 || size.height === 0) {
    return { x: 0, y: 0, scale: 1 }
  }
  const minX = Math.min(...pts.map((p) => p.x))
  const minY = Math.min(...pts.map((p) => p.y))
  const maxX = Math.max(...pts.map((p) => p.x)) + NODE_WIDTH
  const maxY = Math.max(...pts.map((p) => p.y)) + NODE_HEIGHT
  const pad = 48
  const scale = clampScale(
    Math.min(
      (size.width - pad * 2) / (maxX - minX),
      (size.height - pad * 2) / (maxY - minY),
      1,
    ),
  )
  return {
    scale,
    x: (size.width - (maxX - minX) * scale) / 2 - minX * scale,
    y: (size.height - (maxY - minY) * scale) / 2 - minY * scale,
  }
}

// The world-space rectangle currently on screen, padded so nodes just
// outside the edge do not pop in while panning.
export function visibleWorldRect(
  view: Viewport,
  size: { width: number; height: number },
  margin = 120,
): Rect {
  return {
    x: -view.x / view.scale - margin,
    y: -view.y / view.scale - margin,
    width: size.width / view.scale + margin * 2,
    height: size.height / view.scale + margin * 2,
  }
}

export function nodeIntersects(p: Point, rect: Rect): boolean {
  return (
    p.x + NODE_WIDTH >= rect.x &&
    p.x <= rect.x + rect.width &&
    p.y + NODE_HEIGHT >= rect.y &&
    p.y <= rect.y + rect.height
  )
}

export function healthOf(node: ProjectTopologyNode): CanvasHealth {
  const s = node.status
  if (!s) return 'idle'
  if (s.variant === 'success') return 'healthy'
  if (s.variant === 'destructive') return 'failed'
  return s.label === 'Reconciling' ? 'deploying' : 'idle'
}

export function neighbours(
  edges: ProjectTopologyEdge[],
  id: string,
): { id: string; kind: ProjectTopologyEdge['kind']; outgoing: boolean }[] {
  const out: {
    id: string
    kind: ProjectTopologyEdge['kind']
    outgoing: boolean
  }[] = []
  for (const e of edges) {
    if (e.from === id) out.push({ id: e.to, kind: e.kind, outgoing: true })
    else if (e.to === id)
      out.push({ id: e.from, kind: e.kind, outgoing: false })
  }
  return out
}

function isPoint(v: unknown): v is Point {
  if (typeof v !== 'object' || v === null) return false
  const o = v as Record<string, unknown>
  return Number.isFinite(o.x) && Number.isFinite(o.y)
}

export function parseSavedPositions(raw: string | null): CanvasPositions {
  if (!raw) return {}
  try {
    const parsed: unknown = JSON.parse(raw)
    if (typeof parsed !== 'object' || parsed === null) return {}
    const out: CanvasPositions = {}
    for (const [k, v] of Object.entries(parsed)) {
      if (isPoint(v)) out[k] = { x: v.x, y: v.y }
    }
    return out
  } catch {
    return {}
  }
}

export function positionsStorageKey(projectId: string): string {
  return `infra-canvas:${projectId}`
}
