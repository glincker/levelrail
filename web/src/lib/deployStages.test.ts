import { describe, expect, it } from 'vitest'
import { computeDeployStages, computeRolloutSubStages } from './deployStages'
import type { DeployAttempt } from '../types/deployAttempt'
import type { ReconcileCondition } from '../types/deploy'

const baseAttempt: DeployAttempt = {
  id: 'dep_1',
  service_name: 'web',
  image: 'nginx:latest',
  source: 'image',
  status: 'succeeded',
  started_at: '2026-01-01T00:00:00Z',
  finished_at: '2026-01-01T00:00:05Z',
}

function condition(
  reason: string,
  lastTransitionTime: string,
): ReconcileCondition {
  return {
    Type: 'Ready',
    Status: 'True',
    Reason: reason,
    Message: '',
    LastTransitionTime: lastTransitionTime,
  }
}

describe('computeDeployStages rollout stage', () => {
  const cases: {
    name: string
    reason: string
    transitionTime: string
    want: string
  }[] = [
    {
      name: 'marks rollout done on a Deployed condition after the attempt finished',
      reason: 'Deployed',
      transitionTime: '2026-01-01T00:00:10Z',
      want: 'done',
    },
    {
      name: 'marks rollout done on an AlreadyRunning condition, e.g. redeploying the image already running',
      reason: 'AlreadyRunning',
      transitionTime: '2026-01-01T00:00:10Z',
      want: 'done',
    },
    {
      name: 'stays running when the only matching condition predates the attempt finishing',
      reason: 'AlreadyRunning',
      transitionTime: '2025-12-31T00:00:00Z',
      want: 'running',
    },
    {
      name: 'marks rollout failed on a rollout failure reason after the attempt finished',
      reason: 'StartFailed',
      transitionTime: '2026-01-01T00:00:10Z',
      want: 'failed',
    },
    // Every one of these previously fell through to the 'running'
    // default (a permanently stuck spinner for an already-failed
    // deploy) because ROLLOUT_FAILURE_REASONS didn't list them, either
    // from day one (InspectFailed/EnsureNetworkFailed/
    // VanishedAfterStart/PreDeployHookFailed) or because it wasn't
    // updated when the reconciler learned these two new reasons
    // (OOMKilledDuringReadiness/ExitedDuringReadiness).
    {
      name: 'marks rollout failed on InspectFailed',
      reason: 'InspectFailed',
      transitionTime: '2026-01-01T00:00:10Z',
      want: 'failed',
    },
    {
      name: 'marks rollout failed on EnsureNetworkFailed',
      reason: 'EnsureNetworkFailed',
      transitionTime: '2026-01-01T00:00:10Z',
      want: 'failed',
    },
    {
      name: 'marks rollout failed on VanishedAfterStart',
      reason: 'VanishedAfterStart',
      transitionTime: '2026-01-01T00:00:10Z',
      want: 'failed',
    },
    {
      name: 'marks rollout failed on PreDeployHookFailed',
      reason: 'PreDeployHookFailed',
      transitionTime: '2026-01-01T00:00:10Z',
      want: 'failed',
    },
    {
      name: 'marks rollout failed on OOMKilledDuringReadiness',
      reason: 'OOMKilledDuringReadiness',
      transitionTime: '2026-01-01T00:00:10Z',
      want: 'failed',
    },
    {
      name: 'marks rollout failed on ExitedDuringReadiness',
      reason: 'ExitedDuringReadiness',
      transitionTime: '2026-01-01T00:00:10Z',
      want: 'failed',
    },
  ]

  it.each(cases)('$name', ({ reason, transitionTime, want }) => {
    const [, rollout] = computeDeployStages(
      baseAttempt,
      [condition(reason, transitionTime)],
      true,
    )
    expect(rollout.status).toBe(want)
  })
})

