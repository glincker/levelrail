import { describe, expect, it } from 'vitest'
import {
  MAX_POLLS,
  POLL_INTERVAL_MS,
  certWording,
  recheckDelayMs,
  tlsSummary,
  domainLiveState,
  nextPollInterval,
  shouldShowSetupCard,
} from './proxyIntegration'
import { proxyDomain, proxyIntegration } from '../test/proxyTestUtils'
import type { ProxyDomain } from '../queries/proxyIntegration'

describe('domainLiveState', () => {
  it.each([
    ['missing', { state: 'missing', certificate: null }, 'todo'],
    ['stale', { state: 'stale' }, 'todo'],
    ['error', { state: 'error' }, 'error'],
    ['last_error', { last_error: 'boom' }, 'error'],
    [
      'written, proxy silent',
      { reachable: false, proxy_loaded: null, certificate: null },
      'handled',
    ],
    ['no valid cert yet', { certificate: null }, 'waiting'],
    ['valid cert, reachable', {}, 'live'],
  ] as const)('%s -> %s', (_name, over, want) => {
    expect(domainLiveState(proxyDomain({ ...over }))).toBe(want)
  })
})

describe('nextPollInterval', () => {
  const waiting = proxyIntegration({
    domains: [proxyDomain({ certificate: null })],
  })

  it('polls while a domain is waiting', () => {
    expect(nextPollInterval(waiting, 0)).toBe(POLL_INTERVAL_MS)
  })

  it('stops once every domain is live', () => {
    const live = proxyIntegration({ domains: [proxyDomain()] })
    expect(nextPollInterval(live, 0)).toBe(false)
  })

  it('stops when nothing is written yet', () => {
    expect(nextPollInterval(proxyIntegration(), 0)).toBe(false)
  })

  it('stops after the poll budget is spent', () => {
    expect(nextPollInterval(waiting, MAX_POLLS)).toBe(false)
  })

  it('does not poll without a proxy or on an unavailable server', () => {
    expect(nextPollInterval(null, 0)).toBe(false)
    expect(nextPollInterval(undefined, 0)).toBe(false)
  })
})

describe('shouldShowSetupCard', () => {
  it('hides when no proxy is detected or everything is live', () => {
    const none = proxyIntegration()
    none.detected.kind = 'none'
    expect(shouldShowSetupCard(none)).toBe(false)
    expect(
      shouldShowSetupCard(proxyIntegration({ domains: [proxyDomain()] })),
    ).toBe(false)
    expect(shouldShowSetupCard(proxyIntegration())).toBe(true)
  })
})

describe('tlsSummary and certWording', () => {
  const base = {
    domain: 'a.example.com',
    target: 'app' as const,
    app: 'web',
    file: 'f.yml',
    state: 'written' as const,
    proxy_loaded: true,
    reachable: true,
    last_error: '',
    checked_at: '2026-11-01T10:00:00Z',
  }
  const cert = (over: Partial<NonNullable<ProxyDomain['certificate']>>) => ({
    issuer: 'TRAEFIK DEFAULT CERT',
    not_after: '2027-11-02T00:00:00Z',
    valid: false,
    trusted: false,
    expired: false,
    ...over,
  })

  it('calls a future untrusted certificate untrusted, not expired', () => {
    const d: ProxyDomain = { ...base, certificate: cert({}) }
    expect(certWording(d)).toBe('untrusted')
    expect(certWording({ ...d, certificate: cert({ expired: true }) })).toBe(
      'expired',
    )
    expect(
      certWording({
        ...d,
        certificate: cert({ valid: true, trusted: true }),
      }),
    ).toBe('valid')
  })

  it('picks one summary per state', () => {
    expect(
      tlsSummary({
        ...base,
        certificate: cert({ valid: true, trusted: true }),
      }),
    ).toBe('ready')
    expect(
      tlsSummary({
        ...base,
        certificate: cert({}),
        diagnosis: 'acme_pending',
        last_error: 'x',
      }),
    ).toBe('waiting')
    expect(
      tlsSummary({
        ...base,
        certificate: cert({}),
        diagnosis: 'acme_failed',
        last_error: 'x',
      }),
    ).toBe('default')
    expect(tlsSummary({ ...base, state: 'missing', certificate: null })).toBe(
      'unwired',
    )
  })

  it('backs off up to a cap', () => {
    expect(recheckDelayMs(0)).toBe(5_000)
    expect(recheckDelayMs(10)).toBe(30_000)
  })
})
