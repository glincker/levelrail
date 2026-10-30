import { describe, expect, it } from 'vitest'
import {
  belowFloor,
  groupRunsByDay,
  groupTargets,
  isShortening,
  percentOf,
  retentionOptions,
  runsExportUrl,
} from './cleanup'
import type {
  CleanupRun,
  CleanupRunFilters,
  CleanupTarget,
} from '../types/cleanup'

const NONE: CleanupRunFilters = {
  target: '',
  since: '',
  until: '',
  kind: 'all',
  status: 'all',
}

function target(
  id: string,
  category: CleanupTarget['category'],
  bytes: number,
) {
  return {
    id,
    category,
    usage: { rows: 1, bytes },
  } as CleanupTarget
}

describe('belowFloor', () => {
  it('refuses positive windows under the floor but allows keep forever', () => {
    expect(belowFloor(30, 90)).toBe(true)
    expect(belowFloor(90, 90)).toBe(false)
    expect(belowFloor(0, 90)).toBe(false)
    expect(belowFloor(7, 0)).toBe(false)
  })
})

describe('isShortening', () => {
  it('treats 0 as the longest window', () => {
    expect(isShortening(0, 90)).toBe(true)
    expect(isShortening(90, 0)).toBe(false)
    expect(isShortening(180, 90)).toBe(true)
    expect(isShortening(90, 180)).toBe(false)
    expect(isShortening(90, 90)).toBe(false)
  })
})

describe('retentionOptions', () => {
  it('keeps an env override value in the list, sorted', () => {
    expect(retentionOptions([0, 30, 90], 45)).toEqual([0, 30, 45, 90])
  })
})

describe('percentOf', () => {
  it('is safe for an empty whole', () => {
    expect(percentOf(5, 0)).toBe(0)
    expect(percentOf(25, 100)).toBe(25)
  })
})

describe('groupTargets', () => {
  it('orders by category and sums bytes', () => {
    const groups = groupTargets([
      target('a', 'audit', 10),
      target('b', 'auth', 5),
      target('c', 'audit', 20),
    ])
    expect(groups.map((g) => g.category)).toEqual(['auth', 'audit'])
    expect(groups[1]?.bytes).toBe(30)
  })
})

describe('runsExportUrl', () => {
  it('builds csv and json links that carry the active filters', () => {
    const csv = runsExportUrl(
      { ...NONE, target: 'audit.log', kind: 'dry' },
      'csv',
    )
    expect(csv).toContain('/api/v1/cleanup/runs?')
    expect(csv).toContain('format=csv')
    expect(csv).toContain('target=audit.log')
    expect(csv).toContain('dry_run=true')
    expect(runsExportUrl(NONE, 'json')).toBe('/api/v1/cleanup/runs?format=json')
  })

  it('extends the until date to the end of that day', () => {
    const url = runsExportUrl({ ...NONE, until: '2026-10-01' }, 'csv')
    const until = new URL(url, 'http://x').searchParams.get('until') ?? ''
    expect(new Date(until).getTime()).toBeGreaterThan(
      new Date('2026-10-01T23:59:00').getTime(),
    )
  })
})

describe('groupRunsByDay', () => {
  it('groups adjacent runs by local day', () => {
    const run = (id: number, at: string) =>
      ({ id, started_at: at }) as CleanupRun
    const days = groupRunsByDay([
      run(1, '2026-10-02T12:00:00'),
      run(2, '2026-10-02T08:00:00'),
      run(3, '2026-10-01T09:00:00'),
    ])
    expect(days.map((d) => d.runs.length)).toEqual([2, 1])
  })
})
