import { describe, expect, it } from 'vitest'
import {
  configFromForm,
  formFromConfig,
  summarizePool,
  validateForm,
} from './loadBalancer'

describe('formFromConfig', () => {
  it('defaults an unconfigured balancer to round robin with one weight per replica', () => {
    const form = formFromConfig(undefined, 3)
    expect(form.algorithm).toBe('round_robin')
    expect(form.weights).toEqual([1, 1, 1])
    expect(form.healthEnabled).toBe(false)
  })

  it('keeps stored weights and extends to the replica count', () => {
    const form = formFromConfig({ algorithm: 'weighted', weights: [5, 2] }, 3)
    expect(form.weights).toEqual([5, 2, 1])
  })
})

describe('configFromForm', () => {
  it('emits only what the operator enabled', () => {
    const cfg = configFromForm(formFromConfig(undefined, 2))
    expect(cfg).toEqual({ algorithm: 'round_robin' })
  })

  it('round-trips a full config', () => {
    const full = {
      algorithm: 'weighted' as const,
      weights: [3, 1],
      active_health: {
        path: '/healthz',
        interval: '5s',
        timeout: '2s',
        passes: 2,
        fails: 3,
        expect_status: 204,
      },
      passive_health: { max_fails: 3, fail_duration: '30s' },
      retries: { count: 2, try_duration: '5s' },
      slow_start: '30s',
      drain_timeout: '15s',
      request_timeout: '30s',
      rate_limit: { rps: 20, burst: 40 },
      upstream_tls: { insecure_skip_verify: true, server_name: 'app' },
    }
    expect(configFromForm(formFromConfig(full, 2))).toEqual(full)
  })

  it('drops weights and slow start when the algorithm is not weighted', () => {
    const form = {
      ...formFromConfig(
        { algorithm: 'weighted', weights: [3, 1], slow_start: '30s' },
        2,
      ),
      algorithm: 'least_conn' as const,
    }
    expect(configFromForm(form)).toEqual({ algorithm: 'least_conn' })
  })

  it('keeps the cookie name only for the cookie algorithm', () => {
    const cookie = {
      ...formFromConfig(undefined, 1),
      algorithm: 'cookie' as const,
      cookieName: 'sid',
    }
    expect(configFromForm(cookie).cookie_name).toBe('sid')
    expect(
      configFromForm({ ...cookie, algorithm: 'ip_hash' }).cookie_name,
    ).toBeUndefined()
  })
})

describe('validateForm', () => {
  const base = formFromConfig(undefined, 1)

  it('accepts a default form', () => {
    expect(validateForm(base)).toEqual({})
  })

  it.each([
    ['bad path', { healthEnabled: true, healthPath: 'healthz' }, 'healthPath'],
    ['bad duration', { drainTimeout: 'soon' }, 'drainTimeout'],
    [
      'timeout not below interval',
      { healthEnabled: true, healthInterval: '2s', healthTimeout: '5s' },
      'healthTimeout',
    ],
    [
      'bad status',
      { healthEnabled: true, healthStatus: '999' },
      'healthStatus',
    ],
    [
      'too many retries',
      { retriesEnabled: true, retryCount: '11' },
      'retryCount',
    ],
    ['rate limit without rps', { rateEnabled: true, rps: '' }, 'rps'],
  ])('rejects %s', (_name, patch, key) => {
    expect(validateForm({ ...base, ...patch })).toHaveProperty(key)
  })
})

describe('summarizePool', () => {
  it('counts healthy upstreams', () => {
    expect(summarizePool([{ healthy: true }, { healthy: false }])).toBe(
      '1 of 2 healthy',
    )
  })
})
