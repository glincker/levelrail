import { describe, expect, it } from 'vitest'
import { dnsPillState, proxyCertTrusted } from './dnsPill'

describe('dnsPillState', () => {
  it.each([
    ['connected', true, false, 'cert_pending'],
    ['connected', true, true, 'connected'],
    ['connected', false, false, 'connected'],
    ['not_resolving', true, true, 'not_resolving'],
    ['resolves_elsewhere', true, false, 'resolves_elsewhere'],
    ['propagating', true, true, 'propagating'],
  ] as const)('%s proxy=%s trusted=%s -> %s', (s, p, c, want) => {
    expect(dnsPillState(s, p, c)).toBe(want)
  })
})

describe('proxyCertTrusted', () => {
  it('is false without a row or certificate', () => {
    expect(proxyCertTrusted(undefined, undefined)).toBe(false)
  })
  it('trusts a healthy served certificate when no row exists', () => {
    expect(proxyCertTrusted(undefined, { status: 'healthy' })).toBe(true)
    expect(proxyCertTrusted(undefined, { status: 'expired' })).toBe(false)
  })
})
