import type { DeployAttempt } from '../types/deployAttempt'
import type { ReconcileCondition } from '../types/deploy'

// A deploy attempt's real, structural stage timeline: Build (this
// package's internal/deploy.Pipeline.Deploy: build the image, or nothing
// at all for a bare image-tag redeploy) then Roll out (the reconciler's
// own async create/start/readiness sequence, internal/reconcile/
// application/controller.go). Derived entirely from data already fetched
// elsewhere (a deploy attempt row plus the app's current reconcile
// conditions), not from a new backend concept: internal/build.ProgressEvent
// does carry a Step field, but it is per-BuildKit-vertex (one event per
// Dockerfile instruction) and is dropped before it ever reaches the SSE
// stream (internal/deploylog.Recorder.Progress ignores step-lifecycle
// events), so it is the wrong granularity for a stage timeline and would
// need real backend work to surface usefully. This file is the
// deliberately backend-free alternative: no new schema, no new SSE
// fields, just an honest read of state that already exists.
export type DeployStageStatus =
  | 'pending'
  | 'running'
  | 'done'
  | 'failed'
  | 'skipped'
  | 'unknown'

export interface DeployStage {
  key: 'build' | 'rollout'
  label: string
  status: DeployStageStatus
  detail?: string
  startedAt?: string
  finishedAt?: string
}

const ROLLOUT_FAILURE_REASONS = ['CreateFailed', 'StartFailed', 'ReadinessFailed']

// Fixed-length tuple, not DeployStage[]: always exactly Build then Roll
// out, so callers indexing stages[0] don't need an unnecessary
// possibly-undefined check.
export function computeDeployStages(
  attempt: DeployAttempt,
  conditions: ReconcileCondition[],
  isLatestAttempt: boolean,
): [DeployStage, DeployStage] {
  return [computeBuildStage(attempt), computeRolloutStage(attempt, conditions, isLatestAttempt)]
}

function computeBuildStage(attempt: DeployAttempt): DeployStage {
  if (attempt.source === 'image') {
    return {
      key: 'build',
      label: 'Build',
      status: 'skipped',
      detail: 'Using a pre-built image, no build step ran.',
    }
  }
  return {
    key: 'build',
    label: 'Build',
    status:
      attempt.status === 'running'
        ? 'running'
        : attempt.status === 'succeeded'
          ? 'done'
          : 'failed',
    detail: attempt.status === 'failed' ? attempt.error : undefined,
    startedAt: attempt.started_at,
    finishedAt: attempt.finished_at,
  }
}

// Roll out is only ever knowable for the app's single latest attempt:
// internal/store.UpsertConditions keeps only the latest condition per
// (controller, type) pair, never an attempt-scoped history (see
// types/deploy.ts's own doc comment), so an older attempt's roll-out
// outcome cannot be reconstructed once a later deploy has overwritten
// those same rows.
function computeRolloutStage(
  attempt: DeployAttempt,
  conditions: ReconcileCondition[],
  isLatestAttempt: boolean,
): DeployStage {
  const key = 'rollout' as const
  const label = 'Roll out'

  if (attempt.status === 'failed') {
    return { key, label, status: 'skipped', detail: 'The build failed before a roll out could start.' }
  }
  if (attempt.status === 'running') {
    return { key, label, status: 'pending' }
  }
  if (!isLatestAttempt) {
    return {
      key,
      label,
      status: 'unknown',
      detail: "Not tracked for past attempts: reconcile status only reflects the app's current state.",
    }
  }

  // attempt.status === 'succeeded' and this is the latest attempt: the
  // build finished, so a condition transition at or after finished_at
  // reflects the reconciler reacting to this exact attempt's desired
  // state, not one left over from an earlier deploy.
  const referenceMs = attempt.finished_at ? new Date(attempt.finished_at).getTime() : null

  const failedCondition = conditions.find(
    (c) => ROLLOUT_FAILURE_REASONS.includes(c.Reason) && isAtOrAfter(c.LastTransitionTime, referenceMs),
  )
  if (failedCondition) {
    return {
      key,
      label,
      status: 'failed',
      detail: failedCondition.Message,
      startedAt: attempt.finished_at,
      finishedAt: failedCondition.LastTransitionTime,
    }
  }

  const deployedCondition = conditions.find(
    (c) => c.Reason === 'Deployed' && isAtOrAfter(c.LastTransitionTime, referenceMs),
  )
  if (deployedCondition) {
    return {
      key,
      label,
      status: 'done',
      startedAt: attempt.finished_at,
      finishedAt: deployedCondition.LastTransitionTime,
    }
  }

  return { key, label, status: 'running', startedAt: attempt.finished_at }
}

function isAtOrAfter(timestamp: string, referenceMs: number | null): boolean {
  if (referenceMs === null) return false
  const t = new Date(timestamp).getTime()
  return Number.isFinite(t) && t >= referenceMs
}
