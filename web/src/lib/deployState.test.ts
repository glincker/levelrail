import { describe, expect, it } from 'vitest'
import { deriveDeployState, type DeployStateInput } from './deployState'
import { computeDeployStages } from './deployStages'
import type { DeployAttempt } from '../types/deployAttempt'
import type { ReconcileCondition } from '../types/deploy'

const FINISHED = '2026-10-15T10:00:00Z'
const NOW = new Date('2026-10-15T10:05:12Z').getTime()

const attempt: DeployAttempt = {
  id: 'dep_1',
  service_name: 'site',
  image: 'site:1',
  status: 'succeeded',
  started_at: '2026-10-15T09:59:00Z',
  finished_at: FINISHED,
}

const awaiting: ReconcileCondition = {
  Type: 'Ready',
  Status: 'Unknown',
  Reason: 'AwaitingFirstBuild',
  Message: 'Awaiting first build',
  LastTransitionTime: '2026-10-15T09:58:00Z',
}

function input(over: Partial<DeployStateInput> = {}): DeployStateInput {
  const conditions = over.conditions ?? [awaiting]
  const a = over.attempt ?? attempt
  return {
    attempt: a,
    conditions,
    stages: computeDeployStages(a, conditions, true),
    isLatestAttempt: true,
    isStatic: false,
    appStatusLabel: 'Reconciling',
    nowMs: NOW,
    stuckAfterMs: 120_000,
    ...over,
  }
}

describe('deriveDeployState', () => {
  it('reconciles a succeeded build with a rollout stuck waiting for a first build', () => {
    const s = deriveDeployState(input())
    expect(s.phase).toBe('rolling_out')
    expect(s.label.key).toBe('state.label.rollingOut')
    expect(s.headline.key).toBe('state.headline.rollingOutNoImage')
    expect(s.headline.values?.elapsed).toBe('5m 12s')
    expect(s.disagreement?.key).toBe('state.mismatch.buildOkRolloutPending')
    expect(s.stuck).toBe(true)
    expect(s.stuckCause?.key).toBe('state.stuck.noImage')
    expect(s.waitingReason).toBe('AwaitingFirstBuild')
    expect(s.actions).toContain('redeploy')
  })

  it('is not stuck inside the threshold', () => {
    const s = deriveDeployState(input({ stuckAfterMs: 600_000 }))
    expect(s.stuck).toBe(false)
    expect(s.tone).toBe('info')
  })

  it('treats a static site as published with no container concepts', () => {
    const s = deriveDeployState(input({ isStatic: true }))
    expect(s.phase).toBe('live')
    expect(s.headline.key).toBe('state.headline.staticPublished')
    expect(s.stuck).toBe(false)
    expect(s.disagreement?.key).toBe('state.mismatch.staticChip')
    expect(s.actions).not.toContain('healthPath')
  })

  it('reports a failed rollout over a succeeded build', () => {
    const failing: ReconcileCondition = {
      ...awaiting,
      Status: 'False',
      Reason: 'ReadinessFailed',
      Message: 'probe timed out',
      LastTransitionTime: '2026-10-15T10:01:00Z',
    }
    const s = deriveDeployState(input({ conditions: [failing] }))
    expect(s.phase).toBe('rollout_failed')
    expect(s.headline.values?.reason).toBe('probe timed out')
  })

  it('reports a running build', () => {
    const running = {
      ...attempt,
      status: 'running' as const,
      finished_at: undefined,
    }
    expect(deriveDeployState(input({ attempt: running })).phase).toBe(
      'building',
    )
  })
})
