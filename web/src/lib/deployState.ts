import type { DeployAttempt } from '../types/deployAttempt'
import type { ReconcileCondition } from '../types/deploy'
import type { Tone } from '../components/kit/tone'
import type { DeployStage } from './deployStages'

export type DeployPhase =
  | 'queued'
  | 'held'
  | 'awaiting_approval'
  | 'building'
  | 'rolling_out'
  | 'live'
  | 'failed'
  | 'rollout_failed'
  | 'canceled'
  | 'superseded'
  | 'untracked'

/** A translation key under the deployDetail namespace plus its values. */
export interface MessageRef {
  key: string
  values?: Record<string, string | number>
}

export type DeployAction =
  | 'containerLogs'
  | 'healthPath'
  | 'network'
  | 'rollback'
  | 'redeploy'
  | 'buildTab'

export interface DeployState {
  phase: DeployPhase
  tone: Tone
  /** The one status label shown in place of the raw attempt badge. */
  label: MessageRef
  /** What is happening right now. */
  headline: MessageRef
  /** What happens after this, or what to do. */
  next?: MessageRef
  /** Set when two signals disagree, so the page names the mismatch. */
  disagreement?: MessageRef
  stuck: boolean
  stuckCause?: MessageRef
  /** Reconcile reason that is holding the rollout, when one is known. */
  waitingReason?: string
  elapsedMs?: number
  /** The threshold that was exceeded, set only while stuck. */
  stuckAfterMs?: number
  actions: DeployAction[]
  isStatic: boolean
}

export interface DeployStateInput {
  attempt: DeployAttempt
  stages: [DeployStage, DeployStage]
  conditions: ReconcileCondition[]
  isLatestAttempt: boolean
  isStatic: boolean
  appStatusLabel: string
  nowMs: number
  stuckAfterMs: number
}

const AWAITING_FIRST_BUILD = 'AwaitingFirstBuild'

function ms(iso?: string): number | null {
  if (!iso) return null
  const t = new Date(iso).getTime()
  return Number.isFinite(t) ? t : null
}

function elapsedSince(
  iso: string | undefined,
  nowMs: number,
): number | undefined {
  const start = ms(iso)
  return start === null ? undefined : Math.max(0, nowMs - start)
}

