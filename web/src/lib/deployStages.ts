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
  'pending' | 'running' | 'done' | 'failed' | 'skipped' | 'unknown'

export interface DeployStage {
  key: 'build' | 'rollout' | 'health-check' | 'cutover' | 'cleanup'
  label: string
  status: DeployStageStatus
  detail?: string
  startedAt?: string
  finishedAt?: string
}

// Every failure reason ensureReplicaRunning (internal/reconcile/
// application/controller.go) can actually report, not just the three
// original ones: InspectFailed, EnsureNetworkFailed, VanishedAfterStart,
// and PreDeployHookFailed were always missing here too, and
// OOMKilledDuringReadiness/ExitedDuringReadiness (added once the
// reconciler learned to fail fast on a crash during the readiness wait)
// were never added when that shipped. Missing any of these means this
// function falls through to its own "running" default below for a
// deploy that has actually already failed, showing a permanently stuck
// spinner instead of the real outcome.
const ROLLOUT_FAILURE_REASONS = [
  'CreateFailed',
  'StartFailed',
  'ReadinessFailed',
  'InspectFailed',
  'EnsureNetworkFailed',
  'VanishedAfterStart',
  'PreDeployHookFailed',
  'OOMKilledDuringReadiness',
  'ExitedDuringReadiness',
]

// Both are the application controller's own terminal-success Ready
// reasons (internal/reconcile/application/controller.go's "Deployed",
// liveness.go's steadyStateResult "AlreadyRunning"): a redeploy to a
// spec that already matches the running container converges without
// ever creating anything, so it reports AlreadyRunning instead of
// Deployed. Treating only "Deployed" as done here left every such
// redeploy (e.g. re-triggering the same image tag) stuck on "running"
// forever, since that condition transition never occurs.
const ROLLOUT_DONE_REASONS = ['Deployed', 'AlreadyRunning']

