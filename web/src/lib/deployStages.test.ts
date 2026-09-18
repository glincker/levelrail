import { describe, expect, it } from 'vitest'
import { computeDeployStages } from './deployStages'
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

function condition(reason: string, lastTransitionTime: string): ReconcileCondition {
  return {
    Type: 'Ready',
    Status: 'True',
    Reason: reason,
    Message: '',
    LastTransitionTime: lastTransitionTime,
  }
}

describe('computeDeployStages rollout stage', () => {
  const cases: { name: string; reason: string; transitionTime: string; want: string }[] = [
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
