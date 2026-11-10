import type { Tone } from '@/components/kit'
import { formatBytes } from '../../lib/format'
import type { BuildRow } from '../../queries/buildCenter'

const STUCK_RUNNING_SECONDS = 60 * 60
const STUCK_QUEUED_SECONDS = 5 * 60

export function statusTone(status: string): Tone {
  switch (status) {
    case 'succeeded':
      return 'success'
    case 'failed':
      return 'danger'
    case 'running':
      return 'info'
    case 'queued':
    case 'held':
      return 'warning'
    default:
      return 'neutral'
  }
}

export function isWaiting(row: BuildRow): boolean {
  return row.status === 'queued' || row.status === 'held'
}

// A build running for over an hour, or a queue nobody is draining, needs a human.
export function isStuck(
  row: BuildRow,
  running: number,
  nowMs: number,
): boolean {
  const since = Date.parse(row.queued_at ?? row.started_at)
  if (Number.isNaN(since)) return false
  const elapsed = (nowMs - since) / 1000
  if (row.status === 'running') return elapsed > STUCK_RUNNING_SECONDS
  return isWaiting(row) && running === 0 && elapsed > STUCK_QUEUED_SECONDS
}

export function durationLabel(seconds: number | undefined): string {
  if (seconds === undefined) return '-'
  if (seconds < 60) return `${Math.round(seconds)}s`
  const m = Math.floor(seconds / 60)
  const s = Math.round(seconds % 60)
  return s === 0 ? `${m}m` : `${m}m ${s}s`
}

export function cacheLabel(ratio: number | undefined): string {
  return ratio === undefined ? '-' : `${Math.round(ratio * 100)}%`
}

export function sizeLabel(bytes: number | undefined): string {
  return bytes && bytes > 0 ? formatBytes(bytes) : '-'
}

export function deltaLabel(delta: number | undefined): string | null {
  if (delta === undefined || delta === 0) return null
  return `${delta > 0 ? '+' : '-'}${formatBytes(Math.abs(delta))}`
}

const BUILD_STATUS_FILTERS = ['queued', 'running', 'failed'] as const
export type BuildStatusFilter = (typeof BUILD_STATUS_FILTERS)[number]

/** parseBuildStatusFilter keeps only the statuses the Builds tiles can filter by. */
export function parseBuildStatusFilter(
  value: unknown,
): BuildStatusFilter | undefined {
  return BUILD_STATUS_FILTERS.find((s) => s === value)
}

/** matchesBuildStatus treats held builds as queued, the same grouping the Queued tile counts. */
export function matchesBuildStatus(
  status: string,
  filter: BuildStatusFilter,
): boolean {
  return filter === 'queued'
    ? status === 'queued' || status === 'held'
    : status === filter
}
