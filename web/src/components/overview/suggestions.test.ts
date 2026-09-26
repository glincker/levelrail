import { describe, expect, it } from 'vitest'
import {
  computeSuggestions,
  computeSetup,
  type SuggestionInput,
} from './suggestions'
import type { TrafficStats } from '../../queries/appTraffic'

const zero = { requests: 0, ratePerSec: 0, errorRate: 0, p95Ms: 0 }
const traffic = (errorRate: number, requests = 100): TrafficStats => ({
  hasTraffic: requests > 0,
  current: { ...zero, requests, errorRate },
  previous: zero,
  rateSeries: [],
  errorSeries: [],
  p95Series: [],
})

const healthy: SuggestionInput = {
  app: {
    health: { readiness: { path: '/healthz' } },
    resources: { memory_bytes: 1 },
    domains: ['a.example.com'],
    env_dirty: false,
    suspended: false,
  },
  gitSourceKnown: true,
  hasGitSource: true,
  dismissed: new Set(),
}

const ids = (i: SuggestionInput) => computeSuggestions(i).map((s) => s.id)

describe('computeSuggestions', () => {
  it('shows nothing when nothing is actionable', () => {
    expect(ids(healthy)).toEqual([])
  })

  it('suggests a health check with the default path', () => {
    const [s] = computeSuggestions({
      ...healthy,
      app: { ...healthy.app, health: null },
    })
    expect(s?.id).toBe('no_health_check')
    expect(s?.action).toEqual({ kind: 'add_health', label: 'Add /healthz' })
  })

  it('does not suggest a health check for a stopped app', () => {
    expect(
      ids({
        ...healthy,
        app: { ...healthy.app, health: null, suspended: true },
      }),
    ).toEqual([])
  })

  it('uses the pending-changes API when it reports pending', () => {
    const [s] = computeSuggestions({
      ...healthy,
      pending: {
        pending: true,
        apply_action: 'restart',
        changes: [
          { kind: 'env', keys: ['A', 'B', 'C'], since: '2026-01-01T00:00:00Z' },
        ],
      },
    })
    expect(s?.title).toBe('3 changes not applied')
    expect(s?.action.kind).toBe('apply_pending')
  })

  it('falls back to env_dirty with a restart action', () => {
    const [s] = computeSuggestions({
      ...healthy,
      app: { ...healthy.app, env_dirty: true },
    })
    expect(s?.action.kind).toBe('restart')
  })

  it('flags high error rate only above the threshold and with traffic', () => {
    expect(ids({ ...healthy, traffic: traffic(0.2) })).toEqual(['high_errors'])
    expect(ids({ ...healthy, traffic: traffic(0.001) })).toEqual([])
    expect(ids({ ...healthy, traffic: traffic(0.9, 0) })).toEqual([])
  })

  it('flags memory pressure only without a limit', () => {
    const noLimit = { ...healthy.app, resources: null }
    const memory = { usage: 90, limit: 100 }
    expect(ids({ ...healthy, app: noLimit, memory })).toEqual([
      'memory_no_limit',
    ])
    expect(ids({ ...healthy, memory })).toEqual([])
  })

  it('failed deploy offers the fix when a cause is known', () => {
    const [withCause] = computeSuggestions({
      ...healthy,
      latestAttemptStatus: 'failed',
      topCauseTitle: 'Wrong port',
    })
    expect(withCause?.title).toBe('Last deploy failed: Wrong port')
    expect(withCause?.action.kind).toBe('show_fix')
    const [noCause] = computeSuggestions({
      ...healthy,
      latestAttemptStatus: 'failed',
    })
    expect(noCause?.action.kind).toBe('open_deploy')
  })

  it('flags a domain that is not serving', () => {
    expect(ids({ ...healthy, domainStatus: 'not_resolving' })).toEqual([
      'domain_not_serving',
    ])
    expect(ids({ ...healthy, domainStatus: 'connected' })).toEqual([])
  })

  it('suggests git only once the source lookup has settled', () => {
    expect(ids({ ...healthy, hasGitSource: false })).toEqual(['no_git_source'])
    expect(
      ids({ ...healthy, hasGitSource: false, gitSourceKnown: false }),
    ).toEqual([])
  })

  it('caps at three, highest priority first, and honours dismissals', () => {
    const noisy: SuggestionInput = {
      ...healthy,
      app: { ...healthy.app, health: null, env_dirty: true },
      latestAttemptStatus: 'failed',
      traffic: traffic(0.5),
      hasGitSource: false,
    }
    expect(ids(noisy)).toEqual([
      'failed_deploy',
      'high_errors',
      'pending_changes',
    ])
    expect(ids({ ...noisy, dismissed: new Set(['failed_deploy']) })).toEqual([
      'high_errors',
      'pending_changes',
      'no_health_check',
    ])
  })
})

describe('computeSetup', () => {
  it('counts only actionable steps', () => {
    const { items, done } = computeSetup({
      app: { health: null, resources: null, domains: ['a'] },
      hasGitSource: false,
    })
    expect(items).toHaveLength(4)
    expect(done).toBe(1)
  })
})