// Fixed-length tuple, not DeployStage[]: always exactly Build then Roll
// out, so callers indexing stages[0] don't need an unnecessary
// possibly-undefined check.
export function computeDeployStages(
  attempt: DeployAttempt,
  conditions: ReconcileCondition[],
  isLatestAttempt: boolean,
): [DeployStage, DeployStage] {
  return [
    computeBuildStage(attempt),
    computeRolloutStage(attempt, conditions, isLatestAttempt),
  ]
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
    return {
      key,
      label,
      status: 'skipped',
      detail: 'The build failed before a roll out could start.',
    }
  }
  if (attempt.status === 'running') {
    return { key, label, status: 'pending' }
  }
  if (!isLatestAttempt) {
    return {
      key,
      label,
      status: 'unknown',
      detail:
        "Not tracked for past attempts: reconcile status only reflects the app's current state.",
    }
  }

  // attempt.status === 'succeeded' and this is the latest attempt: the
  // build finished, so a condition transition at or after finished_at
  // reflects the reconciler reacting to this exact attempt's desired
  // state, not one left over from an earlier deploy.
  const referenceMs = attempt.finished_at
    ? new Date(attempt.finished_at).getTime()
    : null

  const failedCondition = conditions.find(
    (c) =>
      ROLLOUT_FAILURE_REASONS.includes(c.Reason) &&
      isAtOrAfter(c.LastTransitionTime, referenceMs),
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
    (c) =>
      ROLLOUT_DONE_REASONS.includes(c.Reason) &&
      isAtOrAfter(c.LastTransitionTime, referenceMs),
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

// Every reason internal/reconcile/application/controller.go can report
// before a new replica is confirmed running and passing its readiness
// probe: everything ensureReplicaRunning itself can fail with, plus the
// two failures that can occur before it (StoreError/SuspendFailed) or
// reachable only via the recreate strategy's own pre-start teardown
// (CleanupFailed, distinct from RunningStaleCleanupFailed below: that one
// fires after a successful deploy, this one before any new container
// exists at all).
const HEALTH_CHECK_FAILURE_REASONS = [
  'StoreError',
  'SuspendFailed',
  'StrategyUnrecognized',
  'InspectFailed',
  'CleanupFailed',
  'CreateFailed',
  'EnsureNetworkFailed',
  'StartFailed',
  'VanishedAfterStart',
  'PreDeployHookFailed',
  'ReadinessFailed',
  'OOMKilledDuringReadiness',
  'ExitedDuringReadiness',
]

// The reconciler removes an old container only after a new one is
// confirmed running and ready (controller.go's removeStale, called from
// each strategy's own reconcile method), so this Reason is only ever
// reachable once the health-check and cutover work above has already
// succeeded: Status stays True (the important fact, a healthy set is
// serving, is still true), just with this Reason marking the cleanup
// step specifically as incomplete.
const CLEANUP_FAILURE_REASON = 'RunningStaleCleanupFailed'

// Every Reason finishReconcile can report once a fresh replica set is
// confirmed running, ready, and cleaned up: the fully clean case
// (Deployed/AlreadyRunning) plus two reasons for a secondary, post-cutover
// step (a configured post-deploy hook, recording the deploy metric)
// failing without undoing the cutover that already succeeded. All of
// these mean health check, cutover, and cleanup themselves all
// succeeded; CLEANUP_FAILURE_REASON is included separately since it's the
// one member of this set where cleanup specifically did not.
const ROLLOUT_SUCCESS_REASONS = [
  'Deployed',
  'AlreadyRunning',
  'PostDeployHookFailed',
  'DeployedMetricRecordFailed',
  CLEANUP_FAILURE_REASON,
]

const NOT_LATEST_ATTEMPT_DETAIL =
  "Not tracked for past attempts: reconcile status only reflects the app's current state."

type RolloutSubStage = [DeployStage, DeployStage, DeployStage]

function rolloutSubStageShells(
  status: DeployStageStatus,
  detail?: string,
): RolloutSubStage {
  return [
    { key: 'health-check', label: 'Health check', status, detail },
    { key: 'cutover', label: 'Cutover', status, detail },
    { key: 'cleanup', label: 'Cleanup', status, detail },
  ]
}

// computeRolloutSubStages splits the single "Roll out" stage
// computeDeployStages already returns into the three real steps a blue-
// green/rolling reconcile pass actually takes (controller.go: get a new
// replica running and passing its readiness probe, confirm it as the
// container now serving, then remove the old one). This is additive,
// not a replacement: computeDeployStages' own 2-tuple return shape is a
// real, typed contract other callers (DeployMetaCard, DeployStageTimeline,
// DeployAttemptsList, DeployInProgressBanner, useDeployProgress) already
// depend on, so it stays exactly as-is; this is a second, separate view
// over the same underlying conditions for callers that want the finer
// breakdown (currently just the deploy log page).
//
// Real limitation, not a simplification: internal/reconcile's Engine
// persists exactly one terminal condition per Reconcile pass (see
// engine.go's reconcileOne), not one per sub-step, so which of the three
// is currently in flight is not observable while a rollout is still
// converging. All three report 'pending' until a terminal condition
// lands, then are back-filled from its Reason in one step, the same
// honest "derived from data that already exists, nothing new invented"
// approach computeRolloutStage above already takes.
export function computeRolloutSubStages(
  attempt: DeployAttempt,
  conditions: ReconcileCondition[],
  isLatestAttempt: boolean,
): RolloutSubStage {
  if (attempt.status === 'failed') {
    return rolloutSubStageShells(
      'skipped',
      'The build failed before a roll out could start.',
    )
  }
  if (attempt.status === 'running') {
    return rolloutSubStageShells('pending')
  }
  if (!isLatestAttempt) {
    return rolloutSubStageShells('unknown', NOT_LATEST_ATTEMPT_DETAIL)
  }

  const referenceMs = attempt.finished_at
    ? new Date(attempt.finished_at).getTime()
    : null

  const failedCondition = conditions.find(
    (c) =>
      HEALTH_CHECK_FAILURE_REASONS.includes(c.Reason) &&
      isAtOrAfter(c.LastTransitionTime, referenceMs),
  )
  if (failedCondition) {
    return [
      {
        key: 'health-check',
        label: 'Health check',
        status: 'failed',
        detail: failedCondition.Message,
        startedAt: attempt.finished_at,
        finishedAt: failedCondition.LastTransitionTime,
      },
      {
        key: 'cutover',
        label: 'Cutover',
        status: 'skipped',
        detail: 'The new container never passed its health check.',
      },
      {
        key: 'cleanup',
        label: 'Cleanup',
        status: 'skipped',
        detail: 'The new container never passed its health check.',
      },
    ]
  }

  const successCondition = conditions.find(
    (c) =>
      ROLLOUT_SUCCESS_REASONS.includes(c.Reason) &&
      isAtOrAfter(c.LastTransitionTime, referenceMs),
  )
  if (successCondition) {
    const cleanupFailed = successCondition.Reason === CLEANUP_FAILURE_REASON
    return [
      {
        key: 'health-check',
        label: 'Health check',
        status: 'done',
        startedAt: attempt.finished_at,
        finishedAt: successCondition.LastTransitionTime,
      },
      {
        key: 'cutover',
        label: 'Cutover',
        status: 'done',
        startedAt: attempt.finished_at,
        finishedAt: successCondition.LastTransitionTime,
      },
      {
        key: 'cleanup',
        label: 'Cleanup',
        status: cleanupFailed ? 'failed' : 'done',
        detail: cleanupFailed ? successCondition.Message : undefined,
        startedAt: attempt.finished_at,
        finishedAt: successCondition.LastTransitionTime,
      },
    ]
  }

  return [
    {
      key: 'health-check',
      label: 'Health check',
      status: 'running',
      startedAt: attempt.finished_at,
    },
    { key: 'cutover', label: 'Cutover', status: 'pending' },
    { key: 'cleanup', label: 'Cleanup', status: 'pending' },
  ]
}
