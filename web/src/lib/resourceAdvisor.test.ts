import { describe, expect, it } from 'vitest'
import {
  formatActionValue,
  groupByResource,
  isKnownKind,
  sortBySavings,
  splitPending,
} from './resourceAdvisor'
import type { AdvisorFinding } from '../types/resourceAdvisor'

function finding(
  id: string,
  over: Partial<AdvisorFinding> = {},
): AdvisorFinding {
  return {
    id,
    kind: 'memory_limit_missing',
    severity: 'suggest',
    resource: { kind: 'database', name: 'main' },
    title: id,
    why: '',
    evidence: { window: 'last 7 days', window_seconds: 604800, samples: 10 },
    impact: {},
    appliable: true,
    can_apply: true,
    ...over,
  }
}

describe('groupByResource', () => {
  it('puts the most severe resource first and sums savings', () => {
    const groups = groupByResource([
      finding('a', { resource: { kind: 'app', name: 'web' } }),
      finding('b', {
        severity: 'warn',
        resource: { kind: 'database', name: 'main' },
        impact: { saving_memory_bytes: 10 },
      }),
      finding('c', {
        resource: { kind: 'database', name: 'main' },
        impact: { saving_memory_bytes: 5 },
      }),
    ])
    expect(groups.map((g) => g.ref.name)).toEqual(['main', 'web'])
    expect(groups[0]).toMatchObject({ severity: 'warn', saving: 15 })
  })
})

describe('sortBySavings', () => {
  it('orders by saving then severity', () => {
    const sorted = sortBySavings([
      finding('none'),
      finding('disk', { impact: { saving_disk_bytes: 50 } }),
      finding('mem', { impact: { saving_memory_bytes: 100 } }),
      finding('warn', { severity: 'warn' }),
    ])
    expect(sorted.map((f) => f.id)).toEqual(['mem', 'disk', 'warn', 'none'])
  })
})

describe('helpers', () => {
  it('splits pending data notes from advice', () => {
    const { advice, pending } = splitPending([
      finding('a'),
      finding('b', { kind: 'not_enough_data' }),
    ])
    expect(advice).toHaveLength(1)
    expect(pending).toHaveLength(1)
  })

  it('formats action values in their unit', () => {
    expect(formatActionValue('set_memory_limit', 256 * 1024 * 1024)).toBe(
      '256 MiB',
    )
    expect(formatActionValue('set_cpu_limit', 500_000_000)).toBe('0.50 cores')
    expect(formatActionValue('set_memory_limit', 0)).toBeNull()
    expect(formatActionValue('manual', 1)).toBeNull()
  })

  it('knows every server finding kind', () => {
    expect(isKnownKind('engine_tuning')).toBe(true)
    expect(isKnownKind('made_up')).toBe(false)
  })
})
