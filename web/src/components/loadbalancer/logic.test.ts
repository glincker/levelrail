import { describe, expect, it } from 'vitest'
import { formFromConfig } from '../../lib/loadBalancer'
import type { LoadBalancerConfig } from '../../queries/appLoadBalancer'
import type { LiveUpstream } from '../../queries/loadBalancerLive'
import { detectionTimes, weightShares } from './detection'
import { LB_PRESETS, presetById } from './presets'
import { rollupPool, upstreamShares, upstreamView } from './rollup'
import { predictEffect, summarizeChanges } from './changes'
import { computeSuggestions, p95 } from './suggestions'

function up(id: number, patch: Partial<LiveUpstream> = {}): LiveUpstream {
  return {
    id: `a#${id}`,
    dial: `10.0.0.${id}:80`,
    replica: id,
    weight: 0,
    state: 'healthy',
    healthy: true,
    active_connections: 0,
    fails: 0,
    ...patch,
  }
}
const down = (id: number) => up(id, { state: 'unhealthy', healthy: false })

describe('rollupPool', () => {
  const cases: [string, LiveUpstream[], string, boolean][] = [
    ['none', [], 'idle', false],
    ['all healthy', [up(0), up(1)], 'balancing', false],
    ['one down', [up(0), down(1)], 'degraded', false],
    ['all down', [down(0), down(1)], 'down', true],
    [
      'disabled does not count',
      [up(0), up(1, { admin_state: 'disabled' })],
      'degraded',
      false,
    ],
  ]
  it.each(cases)('%s', (_n, ups, level, allDown) => {
    const r = rollupPool(ups)
    expect(r.level).toBe(level)
    expect(r.allDown).toBe(allDown)
  })
  it('counts healthy of total', () => {
    expect(rollupPool([up(0), down(1), up(2)])).toMatchObject({
      healthy: 2,
      total: 3,
    })
  })
})

describe('upstreamView', () => {
  it('always carries a reason for non-healthy states', () => {
    const states = [
      down(0),
      up(1, { state: 'draining', healthy: false }),
      up(2, { admin_state: 'disabled' }),
      up(3, { state: 'unknown', healthy: false }),
    ]
    for (const u of states) {
      expect(upstreamView(u).reason.length).toBeGreaterThan(0)
    }
    const disabled = upstreamView(
      up(4, {
        state: 'disabled',
        healthy: false,
        reason: 'disabled by operator',
      }),
    )
    const draining = upstreamView(up(5, { state: 'draining', healthy: false }))
    expect(disabled.glyph).toBe('off')
    expect(draining.glyph).toBe('warn')
    expect(disabled.label).not.toBe(draining.label)
    expect(upstreamView(up(0)).glyph).toBe('ok')
    expect(upstreamView(down(0)).glyph).toBe('down')
  })
})

describe('detectionTimes', () => {
  const cases: [Record<string, string>, number, number][] = [
    [{ healthInterval: '5s', healthFails: '3', healthPasses: '2' }, 15, 10],
    [{ healthInterval: '10s', healthFails: '', healthPasses: '' }, 10, 10],
    [{ healthInterval: '3s', healthFails: '2', healthPasses: '3' }, 6, 9],
  ]
  it.each(cases)('%j', (patch, detect, recover) => {
    const f = { ...formFromConfig(undefined, 2), healthEnabled: true, ...patch }
    const d = detectionTimes(f)
    expect(d?.detectSeconds).toBe(detect)
    expect(d?.recoverSeconds).toBe(recover)
  })
  it('is null without a health check', () => {
    expect(detectionTimes(formFromConfig(undefined, 2))).toBeNull()
  })
  it('renders text', () => {
    const f = {
      ...formFromConfig(undefined, 2),
      healthEnabled: true,
      healthInterval: '5s',
      healthFails: '3',
      healthPasses: '2',
    }
    expect(detectionTimes(f)?.text).toBe('Down in about 15s, back in about 10s')
  })
})

describe('shares', () => {
  it('weightShares sums to 100', () => {
    expect(weightShares([9, 1])).toEqual([90, 10])
    expect(weightShares([1, 1, 1])).toEqual([34, 33, 33])
    expect(weightShares([0, 0])).toEqual([0, 0])
  })
  it('upstreamShares splits evenly among healthy and by weight when weighted', () => {
    const ups = [up(0, { weight: 9 }), up(1, { weight: 1 }), down(2)]
    expect(upstreamShares(ups, 'least_conn').get('a#0')).toBe(50)
    expect(upstreamShares(ups, 'weighted').get('a#0')).toBe(90)
    expect(upstreamShares(ups, 'weighted').get('a#2')).toBe(0)
  })
})

