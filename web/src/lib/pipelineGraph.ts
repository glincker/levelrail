import type { PipelineJob, PipelineStatus } from '../types/pipelines'
import { baseJobName } from './pipelineStatus'

export const NODE_WIDTH = 212
export const COLUMN_GAP = 56
export const ROW_GAP = 14
export const SINGLE_HEIGHT = 40
export const GROUP_HEADER_HEIGHT = 28
export const MEMBER_HEIGHT = 26
export const PADDING = 8

export interface GraphMember {
  key: string
  label: string
  status: PipelineStatus
  job: PipelineJob
  y: number
  height: number
}

export interface GraphNode {
  id: string
  label: string
  status: PipelineStatus
  grouped: boolean
  column: number
  x: number
  y: number
  width: number
  height: number
  members: GraphMember[]
}

export interface GraphEdge {
  from: string
  to: string
  path: string
}

export interface GraphLayout {
  nodes: GraphNode[]
  edges: GraphEdge[]
  width: number
  height: number
}

// rollupStatus is the one status a matrix group shows: anything failed wins,
// then anything still moving, then the settled states.
export function rollupStatus(statuses: PipelineStatus[]): PipelineStatus {
  const has = (s: PipelineStatus) => statuses.includes(s)
  if (has('failed')) {
    return 'failed'
  }
  if (has('running')) {
    return 'running'
  }
  if (has('waiting_approval')) {
    return 'waiting_approval'
  }
  if (has('cancelled')) {
    return 'cancelled'
  }
  if (statuses.length > 0 && statuses.every((s) => s === 'skipped')) {
    return 'skipped'
  }
  if (
    statuses.length > 0 &&
    statuses.every((s) => s === 'succeeded' || s === 'skipped')
  ) {
    return 'succeeded'
  }
  return 'pending'
}

function memberLabel(job: PipelineJob): string {
  const i = job.key.indexOf('[')
  return i >= 0 ? job.key.slice(i + 1, job.key.length - 1) : job.key
}

// assignDepths gives each group one more than the deepest group it needs.
// A group on a dependency cycle keeps the depth found before the cycle
// closed, so a bad graph still lays out instead of looping.
export function assignDepths(
  needs: Map<string, string[]>,
): Map<string, number> {
  const depth = new Map<string, number>()
  const visiting = new Set<string>()
  const resolve = (id: string): number => {
    const known = depth.get(id)
    if (known !== undefined) {
      return known
    }
    if (visiting.has(id)) {
      return -1
    }
    visiting.add(id)
    let d = 0
    for (const n of needs.get(id) ?? []) {
      if (needs.has(n) && n !== id) {
        d = Math.max(d, resolve(n) + 1)
      }
    }
    visiting.delete(id)
    depth.set(id, d)
    return d
  }
  for (const id of needs.keys()) {
    resolve(id)
  }
  return depth
}

function edgePath(x1: number, y1: number, x2: number, y2: number): string {
  const dx = Math.max(24, (x2 - x1) / 2)
  return `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`
}

// layoutGraph places matrix-grouped jobs in columns by dependency depth and
// routes a curved edge from every needed group to the group needing it.
// It is pure so the same input always gives the same picture.
export function layoutGraph(jobs: PipelineJob[]): GraphLayout {
  const groups = new Map<string, PipelineJob[]>()
  for (const j of jobs) {
    const id = baseJobName(j.key)
    groups.set(id, [...(groups.get(id) ?? []), j])
  }
  const needs = new Map<string, string[]>()
  for (const [id, members] of groups) {
    needs.set(id, members[0]?.needs ?? [])
  }
  const depths = assignDepths(needs)

  const columns: string[][] = []
  for (const id of groups.keys()) {
    const d = depths.get(id) ?? 0
    columns[d] = [...(columns[d] ?? []), id]
  }

  const nodes = new Map<string, GraphNode>()
  const order = new Map<string, number>()
  columns.forEach((ids, column) => {
    if (!ids) {
      return
    }
    const sorted = column === 0 ? ids : sortByParents(ids, needs, order)
    let y = PADDING
    sorted.forEach((id, idx) => {
      const node = buildNode(id, groups.get(id) ?? [], column, y)
      nodes.set(id, node)
      order.set(id, idx)
      y += node.height + ROW_GAP
    })
  })

  const edges: GraphEdge[] = []
  for (const [id, node] of nodes) {
    for (const n of needs.get(id) ?? []) {
      const from = nodes.get(n)
      if (!from || n === id) {
        continue
      }
      edges.push({
        from: n,
        to: id,
        path: edgePath(
          from.x + from.width,
          from.y + Math.min(from.height, SINGLE_HEIGHT) / 2,
          node.x,
          node.y + Math.min(node.height, SINGLE_HEIGHT) / 2,
        ),
      })
    }
  }

  const list = [...nodes.values()]
  return {
    nodes: list,
    edges,
    width: list.reduce((m, n) => Math.max(m, n.x + n.width), 0) + PADDING,
    height: list.reduce((m, n) => Math.max(m, n.y + n.height), 0) + PADDING,
  }
}

