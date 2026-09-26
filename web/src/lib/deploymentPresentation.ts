import {
  ArrowUUpLeftIcon,
  CheckCircleIcon,
  ClockIcon,
  GavelIcon,
  ProhibitIcon,
  PauseCircleIcon,
  SkipForwardIcon,
  SpinnerGapIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { Tone } from '@/components/kit'
import type { Deployment, DeploymentStatus } from '../types/deployment'
import { formatDurationMs } from './deployDuration'

export interface StatusView {
  label: string
  tone: Tone
  icon: Icon
  spin?: boolean
}

export const STATUS_VIEW: Record<DeploymentStatus, StatusView> = {
  building: {
    label: 'Building',
    tone: 'info',
    icon: SpinnerGapIcon,
    spin: true,
  },
  queued: { label: 'Queued', tone: 'neutral', icon: ClockIcon },
  ready: { label: 'Ready', tone: 'success', icon: CheckCircleIcon },
  failed: { label: 'Failed', tone: 'danger', icon: XCircleIcon },
  canceled: { label: 'Canceled', tone: 'neutral', icon: ProhibitIcon },
  held: { label: 'Held', tone: 'warning', icon: PauseCircleIcon },
  awaiting_approval: {
    label: 'Awaiting approval',
    tone: 'warning',
    icon: GavelIcon,
  },
  rolled_back: {
    label: 'Rolled back',
    tone: 'neutral',
    icon: ArrowUUpLeftIcon,
  },
  superseded: { label: 'Superseded', tone: 'neutral', icon: SkipForwardIcon },
}

export function statusView(status: string): StatusView {
  return (
    STATUS_VIEW[status as DeploymentStatus] ?? {
      label: status,
      tone: 'neutral',
      icon: ClockIcon,
    }
  )
}

export const TRIGGER_LABEL: Record<string, string> = {
  'git push': 'Git push',
  manual: 'Manual',
  rollback: 'Rollback',
  api: 'API',
  preview: 'Preview',
  schedule: 'Schedule',
  pipeline: 'Pipeline',
}

export function triggerLabel(trigger: string): string {
  return TRIGGER_LABEL[trigger] ?? trigger
}

export function isInProgress(d: Deployment): boolean {
  return d.status === 'building' || d.status === 'queued'
}

export function shortSha(sha: string): string {
  return sha.slice(0, 7)
}

export function shortId(id: string): string {
  return id.slice(0, 8)
}

export function isProduction(env: string): boolean {
  const e = env.toLowerCase()
  return e === 'production' || e === 'prod'
}

export function isRollback(d: Deployment): boolean {
  return d.trigger === 'rollback' || d.rollback_of !== null
}

/** A redeploy re-ships an existing image: no commit, no branch, not a rollback. */
export function isRedeploy(d: Deployment): boolean {
  return !isRollback(d) && d.commit_sha === '' && d.branch === ''
}

export type RowKind =
  | 'building'
  | 'queued'
  | 'live'
  | 'failed'
  | 'rollback'
  | 'redeploy'
  | 'superseded'
  | 'standard'

export function rowKind(d: Deployment): RowKind {
  if (d.status === 'building') return 'building'
  if (d.status === 'queued') return 'queued'
  if (d.status === 'failed') return 'failed'
  if (d.status === 'superseded') return 'superseded'
  if (isRollback(d)) return 'rollback'
  if (d.is_live) return 'live'
  if (isRedeploy(d)) return 'redeploy'
  return 'standard'
}

function imageTag(image: string): string {
  return image.split('/').pop() ?? image
}

/** The left-hand headline: commit message, or what a redeploy or rollback did. */
export function headline(d: Deployment): string {
  if (isRollback(d)) {
    return d.rollback_of ? `Rollback to ${shortId(d.rollback_of)}` : 'Rollback'
  }
  if (d.commit_message) return d.commit_message
  if (isRedeploy(d)) return `Redeploy of ${imageTag(d.image)}`
  return imageTag(d.image) || d.id
}

/** The reason a non-happy row is in that state, so a status is never bare. */
export function subtitle(d: Deployment): string {
  switch (d.status) {
    case 'failed':
      return d.error_summary ?? d.reason
    case 'superseded':
      return d.superseded_by
        ? `Superseded by ${shortId(d.superseded_by)}`
        : d.reason
    case 'awaiting_approval':
      return d.reason || 'Waiting for approval'
    case 'held':
    case 'canceled':
    case 'queued':
      return d.reason
    default:
      return ''
  }
}

export function initials(name: string): string {
  const parts = name
    .trim()
    .split(/[\s._-]+/)
    .filter(Boolean)
  if (parts.length === 0) return '?'
  const a = parts[0]?.[0] ?? ''
  const b = parts[1]?.[0] ?? ''
  return (a + b).toUpperCase()
}

/** Elapsed or final duration in ms, ticking against `now` while in progress. */
export function durationMs(d: Deployment, now: number): number | null {
  if (d.duration_ms !== null) return d.duration_ms
  if (d.status === 'building') {
    const ms = now - new Date(d.started_at).getTime()
    return ms >= 0 ? ms : 0
  }
  return null
}

export function durationLabel(d: Deployment, now: number): string {
  const ms = durationMs(d, now)
  return ms === null ? '' : (formatDurationMs(ms) ?? '')
}

export function stepProgress(d: Deployment): string {
  const s = d.steps
  if (!s) return ''
  const total = s.done + s.running + s.failed
  return total > 0 ? `${String(s.done)}/${String(total)} steps` : ''
}

/** Digest-truthful ref: only a registry-verified digest is ever presented as `repo@sha256`. */
export function pinnedRef(d: Deployment): string {
  return d.image_ref.includes('@sha256:') ? d.image_ref : ''
}

export function rollbackImage(d: Deployment): string {
  return pinnedRef(d) || d.image
}

export const REDEPLOYABLE: DeploymentStatus[] = [
  'ready',
  'failed',
  'rolled_back',
  'superseded',
  'canceled',
]

export function canRedeploy(d: Deployment): boolean {
  return REDEPLOYABLE.includes(d.status) && d.image !== ''
}

export function canRollbackTo(d: Deployment): boolean {
  return (
    (d.status === 'ready' || d.status === 'rolled_back') &&
    !d.is_live &&
    d.image !== ''
  )
}

export function canCancel(d: Deployment): boolean {
  return isInProgress(d)
}

export function stepPercent(d: Deployment): number | null {
  const s = d.steps
  if (!s) return null
  const total = s.done + s.running + s.failed
  return total > 0 ? Math.round((s.done / total) * 100) : null
}

export function formatFailureRate(rate: number | null): string {
  if (rate === null) return 'n/a'
  const pct = rate * 100
  return `${Number.isInteger(pct) ? String(pct) : pct.toFixed(1)}%`
}
