import { describe, expect, it } from 'vitest'
import {
  allStepsDone,
  readinessSteps,
  routeProblem,
  ttlLabel,
} from './domainReadiness'

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
    expect(s[1]?.state).toBe('blocked')
    expect(allStepsDone(s)).toBe(false)
  })
  it('formats ttl in the largest sensible unit', () => {
    expect(ttlLabel(3600)).toEqual({ count: 1, unit: 'h' })
    expect(ttlLabel(300)).toEqual({ count: 5, unit: 'm' })
  })
})

describe('route status', () => {
  const base = {
    status: 'connected' as const,
    hasAcmeFailure: false,
    certTrusted: true,
    tlsOwner: 'custom' as const,
  }
  const at = '2026-11-06T10:00:00Z'
  it('blocks the site step on a 503 route and names it', () => {
    const route = { status: 503, ok: false, checked_at: at }
    expect(readinessSteps({ ...base, route })[2]?.state).toBe('blocked')
    expect(routeProblem(undefined, route)).toBe('503')
  })
  it('names 404 and other 5xx answers', () => {
    expect(
      routeProblem(undefined, { status: 404, ok: false, checked_at: at }),
    ).toBe('404')
    expect(
      routeProblem(undefined, { status: 502, ok: false, checked_at: at }),
    ).toBe('5xx')
  })
  it('marks the site done on a 2xx or 3xx route', () => {
    for (const status of [200, 301]) {
      const route = { status, ok: true, checked_at: at }
      expect(readinessSteps({ ...base, route })[2]?.state).toBe('done')
      expect(routeProblem(undefined, route)).toBeUndefined()
    }
  })
  it('falls back to proxy row text when the route is absent', () => {
    const row = {
      reachable: false,
      last_error: 'upstream answered 503',
      evidence: [],
    } as never
    expect(routeProblem(row)).toBe('503')
  })
})
