import { describe, expect, it } from 'vitest'
import { summarizeAppStatus } from './appStatus'
import type { ReconcileCondition } from '../types/deploy'

function condition(overrides: Partial<ReconcileCondition>): ReconcileCondition {
  return {
    Type: 'Ready',
    Status: 'True',
    Reason: 'AlreadyRunning',
    Message: '',
    LastTransitionTime: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('summarizeAppStatus', () => {
  it('reports no status yet for an empty conditions list', () => {
    expect(summarizeAppStatus([])).toEqual({
      label: 'No status yet',
      variant: 'muted',
    })
  })

  it('reports healthy when every condition is true', () => {
    expect(summarizeAppStatus([condition({})])).toEqual({
      label: 'Healthy',
      variant: 'success',
    })
  })

  it('reports attention needed for any false condition regardless of others', () => {
    const result = summarizeAppStatus([
      condition({}),
      condition({ Type: 'EgressPolicyReady', Status: 'False' }),
    ])
    expect(result).toEqual({
      label: 'Attention needed',
      variant: 'destructive',
    })
  })

  // The exact regression found via real deployment testing: an app with
  // no egress policy configured (the common case for virtually every
  // app) carries a permanently-Unknown EgressPolicyReady condition that
  // must not keep an otherwise-healthy app stuck on "Reconciling".
  it('reports healthy when the only non-true condition is an unconfigured optional feature', () => {
    const result = summarizeAppStatus([
      condition({}),
      condition({
        Type: 'EgressPolicyReady',
        Status: 'Unknown',
        Reason: 'NotConfigured',
      }),
    ])
    expect(result).toEqual({ label: 'Healthy', variant: 'success' })
  })

  it('reports healthy when the only non-true condition is a disabled optional integration', () => {
    const result = summarizeAppStatus([
      condition({}),
      condition({ Type: 'Ready', Status: 'Unknown', Reason: 'Disabled' }),
    ])
    expect(result).toEqual({ label: 'Healthy', variant: 'success' })
  })

  it('still reports reconciling for a genuinely pending unknown condition', () => {
    const result = summarizeAppStatus([
      condition({ Status: 'Unknown', Reason: 'AwaitingFirstBuild' }),
    ])
    expect(result).toEqual({ label: 'Reconciling', variant: 'muted' })
  })

  it('reports stopped when any condition reports Suspended, overriding everything else', () => {
    const result = summarizeAppStatus([
      condition({ Reason: 'Suspended' }),
      condition({ Type: 'EgressPolicyReady', Status: 'False' }),
    ])
    expect(result).toEqual({ label: 'Stopped', variant: 'muted' })
  })
})