describe('computeRolloutSubStages', () => {
  it('marks all three sub-stages pending while the build is still running', () => {
    const [health, cutover, cleanup] = computeRolloutSubStages(
      { ...baseAttempt, status: 'running', finished_at: undefined },
      [],
      true,
    )
    expect([health.status, cutover.status, cleanup.status]).toEqual([
      'pending',
      'pending',
      'pending',
    ])
  })

  it('skips all three sub-stages when the build itself failed', () => {
    const [health, cutover, cleanup] = computeRolloutSubStages(
      { ...baseAttempt, status: 'failed' },
      [],
      true,
    )
    expect([health.status, cutover.status, cleanup.status]).toEqual([
      'skipped',
      'skipped',
      'skipped',
    ])
    expect(health.detail).toBe(
      'The build failed before a roll out could start.',
    )
  })

  it('marks unknown for a non-latest attempt', () => {
    const [health, cutover, cleanup] = computeRolloutSubStages(
      baseAttempt,
      [],
      false,
    )
    expect([health.status, cutover.status, cleanup.status]).toEqual([
      'unknown',
      'unknown',
      'unknown',
    ])
  })

  it('reports health check running and the rest pending while no terminal condition has landed', () => {
    const [health, cutover, cleanup] = computeRolloutSubStages(
      baseAttempt,
      [],
      true,
    )
    expect(health.status).toBe('running')
    expect(cutover.status).toBe('pending')
    expect(cleanup.status).toBe('pending')
  })

  it('marks health check failed and skips cutover/cleanup on a health-check failure reason', () => {
    const [health, cutover, cleanup] = computeRolloutSubStages(
      baseAttempt,
      [condition('ReadinessFailed', '2026-01-01T00:00:10Z')],
      true,
    )
    expect(health.status).toBe('failed')
    expect(cutover.status).toBe('skipped')
    expect(cleanup.status).toBe('skipped')
  })

  it('marks every reachable health-check failure reason as a health-check failure', () => {
    const reasons = [
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
    for (const reason of reasons) {
      const [health] = computeRolloutSubStages(
        baseAttempt,
        [condition(reason, '2026-01-01T00:00:10Z')],
        true,
      )
      expect(health.status).toBe('failed')
    }
  })

  it('marks all three done on a clean Deployed condition', () => {
    const [health, cutover, cleanup] = computeRolloutSubStages(
      baseAttempt,
      [condition('Deployed', '2026-01-01T00:00:10Z')],
      true,
    )
    expect([health.status, cutover.status, cleanup.status]).toEqual([
      'done',
      'done',
      'done',
    ])
  })

  it('marks all three done on AlreadyRunning, e.g. redeploying the already-running image', () => {
    const [health, cutover, cleanup] = computeRolloutSubStages(
      baseAttempt,
      [condition('AlreadyRunning', '2026-01-01T00:00:10Z')],
      true,
    )
    expect([health.status, cutover.status, cleanup.status]).toEqual([
      'done',
      'done',
      'done',
    ])
  })

  it('marks health check and cutover done but cleanup failed on RunningStaleCleanupFailed', () => {
    const [health, cutover, cleanup] = computeRolloutSubStages(
      baseAttempt,
      [condition('RunningStaleCleanupFailed', '2026-01-01T00:00:10Z')],
      true,
    )
    expect(health.status).toBe('done')
    expect(cutover.status).toBe('done')
    expect(cleanup.status).toBe('failed')
  })

  it('marks all three done on PostDeployHookFailed: cutover and cleanup already succeeded by then', () => {
    const [health, cutover, cleanup] = computeRolloutSubStages(
      baseAttempt,
      [condition('PostDeployHookFailed', '2026-01-01T00:00:10Z')],
      true,
    )
    expect([health.status, cutover.status, cleanup.status]).toEqual([
      'done',
      'done',
      'done',
    ])
  })

  it('ignores a matching condition that predates the attempt finishing', () => {
    const [health, cutover, cleanup] = computeRolloutSubStages(
      baseAttempt,
      [condition('Deployed', '2025-12-31T00:00:00Z')],
      true,
    )
    expect(health.status).toBe('running')
    expect(cutover.status).toBe('pending')
    expect(cleanup.status).toBe('pending')
  })
})
