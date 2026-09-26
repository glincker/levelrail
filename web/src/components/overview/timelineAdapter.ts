import { imageTagOf } from './imageTag'
import type { TimelineEntry, TimelineStatus } from '../../queries/appTimeline'
import type { AppDetail } from '../../types/appDetail'
import type { ReconcileCondition } from '../../types/deploy'
import type {
  DeployAttempt,
  DeployAttemptSource,
  DeployAttemptStatus,
} from '../../types/deployAttempt'

const ATTEMPT_STATUS: Record<DeployAttemptStatus, TimelineStatus> = {
  running: 'in_progress',
  succeeded: 'succeeded',
  failed: 'failed',
  held: 'pending',
  superseded: 'info',
  queued: 'pending',
  canceled: 'info',
}

const SOURCE_ACTOR: Record<DeployAttemptSource, string> = {
  webhook: 'git push',
  manual: 'dashboard',
  image: 'image deploy',
  auto_rollback: 'auto rollback',
}

function attemptTitle(a: DeployAttempt): string {
  if (a.source === 'auto_rollback')
    return `Rolled back to ${imageTagOf(a.image)}`
  if (a.commit_sha) return `Deploy ${a.commit_sha.slice(0, 7)}`
  return `Deploy ${imageTagOf(a.image)}`
}

/** Builds the timeline from data the page already has, newest first. */
export function fallbackTimeline(
  attempts: DeployAttempt[],
  app: Pick<AppDetail, 'env_dirty' | 'suspended'>,
  conditions: ReconcileCondition[],
  limit = 8,
): TimelineEntry[] {
  const entries: TimelineEntry[] = attempts.map((a) => ({
    id: `attempt-${a.id}`,
    at: a.started_at,
    kind: a.source === 'auto_rollback' ? 'rollback' : 'deploy',
    status: ATTEMPT_STATUS[a.status],
    actor: a.source ? SOURCE_ACTOR[a.source] : 'system',
    title: attemptTitle(a),
    detail: a.status === 'failed' ? a.error : a.reason,
    ref: { type: 'deploy_attempt', id: a.id },
  }))

  const latestTransition = conditions
    .map((c) => c.LastTransitionTime)
    .filter(Boolean)
    .sort()
    .pop()
  if (latestTransition && app.env_dirty) {
    entries.push({
      id: 'inferred-env-pending',
      at: latestTransition,
      kind: 'env_change',
      status: 'pending',
      actor: 'you',
      title: 'Environment changes waiting for a restart',
    })
  }
  if (latestTransition && app.suspended) {
    entries.push({
      id: 'inferred-suspend',
      at: latestTransition,
      kind: 'suspend',
      status: 'info',
      actor: 'you',
      title: 'App stopped',
    })
  }

  return entries
    .sort((a, b) => Date.parse(b.at) - Date.parse(a.at))
    .slice(0, limit)
}
