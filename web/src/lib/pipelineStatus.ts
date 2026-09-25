import type { PipelineStatus } from '../types/pipelines'

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
