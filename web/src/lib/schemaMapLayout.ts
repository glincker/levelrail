// Deterministic layout for the schema map: connected components, BFS layers,
// sorted slots. No randomness so a reload draws the same picture.

export const NODE_WIDTH = 224
export const HEADER_HEIGHT = 28
export const ROW_HEIGHT = 18
export const MAX_COLUMNS = 8
const GAP_X = 96
const GAP_Y = 28
const GROUP_GAP = 56
const SINGLE_GAP = 24
const PAD = 24

export interface MapColumn {
  name: string
  type: string
  primaryKey: boolean
}

export interface MapTable {
  id: string
  schema: string
  name: string
  columns: MapColumn[]
}

export interface MapEdge {
  id: string
  from: string
  to: string
  // Column in the source table that holds the key; may be absent.
  fromColumn?: string
}

export interface PlacedNode {
  id: string
  x: number
  y: number
  w: number
  h: number
  shown: MapColumn[]
  more: number
}

export interface MapLayout {
  nodes: PlacedNode[]
  width: number
  height: number
}

export function nodeHeight(columnCount: number): number {
  const shown = Math.min(columnCount, MAX_COLUMNS)
  const more = columnCount > MAX_COLUMNS ? 1 : 0
  return HEADER_HEIGHT + (shown + more) * ROW_HEIGHT + 8
}

// Primary keys first so they survive the column cap, then table order.
function visibleColumns(cols: MapColumn[]): MapColumn[] {
  const pks = cols.filter((c) => c.primaryKey)
  const rest = cols.filter((c) => !c.primaryKey)
  return [...pks, ...rest].slice(0, MAX_COLUMNS)
}

