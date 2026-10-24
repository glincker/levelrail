import type {
  CleanupCategory,
  CleanupRun,
  CleanupRunFilters,
  CleanupTarget,
} from '../types/cleanup'
import type { Tone } from '../components/kit/tone'

export const CATEGORY_ORDER: CleanupCategory[] = [
  'auth',
  'audit',
  'deploys',
  'docker',
  'data',
  'observability',
  'messaging',
  'other',
]

export const DEFAULT_PRESETS = [0, 30, 60, 90, 180, 365]

/** A tone can repeat across categories: the legend always names them, so color is never the only cue. */
export const CATEGORY_STYLE: Record<
  CleanupCategory,
  { tone: Tone; faded: boolean }
> = {
  auth: { tone: 'info', faded: false },
  audit: { tone: 'accent', faded: false },
  deploys: { tone: 'success', faded: false },
  docker: { tone: 'warning', faded: false },
  data: { tone: 'neutral', faded: false },
  observability: { tone: 'info', faded: true },
  messaging: { tone: 'accent', faded: true },
  other: { tone: 'neutral', faded: true },
}

export function isReadThrough(target: CleanupTarget): boolean {
  return target.read_through ?? target.policy_key === ''
}

export function totalBytes(targets: CleanupTarget[]): number {
  return targets.reduce((sum, t) => sum + Math.max(0, t.usage.bytes ?? 0), 0)
}

export interface CategorySlice {
  category: CleanupCategory
  bytes: number
  rows: number
  targets: CleanupTarget[]
}

export function groupTargets(targets: CleanupTarget[]): CategorySlice[] {
  return CATEGORY_ORDER.map((category) => {
    const items = targets.filter((t) => t.category === category)
    return {
      category,
      targets: items,
      bytes: totalBytes(items),
      rows: items.reduce((sum, t) => sum + t.usage.rows, 0),
    }
  }).filter((g) => g.targets.length > 0)
}

/** Share of a part in a whole as a 0 to 100 number, safe for an empty whole. */
export function percentOf(part: number, whole: number): number {
  if (whole <= 0 || part <= 0) return 0
  return Math.min(100, (part / whole) * 100)
}

/** Targets a preview-all pass can measure: managed here, switched on or to preview, with a window. */
export function measurableTargets(targets: CleanupTarget[]): CleanupTarget[] {
  return targets.filter(
    (t) => !isReadThrough(t) && t.mode !== 'off' && t.days > 0,
  )
}

export function previewTargets(targets: CleanupTarget[]): CleanupTarget[] {
  return targets.filter((t) => t.mode === 'preview')
}

/** Union of the presets and the live value so an env override still shows. */
export function retentionOptions(allowed: number[], current: number): number[] {
  const set = new Set([...allowed, current])
  return [...set].sort((a, b) => a - b)
}

/** 0 means keep forever, so only a positive value below the floor is refused. */
export function belowFloor(days: number, floor: number): boolean {
  return floor > 0 && days > 0 && days < floor
}

/** Longer retention is never a weakening; 0 (forever) is the longest. */
export function isShortening(from: number, to: number): boolean {
  if (from === to || to === 0) return false
  if (from === 0) return true
  return to < from
}

export function latestRun(targets: CleanupTarget[]): string | null {
  let best: string | null = null
  for (const t of targets) {
    const at = t.last_run?.started_at
    if (at && (best === null || at > best)) best = at
  }
  return best
}

export function runsQuery(
  filters: CleanupRunFilters,
  extra: Record<string, string> = {},
): string {
  const p = new URLSearchParams()
  if (filters.target) p.set('target', filters.target)
  if (filters.since)
    p.set('since', new Date(`${filters.since}T00:00:00`).toISOString())
  if (filters.until) {
    const end = new Date(`${filters.until}T00:00:00`)
    end.setDate(end.getDate() + 1)
    p.set('until', end.toISOString())
  }
  if (filters.kind !== 'all') p.set('dry_run', String(filters.kind === 'dry'))
  if (filters.status !== 'all') p.set('status', filters.status)
  for (const [k, v] of Object.entries(extra)) p.set(k, v)
  return p.toString()
}

export function runsExportUrl(
  filters: CleanupRunFilters,
  format: 'csv' | 'json',
): string {
  return `/api/v1/cleanup/runs?${runsQuery(filters, { format })}`
}

export interface RunDay {
  day: string
  runs: CleanupRun[]
}

function dayKey(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const date = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${month}-${date}`
}

/** Groups newest-first runs under their local calendar day. */
export function groupRunsByDay(runs: CleanupRun[]): RunDay[] {
  const days: RunDay[] = []
  for (const run of runs) {
    const day = dayKey(run.started_at)
    const last = days[days.length - 1]
    if (last && last.day === day) last.runs.push(run)
    else days.push({ day, runs: [run] })
  }
  return days
}
