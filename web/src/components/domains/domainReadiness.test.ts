import { describe, expect, it } from 'vitest'
import { allStepsDone, readinessSteps, ttlLabel } from './domainReadiness'

describe('readinessSteps', () => {
  it('treats public ready DNS as done even when this server lags', () => {
    const s = readinessSteps({
      status: 'propagating',
      verdict: 'public_ready',
      hasAcmeFailure: false,
      certTrusted: false,
    })
    expect(s.map((x) => x.state)).toEqual(['done', 'progress', 'waiting'])
  })
  it('blocks the certificate on an issuing failure', () => {
    const s = readinessSteps({
      status: 'connected',
      hasAcmeFailure: true,
      certTrusted: false,
    })
    expect(s[1].state).toBe('blocked')
    expect(allStepsDone(s)).toBe(false)
  })
  it('formats ttl in the largest sensible unit', () => {
    expect(ttlLabel(3600)).toEqual({ count: 1, unit: 'h' })
    expect(ttlLabel(300)).toEqual({ count: 5, unit: 'm' })
  })
})
