import { describe, expect, it } from 'vitest'
import {
  firstAppPhase,
  liveUrl,
  pickTrackedApp,
  pollInterval,
} from './firstApp'
import type { AppListEntry } from '../types/appDetail'

function app(overrides: Partial<AppListEntry>): AppListEntry {
  return {
    name: 'sample-app',
    image: 'nginx:alpine',
    port: 80,
    bind_address: 'private',
    strategy: 'rolling',
    replicas: 1,
    status: { label: 'Reconciling', variant: 'muted' },
    ...overrides,
  } as AppListEntry
}

describe('pollInterval', () => {
  it.each([
    { name: 'polls while waiting', done: false, elapsed: 1_000, want: 5_000 },
    { name: 'stops when done', done: true, elapsed: 1_000, want: false },
    {
      name: 'stops after the time budget',
      done: false,
      elapsed: 60_000,
      want: false,
    },
  ])('$name', ({ done, elapsed, want }) => {
    expect(pollInterval(done, 0, elapsed, 5_000, 60_000)).toBe(want)
  })
})

describe('first app state machine', () => {
  it('prefers the sample app, else the first app', () => {
    const apps = [app({ name: 'api' }), app({ name: 'sample-app' })]
    expect(pickTrackedApp(apps, 'sample-app')?.name).toBe('sample-app')
    expect(pickTrackedApp([app({ name: 'api' })], 'sample-app')?.name).toBe(
      'api',
    )
    expect(pickTrackedApp([], 'sample-app')).toBeUndefined()
  })

  it.each([
    { name: 'no app yet', a: undefined, elapsed: 0, want: 'none' },
    { name: 'reconciling', a: app({}), elapsed: 1_000, want: 'deploying' },
    {
      name: 'reconciling too long',
      a: app({}),
      elapsed: 200_000,
      want: 'slow',
    },
    {
      name: 'healthy',
      a: app({ status: { label: 'Healthy', variant: 'success' } }),
      elapsed: 0,
      want: 'live',
    },
    {
      name: 'attention needed',
      a: app({ status: { label: 'Attention needed', variant: 'destructive' } }),
      elapsed: 0,
      want: 'failed',
    },
  ])('$name -> $want', ({ a, elapsed, want }) => {
    expect(firstAppPhase(a, 0, elapsed, 120_000)).toBe(want)
  })

  it('picks a domain, then the fallback URL, then a reachable host port', () => {
    expect(liveUrl(app({ domains: ['a.example.com'] }), undefined, 'x')).toBe(
      'https://a.example.com',
    )
    expect(
      liveUrl(
        app({}),
        {
          container_port: 80,
          running: true,
          fallback_url: 'https://s.sslip.io',
        },
        'x',
      ),
    ).toBe('https://s.sslip.io')
    expect(
      liveUrl(
        app({}),
        { container_port: 80, host_port: 32768, running: true },
        'localhost',
      ),
    ).toBe('http://localhost:32768')
    expect(
      liveUrl(
        app({}),
        { container_port: 80, host_port: 32768, running: true },
        '203.0.113.7',
      ),
    ).toBeUndefined()
  })
})
