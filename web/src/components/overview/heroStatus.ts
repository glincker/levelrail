import { summarizeAppStatus } from '../../lib/appStatus'
import type { ReconcileCondition } from '../../types/deploy'
import type { DeployAttemptStatus } from '../../types/deployAttempt'
import type { Tone } from '@/components/kit'

export interface HeroStatusInput {
  conditions: ReconcileCondition[]
  suspended: boolean
  envDirty: boolean
  deploying: boolean
  /** Plain-words label of the running deploy stage, when deploying. */
  deployPhase?: string
  latestAttemptStatus?: DeployAttemptStatus
}

export interface HeroStatus {
  tone: Tone
  label: string
  live: boolean
  title?: string
}

/** Single source of truth for the hero pill: exactly one state wins. */
export function deriveHeroStatus(input: HeroStatusInput): HeroStatus {
  if (input.suspended) {
    return { tone: 'neutral', label: 'Stopped', live: false }
  }
  if (input.deploying) {
    return {
      tone: 'info',
      label: input.deployPhase
        ? `Deploying: ${input.deployPhase}`
        : 'Deploying',
      live: true,
    }
  }
  const summary = summarizeAppStatus(input.conditions)
  if (summary.label === 'Attention needed') {
    const failing = input.conditions.find((c) => c.Status === 'False')
    return {
      tone: 'danger',
      label: 'Needs attention',
      live: false,
      title: failing ? `${failing.Type}: ${failing.Reason}` : undefined,
    }
  }
  if (summary.label === 'Reconciling') {
    return { tone: 'info', label: 'Starting up', live: true }
  }
  if (summary.label === 'No status yet') {
    return { tone: 'neutral', label: 'Waiting for first status', live: false }
  }
  if (input.envDirty) {
    return { tone: 'warning', label: 'Restart to apply changes', live: false }
  }
  if (input.latestAttemptStatus === 'failed') {
    return {
      tone: 'warning',
      label: 'Running, last deploy failed',
      live: false,
    }
  }
  return { tone: 'success', label: 'Healthy', live: false }
}

const STAGE_PHASE: Record<string, string> = {
  build: 'building image',
  rollout: 'rolling out',
  'health-check': 'checking health',
  cutover: 'switching traffic',
  cleanup: 'cleaning up',
}

export function phaseWords(stageKey: string | undefined): string | undefined {
  return stageKey ? STAGE_PHASE[stageKey] : undefined
}