function cmp(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

export function layoutSchemaMap(
  tables: MapTable[],
  edges: MapEdge[],
): MapLayout {
  const byId = new Map(tables.map((t) => [t.id, t]))
  const adj = new Map<string, Set<string>>()
  for (const t of tables) adj.set(t.id, new Set())
  for (const e of edges) {
    if (e.from === e.to || !byId.has(e.from) || !byId.has(e.to)) continue
    adj.get(e.from)?.add(e.to)
    adj.get(e.to)?.add(e.from)
  }

  const ids = [...byId.keys()].sort(cmp)
  const seen = new Set<string>()
  const groups: string[][] = []
  const singles: string[] = []
  for (const start of ids) {
    if (seen.has(start)) continue
    const members: string[] = []
    const queue = [start]
    seen.add(start)
    while (queue.length > 0) {
      const cur = queue.shift() as string
      members.push(cur)
      for (const n of [...(adj.get(cur) ?? [])].sort(cmp)) {
        if (!seen.has(n)) {
          seen.add(n)
          queue.push(n)
        }
      }
    }
    if (members.length === 1) singles.push(start)
    else groups.push(members)
  }

  const nodes: PlacedNode[] = []
  const make = (id: string, x: number, y: number): PlacedNode => {
    const cols = byId.get(id)?.columns ?? []
    const shown = visibleColumns(cols)
    return {
      id,
      x,
      y,
      w: NODE_WIDTH,
      h: nodeHeight(cols.length),
      shown,
      more: Math.max(0, cols.length - shown.length),
    }
  }

  let cursorY = PAD
  let maxX = PAD
  for (const members of groups) {
    // Root is the most connected table, ties broken by id.
    const root = [...members].sort(
      (a, b) => (adj.get(b)?.size ?? 0) - (adj.get(a)?.size ?? 0) || cmp(a, b),
    )[0]
    if (root === undefined) continue
    const layer = new Map<string, number>([[root, 0]])
    const queue = [root]
    while (queue.length > 0) {
      const cur = queue.shift() as string
      for (const n of [...(adj.get(cur) ?? [])].sort(cmp)) {
        if (!layer.has(n)) {
          layer.set(n, (layer.get(cur) ?? 0) + 1)
          queue.push(n)
        }
      }
    }
    const depth = Math.max(...layer.values())
    const columns: string[][] = Array.from({ length: depth + 1 }, () => [])
    for (const id of members) columns[layer.get(id) ?? 0]?.push(id)
    // One barycenter pass against the previous layer to cut crossings.
    const rank = new Map<string, number>()
    columns[0]?.sort(cmp).forEach((id, i) => rank.set(id, i))
    for (let l = 1; l <= depth; l++) {
      const score = (id: string): number => {
        const prev = [...(adj.get(id) ?? [])].filter(
          (n) => layer.get(n) === l - 1,
        )
        if (prev.length === 0) return 0
        return prev.reduce((s, n) => s + (rank.get(n) ?? 0), 0) / prev.length
      }
      columns[l]?.sort((a, b) => score(a) - score(b) || cmp(a, b))
      columns[l]?.forEach((id, i) => rank.set(id, i))
    }
    let groupH = 0
    columns.forEach((col, l) => {
      let y = cursorY
      for (const id of col) {
        const n = make(id, PAD + l * (NODE_WIDTH + GAP_X), y)
        nodes.push(n)
        y += n.h + GAP_Y
        maxX = Math.max(maxX, n.x + n.w)
      }
      groupH = Math.max(groupH, y - cursorY - GAP_Y)
    })
    cursorY += groupH + GROUP_GAP
  }

  if (singles.length > 0) {
    const perRow = Math.max(1, Math.ceil(Math.sqrt(singles.length)))
    let rowY = cursorY
    let rowH = 0
    singles.forEach((id, i) => {
      const col = i % perRow
      if (i > 0 && col === 0) {
        rowY += rowH + SINGLE_GAP
        rowH = 0
      }
      const n = make(id, PAD + col * (NODE_WIDTH + SINGLE_GAP), rowY)
      nodes.push(n)
      rowH = Math.max(rowH, n.h)
      maxX = Math.max(maxX, n.x + n.w)
    })
    cursorY = rowY + rowH + GROUP_GAP
  }

  return {
    nodes,
    width: maxX + PAD,
    height: Math.max(PAD * 2, cursorY - GROUP_GAP + PAD),
  }
}

export interface EdgeGeometry {
  d: string
  x1: number
  y1: number
}

// Anchor on the side facing the other table; a self-loop or same-column pair
// leaves from the right edge and curves back.
export function routeEdge(
  from: PlacedNode,
  to: PlacedNode,
  fromColumn?: string,
): EdgeGeometry {
  const idx = fromColumn
    ? from.shown.findIndex((c) => c.name === fromColumn)
    : -1
  const y1 =
    from.y + HEADER_HEIGHT + (idx >= 0 ? idx : 0) * ROW_HEIGHT + ROW_HEIGHT / 2
  const y2 = to.y + HEADER_HEIGHT / 2
  if (to.x >= from.x + from.w) {
    const x1 = from.x + from.w
    const x2 = to.x
    const mx = (x1 + x2) / 2
    return { d: `M${x1} ${y1} C${mx} ${y1} ${mx} ${y2} ${x2} ${y2}`, x1, y1 }
  }
  if (to.x + to.w <= from.x) {
    const x1 = from.x
    const x2 = to.x + to.w
    const mx = (x1 + x2) / 2
    return { d: `M${x1} ${y1} C${mx} ${y1} ${mx} ${y2} ${x2} ${y2}`, x1, y1 }
  }
  const x1 = from.x + from.w
  const x2 = to.x + to.w
  // Longer spans bulge further so stacked edges do not share one curve.
  const bulge = Math.max(x1, x2) + 24 + Math.min(Math.abs(y2 - y1) / 5, 72)
  return {
    d: `M${x1} ${y1} C${bulge} ${y1} ${bulge} ${y2} ${x2} ${y2}`,
    x1,
    y1,
  }
}
