import type { PipelineJob, PipelineStatus } from '../types/pipelines'

export type BadgeVariant =
  'default' | 'outline' | 'destructive' | 'muted' | 'success' | 'warning'

export const STATUS_LABEL: Record<PipelineStatus, string> = {
  queued: 'Queued',
  pending: 'Pending',
  running: 'Running',
  waiting_approval: 'Needs approval',
  succeeded: 'Succeeded',
  failed: 'Failed',
  cancelled: 'Cancelled',
  skipped: 'Skipped',
}

export const STATUS_VARIANT: Record<PipelineStatus, BadgeVariant> = {
  queued: 'muted',
  pending: 'muted',
  running: 'default',
  waiting_approval: 'warning',
  succeeded: 'success',
  failed: 'destructive',
  cancelled: 'muted',
  skipped: 'muted',
}

export function baseJobName(key: string): string {
  const i = key.indexOf('[')
  return i >= 0 ? key.slice(0, i) : key
}

// layoutJobs groups jobs into columns by dependency depth: a job sits one
// column right of the deepest job it needs, so the columns read left to
// right as the order the pipeline runs in.
export function layoutJobs(jobs: PipelineJob[]): PipelineJob[][] {
  const byBase = new Map<string, PipelineJob[]>()
  for (const j of jobs) {
    const base = baseJobName(j.key)
    byBase.set(base, [...(byBase.get(base) ?? []), j])
  }
  const depth = new Map<string, number>()
  const resolve = (base: string, seen: Set<string>): number => {
    const known = depth.get(base)
    if (known !== undefined) {
      return known
    }
    if (seen.has(base)) {
      return 0
    }
    seen.add(base)
    const needs = byBase.get(base)?.[0]?.needs ?? []
    const d =
      needs.length === 0
        ? 0
        : 1 + Math.max(...needs.map((n) => resolve(n, seen)))
    depth.set(base, d)
    return d
  }
  const columns: PipelineJob[][] = []
  for (const j of jobs) {
    const d = resolve(baseJobName(j.key), new Set())
    columns[d] = [...(columns[d] ?? []), j]
  }
  return columns.filter((c): c is PipelineJob[] => c !== undefined)
}

export function formatDuration(start?: string, end?: string): string {
  if (!start) {
    return ''
  }
  const ms =
    (end ? new Date(end).getTime() : Date.now()) - new Date(start).getTime()
  const s = Math.max(0, Math.round(ms / 1000))
  if (s < 60) {
    return `${s}s`
  }
  const m = Math.floor(s / 60)
  return m < 60 ? `${m}m ${s % 60}s` : `${Math.floor(m / 60)}h ${m % 60}m`
}
