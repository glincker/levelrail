import { describe, expect, it } from 'vitest'
import { deriveConvergence } from './convergence'
import type { ReconcileCondition } from '../types/deploy'

function cond(
  Type: string,
  Status: ReconcileCondition['Status'],
  Reason = '',
): ReconcileCondition {
  return { Type, Status, Reason, Message: '' } as ReconcileCondition
}

describe('deriveConvergence', () => {
  it('does not stay reconciling because an optional feature was never configured', () => {
    expect(
      deriveConvergence([
        cond('Ready', 'True'),
        cond('EgressPolicyReady', 'Unknown', 'NotConfigured'),
        cond('LivenessReady', 'Unknown', 'Disabled'),
      ]),
    ).toBe('converged')
  })

  it('still reports reconciling while a real condition is unresolved', () => {
    expect(
      deriveConvergence([
        cond('Ready', 'True'),
        cond('Rollout', 'Unknown', 'InProgress'),
      ]),
    ).toBe('reconciling')
  })

  it('reports an error for a failed condition even beside optional ones', () => {
    expect(
      deriveConvergence([
        cond('Ready', 'False'),
        cond('Egress', 'Unknown', 'NotConfigured'),
      ]),
    ).toBe('error')
  })

  it('is unknown with no conditions or only unconfigured ones', () => {
    expect(deriveConvergence([])).toBe('unknown')
    expect(
      deriveConvergence([cond('Egress', 'Unknown', 'NotConfigured')]),
    ).toBe('unknown')
  })
})