function fmt(totalMs: number): string {
  const s = Math.round(totalMs / 1000)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s % 60}s`
  return `${s}s`
}

function findWaitingReason(
  conditions: ReconcileCondition[],
): string | undefined {
  const awaiting = conditions.find((c) => c.Reason === AWAITING_FIRST_BUILD)
  if (awaiting) return awaiting.Reason
  return conditions.find((c) => c.Status === 'Unknown')?.Reason
}

function base(
  isStatic: boolean,
  over: Omit<DeployState, 'stuck' | 'actions' | 'isStatic'> &
    Partial<Pick<DeployState, 'stuck' | 'actions'>>,
): DeployState {
  return { stuck: false, actions: [], isStatic, ...over }
}

// One authoritative state from the attempt row, stage statuses and reconcile
// conditions, so the page never shows "Succeeded" next to "Running".
export function deriveDeployState(input: DeployStateInput): DeployState {
  const { attempt, stages, conditions, isStatic, nowMs } = input
  const [build, rollout] = stages

  switch (attempt.status) {
    case 'queued':
      return base(isStatic, {
        phase: 'queued',
        tone: 'neutral',
        label: { key: 'state.label.queued' },
        headline: {
          key: attempt.wait_reason
            ? 'state.headline.queuedWhy'
            : 'state.headline.queued',
          values: { reason: attempt.wait_reason ?? '' },
        },
        next: { key: 'state.next.queued' },
      })
    case 'held':
      return base(isStatic, {
        phase: 'held',
        tone: 'warning',
        label: { key: 'state.label.held' },
        headline: {
          key: 'state.headline.held',
          values: { reason: attempt.reason ?? '' },
        },
        next: { key: 'state.next.held' },
      })
    case 'awaiting_approval':
      return base(isStatic, {
        phase: 'awaiting_approval',
        tone: 'warning',
        label: { key: 'state.label.awaitingApproval' },
        headline: { key: 'state.headline.awaitingApproval' },
      })
    case 'canceled':
      return base(isStatic, {
        phase: 'canceled',
        tone: 'neutral',
        label: { key: 'state.label.canceled' },
        headline: { key: 'state.headline.canceled' },
      })
    case 'superseded':
      return base(isStatic, {
        phase: 'superseded',
        tone: 'neutral',
        label: { key: 'state.label.superseded' },
        headline: { key: 'state.headline.superseded' },
      })
    case 'failed':
      return base(isStatic, {
        phase: 'failed',
        tone: 'danger',
        label: { key: 'state.label.failed' },
        headline: {
          key: attempt.error
            ? 'state.headline.failedWhy'
            : 'state.headline.failed',
          values: { error: attempt.error ?? '' },
        },
        next: { key: 'state.next.failed' },
        actions: ['buildTab', 'redeploy'],
      })
    case 'running': {
      const elapsedMs = elapsedSince(attempt.started_at, nowMs)
      return base(isStatic, {
        phase: 'building',
        tone: 'info',
        label: { key: 'state.label.building' },
        headline: {
          key: 'state.headline.building',
          values: { elapsed: fmt(elapsedMs ?? 0) },
        },
        next: {
          key: isStatic ? 'state.next.buildingStatic' : 'state.next.building',
        },
        elapsedMs,
      })
    }
    case 'succeeded':
      break
  }

  if (rollout.status === 'failed') {
    return base(isStatic, {
      phase: 'rollout_failed',
      tone: 'danger',
      label: { key: 'state.label.rolloutFailed' },
      headline: {
        key: rollout.detail
          ? 'state.headline.rolloutFailedWhy'
          : 'state.headline.rolloutFailed',
        values: { reason: rollout.detail ?? '' },
      },
      disagreement: { key: 'state.mismatch.buildOkRolloutFailed' },
      next: { key: 'state.next.rolloutFailed' },
      actions: isStatic
        ? ['network', 'rollback']
        : ['containerLogs', 'healthPath', 'rollback'],
    })
  }

  if (isStatic) {
    const chipOff =
      input.appStatusLabel !== 'Healthy' && input.appStatusLabel !== ''
    return base(isStatic, {
      phase: 'live',
      tone: 'success',
      label: { key: 'state.label.published' },
      headline: { key: 'state.headline.staticPublished' },
      next: { key: 'state.next.staticPublished' },
      disagreement: chipOff
        ? {
            key: 'state.mismatch.staticChip',
            values: { chip: input.appStatusLabel },
          }
        : undefined,
      actions: ['network'],
    })
  }

  if (rollout.status === 'done') {
    return base(isStatic, {
      phase: 'live',
      tone: 'success',
      label: { key: 'state.label.live' },
      headline: { key: 'state.headline.live' },
    })
  }

  if (rollout.status === 'running') {
    const elapsedMs = elapsedSince(
      rollout.startedAt ?? attempt.finished_at,
      nowMs,
    )
    const waitingReason = findWaitingReason(conditions)
    const stuck = elapsedMs !== undefined && elapsedMs > input.stuckAfterMs
    const noImage = waitingReason === AWAITING_FIRST_BUILD
    const stuckKey = noImage ? 'state.stuck.noImage' : 'state.stuck.health'
    return base(isStatic, {
      phase: 'rolling_out',
      tone: stuck ? 'warning' : 'info',
      label: { key: 'state.label.rollingOut' },
      headline: {
        key: noImage
          ? 'state.headline.rollingOutNoImage'
          : 'state.headline.rollingOut',
        values: { elapsed: fmt(elapsedMs ?? 0) },
      },
      next: { key: 'state.next.rollingOut' },
      disagreement: { key: 'state.mismatch.buildOkRolloutPending' },
      stuck,
      stuckCause: stuck
        ? { key: stuckKey, values: { limit: fmt(input.stuckAfterMs) } }
        : undefined,
      waitingReason,
      elapsedMs,
      stuckAfterMs: stuck ? input.stuckAfterMs : undefined,
      actions: stuck
        ? noImage
          ? ['buildTab', 'redeploy', 'rollback']
          : ['containerLogs', 'healthPath', 'network', 'rollback']
        : [],
    })
  }

  // Latest-only conditions mean an older attempt's rollout is not knowable.
  return base(isStatic, {
    phase: 'untracked',
    tone: 'neutral',
    label: { key: 'state.label.buildDone' },
    headline: {
      key:
        build.status === 'skipped'
          ? 'state.headline.untrackedImage'
          : 'state.headline.untracked',
    },
  })
}
