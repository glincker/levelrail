import type { DeployAttempt, DeployAttemptStatus } from '../types/deployAttempt'
import type { TimelineEntry } from '../queries/appTimeline'
import type { Tone } from '@/components/kit'
import { DEPLOY_ATTEMPT_STATUS_TONE } from './deployAttemptPresentation'

export type HistoryKind = 'deploy' | 'rollback' | 'restart'

export interface HistoryEntry {
  id: string
  at: string
  kind: HistoryKind
  /** Person or trigger that caused it, empty when unknown. */
  who: string
  sha: string
  statusKey: DeployAttemptStatus
  tone: Tone
  durationMs: number | null
  /** Deploy attempt id when the entry links to a log page. */
  attemptId: string | null
}

// A restart reported by both sources within this gap is one event.
const DEDUPE_WINDOW_MS = 60_000

function attemptKind(a: DeployAttempt): HistoryKind {
  if (a.source === 'restart') return 'restart'
  if (a.trigger === 'rollback' || a.source === 'auto_rollback') {
    return 'rollback'
  }
  return 'deploy'
}

function attemptDuration(a: DeployAttempt): number | null {
  if (!a.finished_at) return null
  const ms = Date.parse(a.finished_at) - Date.parse(a.started_at)
  return Number.isFinite(ms) && ms >= 0 ? ms : null
}

function fromAttempt(a: DeployAttempt): HistoryEntry {
  return {
    id: `attempt:${a.id}`,
    at: a.started_at,
    kind: attemptKind(a),
    who: a.author ?? a.canceled_by ?? a.source ?? '',
    sha: a.commit_sha ? a.commit_sha.slice(0, 7) : '',
    statusKey: a.status,
    tone: DEPLOY_ATTEMPT_STATUS_TONE[a.status],
    durationMs: attemptDuration(a),
    attemptId: a.id,
  }
}

const TIMELINE_STATUS: Record<
  TimelineEntry['status'],
  [DeployAttemptStatus, Tone]
> = {
  succeeded: ['succeeded', 'success'],
  failed: ['failed', 'danger'],
  in_progress: ['running', 'info'],
  pending: ['pending', 'warning'],
  info: ['succeeded', 'success'],
}

function fromTimeline(e: TimelineEntry): HistoryEntry | null {
  if (e.kind !== 'restart' && e.kind !== 'rollback') return null
  const [statusKey, tone] = TIMELINE_STATUS[e.status]
  return {
    id: `timeline:${e.id}`,
    at: e.at,
    kind: e.kind,
    who: e.actor,
    sha: '',
    statusKey,
    tone,
    durationMs: null,
    attemptId: e.ref?.id ?? null,
  }
}

/**
 * One newest-first list of deploys, restarts and rollbacks. Activity-feed
 * restarts and rollbacks are added only when no deploy attempt already
 * covers them, so nothing shows twice.
 */
export function mergeAppHistory(
  attempts: readonly DeployAttempt[],
  activity: readonly TimelineEntry[],
): HistoryEntry[] {
  const entries = attempts.map(fromAttempt)
  const attemptIds = new Set(attempts.map((a) => a.id))
  for (const e of activity) {
    if (e.ref && attemptIds.has(e.ref.id)) continue
    const entry = fromTimeline(e)
    if (!entry) continue
    const at = Date.parse(entry.at)
    const dup = entries.some(
      (x) =>
        x.kind === entry.kind &&
        Math.abs(Date.parse(x.at) - at) <= DEDUPE_WINDOW_MS,
    )
    if (!dup) entries.push(entry)
  }
  return entries.sort((a, b) => Date.parse(b.at) - Date.parse(a.at))
}
