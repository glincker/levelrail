import { describe, expect, it } from 'vitest'
import {
  MAX_POLLS,
  POLL_INTERVAL_MS,
  domainLiveState,
  nextPollInterval,
  shouldShowSetupCard,
} from './proxyIntegration'
import { proxyDomain, proxyIntegration } from '../test/proxyTestUtils'

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
