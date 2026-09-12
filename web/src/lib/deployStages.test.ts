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
