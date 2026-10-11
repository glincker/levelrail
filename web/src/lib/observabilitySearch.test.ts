import { describe, expect, it } from 'vitest'
import {
  investigationWindow,
  parseLogsSearch,
  parseMetricsSearch,
  resolveMetricsRange,
} from './observabilitySearch'

const NOW = new Date('2026-10-10T12:00:00Z')

describe('parseMetricsSearch', () => {
  it.each([
    [{}, {}],
    [{ range: '7d' }, { range: '7d' }],
    [{ range: 'bogus' }, {}],
    [{ compare: true }, { compare: true }],
    [{ compare: 'true' }, { compare: true }],
    [{ at: 'not a date' }, {}],
    [
      { from: '2026-10-10T00:00:00Z', to: '2026-10-10T01:00:00Z' },
      {
        range: 'custom',
        from: '2026-10-10T00:00:00Z',
        to: '2026-10-10T01:00:00Z',
      },
    ],
    [{ from: '2026-10-10T02:00:00Z', to: '2026-10-10T01:00:00Z' }, {}],
  ])('parses %j', (raw, want) => {
    expect(parseMetricsSearch(raw)).toEqual(want)
  })

  it('round trips a custom range to the same window', () => {
    const parsed = parseMetricsSearch({
      from: '2026-10-10T00:00:00Z',
      to: '2026-10-10T01:00:00Z',
    })
    const range = resolveMetricsRange(parsed, NOW)
    expect(range.from.toISOString()).toBe('2026-10-10T00:00:00.000Z')
    expect(range.to.toISOString()).toBe('2026-10-10T01:00:00.000Z')
  })

  it('resolves a preset relative to now', () => {
    const range = resolveMetricsRange({ range: '6h' }, NOW)
    expect(range.to).toEqual(NOW)
    expect(NOW.getTime() - range.from.getTime()).toBe(6 * 60 * 60 * 1000)
  })
})

describe('parseLogsSearch', () => {
  it('keeps filters and repeated fields', () => {
    expect(
      parseLogsSearch({
        tab: 'search',
        q: 'timeout',
        level: 'error',
        container: 'abc123',
        stream: 'stderr',
        field: ['status=500', 'user.id~42'],
        from: '2026-10-10T00:00:00Z',
        to: '2026-10-10T01:00:00Z',
      }),
    ).toEqual({
      tab: 'search',
      q: 'timeout',
      level: 'error',
      container: 'abc123',
      stream: 'stderr',
      field: ['status=500', 'user.id~42'],
      from: '2026-10-10T00:00:00Z',
      to: '2026-10-10T01:00:00Z',
    })
  })

  it('wraps a single field, stringifies numeric q, drops half ranges', () => {
    expect(
      parseLogsSearch({ field: 'a=b', q: 500, from: '2026-10-10T00:00:00Z' }),
    ).toEqual({ field: ['a=b'], q: '500' })
  })
})

describe('investigationWindow', () => {
  const at = new Date('2026-10-10T12:00:00Z')
  const range = (hours: number) => ({
    from: new Date(NOW.getTime() - hours * 3_600_000),
    to: NOW,
  })

  it.each([
    [0.25, 5 * 60_000],
    [1, 5 * 60_000],
    [6, 30 * 60_000],
    [24, 60 * 60_000],
    [720, 60 * 60_000],
  ])('range of %f hours gives a %i ms window', (hours, widthMs) => {
    const w = investigationWindow(at, range(hours))
    expect(w.to.getTime() - w.from.getTime()).toBe(widthMs)
    expect(w.from.getTime() + widthMs / 2).toBe(at.getTime())
  })
})
