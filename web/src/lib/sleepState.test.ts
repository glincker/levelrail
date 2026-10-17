import { describe, expect, it } from 'vitest'
import type { AppSleep } from '../queries/appSleep'
import {
  deriveSleepState,
  isValidIdleMinutes,
  sleepCountdown,
} from './sleepState'

const base: AppSleep = {
  enabled: false,
  idle_minutes: 0,
  sleeping: false,
  hold_requests: false,
  eligible: true,
  blockers: [],
  warnings: [],
  settings: {
    idle_minutes: 0,
    hold_requests: false,
    ignore_paths: [],
    port_wake: false,
  },
  port_wake: { supported: false, active: false, ports: [] },
  activity: { open_connections: 0, idle_for_seconds: 0 },
  cold_start_ms_p50: null,
  estimated_memory_freed_bytes: null,
}
const issue = { code: 'pinned_port', title: 't', detail: 'd' }

describe('deriveSleepState', () => {
  const cases: [string, AppSleep, boolean, string][] = [
    ['eligible', { ...base, eligible: true, blockers: [] }, false, 'ready'],
    [
      'blockers',
      { ...base, eligible: false, blockers: [issue] },
      false,
      'blocked',
    ],
    [
      'warnings only',
      { ...base, eligible: true, warnings: [issue] },
      false,
      'ready',
    ],
    ['enabled and awake', { ...base, enabled: true }, false, 'awake'],
    ['sleeping', { ...base, enabled: true, sleeping: true }, false, 'sleeping'],
    ['waking', { ...base, enabled: true, sleeping: true }, true, 'waking'],
  ]
  it.each(cases)('%s', (_name, sleep, wakePending, want) => {
    expect(deriveSleepState(sleep, wakePending)).toBe(want)
  })
})

describe('sleepCountdown', () => {
  const now = Date.parse('2026-01-01T00:00:00Z')
  const cases: [string, string | undefined, unknown][] = [
    ['none', undefined, null],
    ['invalid', 'nope', null],
    ['12 minutes', '2026-01-01T00:12:00Z', { unit: 'minutes', value: 12 }],
    ['rounds up', '2026-01-01T00:01:01Z', { unit: 'minutes', value: 2 }],
    ['seconds', '2026-01-01T00:00:30Z', { unit: 'seconds', value: 30 }],
    ['past', '2025-12-31T23:00:00Z', { unit: 'seconds', value: 0 }],
  ]
  it.each(cases)('%s', (_name, at, want) => {
    expect(sleepCountdown(at, now)).toEqual(want)
  })
})

describe('isValidIdleMinutes', () => {
  it.each([
    [4, false],
    [5, true],
    [10080, true],
    [10081, false],
    [7.5, false],
  ])('%s', (v, want) => {
    expect(isValidIdleMinutes(v)).toBe(want)
  })
})
