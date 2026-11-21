import { describe, expect, it } from 'vitest'
import {
  DEFAULT_CAPABILITIES,
  INITIAL_WIZARD,
  accountStatusState,
  canLeaveStep,
  invalidRegions,
  nextStep,
  parseRegions,
  prevStep,
  scopeIssues,
  toggleCapability,
  type AwsWizardState,
} from './cloudAccounts'

const PRINCIPAL = 'arn:aws:iam::123456789012:role/levelrail'
const ROLE = 'arn:aws:iam::123456789012:role/LevelrailAccess'

function ready(over: Partial<AwsWizardState> = {}): AwsWizardState {
  return { ...INITIAL_WIZARD, trustedPrincipalArn: PRINCIPAL, ...over }
}

describe('aws link wizard rules', () => {
  it('keeps destructive off by default', () => {
    expect(DEFAULT_CAPABILITIES).not.toContain('destructive')
    expect(DEFAULT_CAPABILITIES).toContain('inventory')
  })

  it('toggles a capability and keeps canonical order', () => {
    const on = toggleCapability(['dns', 'inventory'], 's3')
    expect(on).toEqual(['inventory', 's3', 'dns'])
    expect(toggleCapability(on, 's3')).toEqual(['inventory', 'dns'])
  })

  it('parses and dedupes the regions allowlist', () => {
    expect(parseRegions(' eu-west-1, US-EAST-1  eu-west-1\n')).toEqual([
      'eu-west-1',
      'us-east-1',
    ])
    expect(invalidRegions(['eu-west-1', 'mars'])).toEqual(['mars'])
  })

  it('reports scope issues before the kit is generated', () => {
    expect(scopeIssues(INITIAL_WIZARD)).toEqual(['badPrincipal'])
    expect(
      scopeIssues(
        ready({
          capabilities: [],
          regionsText: 'nowhere',
          guardPolicyArn: 'x',
        }),
      ),
    ).toEqual(['noCapabilities', 'badRegions', 'badGuard'])
    expect(scopeIssues(ready())).toEqual([])
  })

  it('gates each step on its own input', () => {
    expect(canLeaveStep('scope', ready(), false)).toBe(true)
    expect(canLeaveStep('kit', ready(), false)).toBe(false)
    expect(canLeaveStep('kit', ready(), true)).toBe(true)
    expect(canLeaveStep('connect', ready({ roleArn: 'nope' }), true)).toBe(
      false,
    )
    expect(canLeaveStep('connect', ready({ roleArn: ROLE }), true)).toBe(true)
  })

  it('walks steps without leaving the range', () => {
    expect(nextStep('scope')).toBe('kit')
    expect(nextStep('connect')).toBe('connect')
    expect(prevStep('scope')).toBe('scope')
    expect(prevStep('connect')).toBe('kit')
  })

  it('maps account status to a badge state', () => {
    expect(accountStatusState('active')).toBe('healthy')
    expect(accountStatusState('error')).toBe('failed')
    expect(accountStatusState('disabled')).toBe('paused')
  })
})
