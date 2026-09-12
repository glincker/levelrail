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
  it('marks rollout done on a Deployed condition after the attempt finished', () => {
    const [, rollout] = computeDeployStages(
      baseAttempt,
      [condition('Deployed', '2026-01-01T00:00:10Z')],
      true,
    )
    expect(rollout.status).toBe('done')
  })

  it('marks rollout done on an AlreadyRunning condition, e.g. redeploying the image already running', () => {
    const [, rollout] = computeDeployStages(
      baseAttempt,
      [condition('AlreadyRunning', '2026-01-01T00:00:10Z')],
      true,
    )
    expect(rollout.status).toBe('done')
  })

  it('stays running when the only matching condition predates the attempt finishing', () => {
    const [, rollout] = computeDeployStages(
      baseAttempt,
      [condition('AlreadyRunning', '2025-12-31T00:00:00Z')],
      true,
    )
    expect(rollout.status).toBe('running')
  })

  it('marks rollout failed on a rollout failure reason after the attempt finished', () => {
    const [, rollout] = computeDeployStages(
      baseAttempt,
      [condition('StartFailed', '2026-01-01T00:00:10Z')],
      true,
    )
    expect(rollout.status).toBe('failed')
  })
})