describe('presets match the brief', () => {
  it('balanced', () => {
    expect(presetById('balanced')?.config).toEqual({
      algorithm: 'least_conn',
      active_health: {
        path: '/healthz',
        interval: '5s',
        timeout: '2s',
        passes: 2,
        fails: 3,
        expect_status: 200,
      },
      passive_health: { max_fails: 3, fail_duration: '30s' },
      retries: { count: 2, try_duration: '5s' },
      drain_timeout: '15s',
    })
  })
  it('canary', () => {
    const c = presetById('canary')?.config
    expect(c?.weights).toEqual([9, 1])
    expect(c?.slow_start).toBe('30s')
    expect(c?.passive_health).toEqual({ max_fails: 2, fail_duration: '20s' })
  })
  it('websockets has retries off and drain 60s', () => {
    const c = presetById('websockets')?.config
    expect(c?.retries?.count).toBe(0)
    expect(c?.drain_timeout).toBe('60s')
    expect(c?.request_timeout).toBeUndefined()
  })
  it('others', () => {
    expect(presetById('sticky')?.config).toMatchObject({
      algorithm: 'cookie',
      cookie_name: 'lb',
      drain_timeout: '30s',
    })
    expect(presetById('cache')?.config.algorithm).toBe('uri_hash')
    expect(presetById('bluegreen')?.config.active_health).toMatchObject({
      interval: '3s',
      timeout: '1s',
      passes: 3,
      fails: 2,
    })
    expect(presetById('apiguard')?.config).toMatchObject({
      rate_limit: { rps: 50, burst: 100 },
      request_timeout: '30s',
    })
    expect(LB_PRESETS).toHaveLength(7)
  })
})

describe('summarizeChanges', () => {
  const before = formFromConfig(
    { algorithm: 'round_robin', active_health: { path: '/', interval: '10s' } },
    2,
  )
  it('is empty for identical forms', () => {
    expect(summarizeChanges(before, before)).toEqual([])
  })
  it('lists before to after chips and flags risk', () => {
    const after = {
      ...before,
      algorithm: 'cookie' as const,
      cookieName: 'lb',
      healthInterval: '5s',
    }
    const chips = summarizeChanges(before, after)
    const interval = chips.find((c) => c.key === 'interval')
    expect(interval).toMatchObject({ before: '10s', after: '5s' })
    expect(chips.find((c) => c.key === 'algorithm')?.risky).toBeTruthy()
    expect(predictEffect(after, chips)).toContain('sticky cookie')
  })
  it('flags disabling health', () => {
    const chips = summarizeChanges(before, { ...before, healthEnabled: false })
    expect(chips.some((c) => c.risky)).toBe(true)
  })
})

describe('computeSuggestions', () => {
  const base = {
    replicas: 2,
    upstreams: [up(0), up(1)],
    now: Date.parse('2026-01-01T00:10:00Z'),
  }
  const ids = (cfg: LoadBalancerConfig, extra = {}) =>
    computeSuggestions({ ...base, config: cfg, ...extra }).map((s) => s.id)

  it('no health check on multiple replicas', () => {
    expect(ids({ algorithm: 'round_robin' })).toContain('no-health')
  })
  it('single replica', () => {
    expect(ids({ algorithm: 'round_robin' }, { replicas: 1 })).toContain(
      'single-replica',
    )
  })
  it('caps at three', () => {
    expect(
      ids({ algorithm: 'ip_hash', weights: [3, 1] }).length,
    ).toBeLessThanOrEqual(3)
  })
  it('ip hash and ignored weights', () => {
    const all = computeSuggestions({
      ...base,
      config: {
        algorithm: 'ip_hash',
        weights: [3, 1],
        active_health: { path: '/' },
        passive_health: {},
        drain_timeout: '5s',
      },
    })
    expect(all.map((s) => s.id)).toEqual(
      expect.arrayContaining(['weights-ignored', 'iphash-proxy']),
    )
  })
  it('flapping needs 3 transitions in 10 minutes', () => {
    const t = (min: number) => ({
      at: `2026-01-01T00:0${min}:00Z`,
      from: 'healthy',
      to: 'unhealthy',
    })
    const history = {
      upstreams: [
        {
          id: 'a#0',
          dial: '',
          admin_state: 'active' as const,
          checks: [],
          transitions: [t(5), t(6), t(7)],
          series: { connections: [], latency_ms: [], fails: [] },
        },
      ],
    }
    const cfg: LoadBalancerConfig = {
      algorithm: 'round_robin',
      active_health: { path: '/', fails: 1 },
    }
    expect(ids(cfg, { history })).toContain('flapping')
    expect(ids(cfg)).not.toContain('flapping')
    const flap = computeSuggestions({ ...base, config: cfg, history }).find(
      (s) => s.id === 'flapping',
    )
    expect(flap?.apply?.(cfg).active_health?.fails).toBe(3)
  })
  it('tight timeout', () => {
    const cfg: LoadBalancerConfig = {
      algorithm: 'round_robin',
      active_health: { path: '/', interval: '10s', timeout: '1s' },
    }
    const upstreams = [up(0, { latency_ms: 700 }), up(1, { latency_ms: 800 })]
    const s = computeSuggestions({ ...base, config: cfg, upstreams }).find(
      (x) => x.id === 'timeout-tight',
    )
    expect(s?.apply?.(cfg).active_health?.timeout).toBe('2s')
  })
  it('applying no-health adds the balanced block', () => {
    const s = computeSuggestions({
      ...base,
      config: { algorithm: 'round_robin' },
    }).find((x) => x.id === 'no-health')
    expect(s?.apply?.({ algorithm: 'round_robin' }).active_health?.path).toBe(
      '/healthz',
    )
  })
  it('p95 helper', () => {
    expect(p95([])).toBe(0)
    expect(p95([1, 2, 3, 4, 100])).toBe(100)
  })
})
