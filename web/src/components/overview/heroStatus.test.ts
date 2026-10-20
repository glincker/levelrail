import { describe, expect, it } from 'vitest'
import type { ReconcileCondition } from '../../types/deploy'
import { deriveHeroStatus, type HeroStatusInput } from './heroStatus'

const cond = (over: Partial<ReconcileCondition>): ReconcileCondition => ({
  Type: 'Ready',
  Status: 'True',
  Reason: 'Deployed',
  Message: '',
  LastTransitionTime: '2026-01-01T00:00:00Z',
  ...over,
})

const base: HeroStatusInput = {
  conditions: [cond({})],
  suspended: false,
  envDirty: false,
  deploying: false,
}

describe('deriveHeroStatus', () => {
  const cases: [string, Partial<HeroStatusInput>, string, string, boolean][] = [
    ['healthy', {}, 'Healthy', 'success', false],
    [
      'stopped wins over everything',
      { suspended: true, envDirty: true, deploying: true },
      'Stopped',
      'neutral',
      false,
    ],
    [
      'deploying with phase',
      { deploying: true, deployPhase: 'building image' },
      'Deploying: building image',
      'info',
      true,
    ],
    ['deploying without phase', { deploying: true }, 'Deploying', 'info', true],
    [
      'deploying beats failing conditions',
      { deploying: true, conditions: [cond({ Status: 'False' })] },
      'Deploying',
      'info',
      true,
    ],
    [
      'failing condition',
      { conditions: [cond({ Status: 'False', Reason: 'ProbeFailed' })] },
      'Needs attention',
      'danger',
      false,
    ],
    [
      'failing condition beats env dirty',
      { envDirty: true, conditions: [cond({ Status: 'False' })] },
      'Needs attention',
      'danger',
      false,
    ],
    [
      'reconciling',
      { conditions: [cond({ Status: 'Unknown', Reason: 'Pending' })] },
      'Starting up',
      'info',
      true,
    ],
    [
      'no conditions',
      { conditions: [] },
      'Waiting for first status',
      'neutral',
      false,
    ],
    [
      'env dirty while healthy',
      { envDirty: true },
      'Restart to apply changes',
      'warning',
      false,
    ],
    [
      'last deploy failed while healthy',
      { latestAttemptStatus: 'failed' },
      'Running, last deploy failed',
      'warning',
      false,
    ],
    [
      'env dirty beats failed attempt',
      { envDirty: true, latestAttemptStatus: 'failed' },
      'Restart to apply changes',
      'warning',
      false,
    ],
    [
      'optional feature unconfigured stays healthy',
      {
        conditions: [
          cond({}),
          cond({
            Type: 'EgressPolicyReady',
            Status: 'Unknown',
            Reason: 'NotConfigured',
          }),
        ],
      },
      'Healthy',
      'success',
      false,
    ],
  ]
  it.each(cases)('%s', (_name, over, label, tone, live) => {
    const s = deriveHeroStatus({ ...base, ...over })
    expect(s.label).toBe(label)
    expect(s.tone).toBe(tone)
    expect(s.live).toBe(live)
  })

  it('never mixes a healthy label with a pending or reconciling one', () => {
    const suspendedOpts = [true, false]
    const dirty = [true, false]
    const deploying = [true, false]
    const conds = [
      [],
      [cond({})],
      [cond({ Status: 'False' })],
      [cond({ Status: 'Unknown', Reason: 'Pending' })],
    ]
    for (const suspended of suspendedOpts)
      for (const envDirty of dirty)
        for (const d of deploying)
          for (const conditions of conds) {
            const s = deriveHeroStatus({
              ...base,
              suspended,
              envDirty,
              deploying: d,
              conditions,
            })
            expect(s.label).not.toMatch(/Healthy.*(Reconcil|Restart|Deploy)/)
            expect(
              s.tone === 'success' ? !envDirty && !d && !suspended : true,
            ).toBe(true)
          }
  })
})
