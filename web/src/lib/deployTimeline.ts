import type { Tone } from '@/components/kit'
import type { Deployment } from '../types/deployment'
import { statusView } from './deploymentPresentation'

export const TIMELINE_ZOOMS = ['1h', '24h', '7d'] as const
export type TimelineZoom = (typeof TIMELINE_ZOOMS)[number]

export const ZOOM_MS: Record<TimelineZoom, number> = {
  '1h': 3_600_000,
  '24h': 86_400_000,
  '7d': 604_800_000,
}

export interface TimelineDeploy {
  id: string
  app: string
  status: string
  trigger: string
  environment: string
  commit_sha: string
  commit_message: string
  author: string
  strategy: string
  project_id: string
  node_id: string
  started_at: string
  finished_at: string | null
  duration_ms: number | null
  rollback_of: string | null
  is_live: boolean
}

export interface TimelineAlert {
  id: string
  app: string
  rule_name: string
  severity: string
  at: string
}

export interface TimelinePage {
  since: string
  until: string
  items: TimelineDeploy[]
  alerts?: TimelineAlert[]
  next_cursor: string
}

export interface TimelineFilters {
  zoom: TimelineZoom
  app: string
  project: string
  node: string
  status: string
}

export const DEFAULT_TIMELINE_FILTERS: TimelineFilters = {
  zoom: '24h',
  app: '',
  project: '',
  node: '',
  status: '',
}

const IN_FLIGHT = new Set(['building', 'queued', 'awaiting_approval'])

export function isInFlight(d: Pick<TimelineDeploy, 'status'>): boolean {
  return IN_FLIGHT.has(d.status)
}

export function isMarker(d: Pick<TimelineDeploy, 'status'>): boolean {
  return d.status === 'failed' || d.status === 'rolled_back'
}

export function barTone(d: Pick<TimelineDeploy, 'status'>): Tone {
  return statusView(d.status).tone
}

/** Tailwind needs literal class names, so the tone to bar color map is spelled out. */
export const BAR_CLASS: Record<Tone, string> = {
  neutral: 'bg-tone-neutral-solid',
  success: 'bg-tone-success-solid',
  warning: 'bg-tone-warning-solid',
  danger: 'bg-tone-danger-solid',
  info: 'bg-tone-info-solid',
  accent: 'bg-tone-accent-solid',
}

export interface BarGeometry {
  left: number
  width: number
}

const MIN_WIDTH_PCT = 0.6

/** Percent offsets of a deploy inside the window; an open deploy runs to now. */
export function barGeometry(
  d: Pick<TimelineDeploy, 'started_at' | 'finished_at'>,
  windowStart: number,
  windowEnd: number,
): BarGeometry | null {
  const span = windowEnd - windowStart
  if (span <= 0) return null
  const start = Date.parse(d.started_at)
  const end = d.finished_at ? Date.parse(d.finished_at) : windowEnd
  if (!Number.isFinite(start) || !Number.isFinite(end)) return null
  if (end < windowStart || start > windowEnd) return null
  const from = Math.max(start, windowStart)
  const to = Math.min(Math.max(end, from), windowEnd)
  const left = ((from - windowStart) / span) * 100
  const width = Math.max(((to - from) / span) * 100, MIN_WIDTH_PCT)
  return { left: Math.min(left, 100 - MIN_WIDTH_PCT), width }
}

export interface Lane {
  app: string
  deploys: TimelineDeploy[]
}

/** One lane per app, the app with the most recent activity first. */
export function groupLanes(deploys: TimelineDeploy[]): Lane[] {
  const byApp = new Map<string, TimelineDeploy[]>()
  for (const d of deploys) {
    const list = byApp.get(d.app)
    if (list) list.push(d)
    else byApp.set(d.app, [d])
  }
  const lanes = [...byApp.entries()].map(([app, list]) => ({
    app,
    deploys: list.sort((a, b) => a.started_at.localeCompare(b.started_at)),
  }))
  const latest = (l: Lane) => l.deploys[l.deploys.length - 1]?.started_at ?? ''
  return lanes.sort(
    (a, b) => latest(b).localeCompare(latest(a)) || a.app.localeCompare(b.app),
  )
}

export function toTimelineDeploy(
  d: Deployment,
  prev?: TimelineDeploy,
): TimelineDeploy {
  return {
    id: d.id,
    app: d.app,
    status: d.status,
    trigger: d.trigger,
    environment: d.environment,
    commit_sha: d.commit_sha,
    commit_message: d.commit_message,
    author: d.author,
    strategy: prev?.strategy ?? '',
    project_id: prev?.project_id ?? '',
    node_id: prev?.node_id ?? '',
    started_at: d.started_at,
    finished_at: d.finished_at,
    duration_ms: d.duration_ms,
    rollback_of: d.rollback_of,
    is_live: d.is_live,
  }
}

/**
 * Whether a streamed deploy belongs in the current view. Project and node
 * are unknown for an app the page has not loaded yet, so those filters
 * only admit apps already placed.
 */
export function matchesFilters(
  d: TimelineDeploy,
  f: TimelineFilters,
  knownApps: ReadonlyMap<string, TimelineDeploy>,
): boolean {
  if (f.app && d.app !== f.app) return false
  if (f.status && d.status !== f.status) return false
  if (f.project || f.node) {
    const known = knownApps.get(d.app)
    if (!known) return false
    if (f.project && known.project_id !== f.project) return false
    if (f.node && known.node_id !== f.node) return false
  }
  return true
}

export function applyStreamedDeploy(
  rows: TimelineDeploy[],
  ev: Deployment,
  f: TimelineFilters,
  windowStart: number,
): TimelineDeploy[] {
  const idx = rows.findIndex((r) => r.id === ev.id)
  const known = new Map<string, TimelineDeploy>()
  for (const r of rows) known.set(r.app, r)
  const next = toTimelineDeploy(ev, idx >= 0 ? rows[idx] : known.get(ev.app))
  const finished = next.finished_at ? Date.parse(next.finished_at) : Infinity
  if (!matchesFilters(next, f, known) || finished < windowStart) {
    return idx >= 0 ? rows.filter((r) => r.id !== ev.id) : rows
  }
  if (idx >= 0) return rows.map((r, i) => (i === idx ? next : r))
  return [next, ...rows]
}

export function toQuery(f: TimelineFilters, cursor: string): string {
  const q = new URLSearchParams({ since: f.zoom, limit: '200' })
  if (f.app) q.set('app', f.app)
  if (f.project) q.set('project', f.project)
  if (f.node) q.set('node', f.node)
  if (f.status) q.set('status', f.status)
  if (cursor) q.set('cursor', cursor)
  return q.toString()
}