function sortByParents(
  ids: string[],
  needs: Map<string, string[]>,
  order: Map<string, number>,
): string[] {
  const score = (id: string): number => {
    const parents = (needs.get(id) ?? []).filter((p) => order.has(p))
    if (parents.length === 0) {
      return Number.MAX_SAFE_INTEGER
    }
    return parents.reduce((s, p) => s + (order.get(p) ?? 0), 0) / parents.length
  }
  return ids
    .map((id, i) => ({ id, i, s: score(id) }))
    .sort((a, b) => a.s - b.s || a.i - b.i)
    .map((e) => e.id)
}

function buildNode(
  id: string,
  jobs: PipelineJob[],
  column: number,
  y: number,
): GraphNode {
  const grouped = jobs.length > 1 || jobs.some((j) => j.key.includes('['))
  const status = rollupStatus(jobs.map((j) => j.status))
  const x = PADDING + column * (NODE_WIDTH + COLUMN_GAP)
  if (!grouped) {
    const job = jobs[0]
    if (!job) {
      return {
        id,
        label: id,
        status,
        grouped,
        column,
        x,
        y,
        width: NODE_WIDTH,
        height: SINGLE_HEIGHT,
        members: [],
      }
    }
    return {
      id,
      label: id,
      status,
      grouped,
      column,
      x,
      y,
      width: NODE_WIDTH,
      height: SINGLE_HEIGHT,
      members: [
        {
          key: job.key,
          label: id,
          status: job.status,
          job,
          y: 0,
          height: SINGLE_HEIGHT,
        },
      ],
    }
  }
  return {
    id,
    label: id,
    status,
    grouped,
    column,
    x,
    y,
    width: NODE_WIDTH,
    height: GROUP_HEADER_HEIGHT + jobs.length * MEMBER_HEIGHT + 4,
    members: jobs.map((job, i) => ({
      key: job.key,
      label: memberLabel(job),
      status: job.status,
      job,
      y: GROUP_HEADER_HEIGHT + i * MEMBER_HEIGHT,
      height: MEMBER_HEIGHT,
    })),
  }
}

export type Direction = 'left' | 'right' | 'up' | 'down'

interface Placed {
  key: string
  column: number
  cy: number
}

function placedMembers(layout: GraphLayout): Placed[] {
  return layout.nodes.flatMap((n) =>
    n.members.map((m) => ({
      key: m.key,
      column: n.column,
      cy: n.y + m.y + m.height / 2,
    })),
  )
}

// neighborKey picks the job to focus after an arrow key: up and down stay
// in the column, left and right go to the nearest job in the next column
// that has any, by vertical distance.
export function neighborKey(
  layout: GraphLayout,
  from: string,
  dir: Direction,
): string | undefined {
  const all = placedMembers(layout)
  const cur = all.find((p) => p.key === from)
  if (!cur) {
    return undefined
  }
  if (dir === 'up' || dir === 'down') {
    const inCol = all
      .filter((p) => p.column === cur.column)
      .sort((a, b) => a.cy - b.cy)
    const i = inCol.findIndex((p) => p.key === from)
    return inCol[dir === 'up' ? i - 1 : i + 1]?.key
  }
  const step = dir === 'right' ? 1 : -1
  const columns = [...new Set(all.map((p) => p.column))].sort((a, b) => a - b)
  const target = columns[columns.indexOf(cur.column) + step]
  if (target === undefined) {
    return undefined
  }
  return all
    .filter((p) => p.column === target)
    .sort((a, b) => Math.abs(a.cy - cur.cy) - Math.abs(b.cy - cur.cy))[0]?.key
}
