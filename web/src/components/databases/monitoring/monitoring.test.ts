import { describe, expect, it } from 'vitest'
import {
  buildRows,
  CHART_DEFS,
  chartsForEngine,
  engineHasQueryTabs,
  isCollected,
  metricsForCharts,
  type SeriesMap,
} from './chartRegistry'
import { buildExportTable, toCsv, toJson } from './exportMetrics'
import { parseCustomRange, stepForSpan } from './monitoringRange'
import {
  isInactive,
  resolveMonitoringState,
  restartedRecently,
} from './monitoringState'
import { deltaVsPrevious, sparklinePath } from './monitoringStats'

const pt = (t: string, value: number) => ({ timestamp: t, value, count: 1 })

describe('chart registry', () => {
  it('hides disk for in-memory engines and engine-only cards elsewhere', () => {
    const ids = (e: string, disk: boolean) =>
      chartsForEngine(e, disk).map((d) => d.id)
    expect(ids('redis', false)).toEqual(['cpu', 'memory', 'network'])
    expect(ids('postgres', true)).toContain('cacheHit')
    expect(ids('mysql', true)).not.toContain('cacheHit')
    expect(ids('mongodb', true)).not.toContain('connections')
  })
  it('marks uncollected cards and only requests real metrics', () => {
    const pending = CHART_DEFS.filter((d) => !isCollected(d))
    expect(pending.length).toBeGreaterThan(0)
    expect(metricsForCharts(CHART_DEFS)).toContain('cpu_percent')
  })
  it('limits query tabs to SQL engines', () => {
    expect(engineHasQueryTabs('postgres')).toBe(true)
    expect(engineHasQueryTabs('redis')).toBe(false)
  })
})

describe('range helpers', () => {
  const now = new Date('2026-01-10T12:00:00Z')
  it('picks a step per span', () => {
    expect(stepForSpan(30 * 60_000)).toBeUndefined()
    expect(stepForSpan(3 * 3_600_000)).toBe('60s')
    expect(stepForSpan(3 * 86_400_000)).toBe('30m')
  })
  it('validates a custom range', () => {
    const max = 86_400_000
    expect(parseCustomRange('x', 'y', now, max)).toEqual({
      ok: false,
      reason: 'invalid',
    })
    expect(
      parseCustomRange(
        '2026-01-10T11:00:00Z',
        '2026-01-10T10:00:00Z',
        now,
        max,
      ),
    ).toEqual({ ok: false, reason: 'order' })
    expect(
      parseCustomRange(
        '2026-01-10T11:00:00Z',
        '2026-01-11T11:00:00Z',
        now,
        max,
      ),
    ).toEqual({ ok: false, reason: 'future' })
    expect(
      parseCustomRange(
        '2026-01-01T00:00:00Z',
        '2026-01-10T00:00:00Z',
        now,
        max,
      ),
    ).toEqual({ ok: false, reason: 'tooLong' })
    const ok = parseCustomRange(
      '2026-01-10T09:00:00Z',
      '2026-01-10T11:00:00Z',
      now,
      max,
    )
    expect(ok.ok && ok.range.step).toBe('60s')
  })
})

describe('csv and json export', () => {
  const data: SeriesMap = {
    cpu_percent: {
      metric: 'cpu_percent',
      points: [pt('2026-01-10T10:00:00Z', 5), pt('2026-01-10T10:01:00Z', 7)],
    },
  }
  const cpu = CHART_DEFS.filter((d) => d.id === 'cpu')
  const table = buildExportTable(cpu, data, () => 'CPU, "main"')
  it('escapes cells and emits one row per timestamp', () => {
    const lines = toCsv(table).trim().split('\n')
    expect(lines[0]).toBe('timestamp,"CPU, ""main"""')
    expect(lines).toHaveLength(3)
    expect(lines[1]).toBe('2026-01-10T10:00:00.000Z,5')
  })
  it('emits json keyed by label', () => {
    const parsed = JSON.parse(toJson(table)) as Array<Record<string, unknown>>
    expect(parsed[1]?.['CPU, "main"']).toBe(7)
  })
  it('builds sorted rows', () => {
    expect(buildRows(cpu[0]!, data).map((r) => r.s0)).toEqual([5, 7])
  })
})

describe('state and stats', () => {
  const base = { restarting: false, hasData: true, isLoading: false }
  it('resolves the inactive reasons in priority order', () => {
    expect(
      resolveMonitoringState({
        ...base,
        provisionState: 'failed',
        suspended: true,
      }),
    ).toBe('failed')
    expect(resolveMonitoringState({ ...base, provisionState: 'pending' })).toBe(
      'provisioning',
    )
    expect(resolveMonitoringState({ ...base, suspended: true })).toBe('stopped')
    expect(resolveMonitoringState({ ...base, hasData: false })).toBe('noData')
    expect(resolveMonitoringState({ ...base, restarting: true })).toBe(
      'restarting',
    )
    expect(isInactive('stopped')).toBe(true)
    expect(isInactive('restarting')).toBe(false)
  })
  it('detects a recent restart from the counter', () => {
    const now = Date.parse('2026-01-10T10:10:00Z')
    const pts = [pt('2026-01-10T10:05:00Z', 1), pt('2026-01-10T10:08:00Z', 2)]
    expect(restartedRecently(pts, now, 300_000)).toBe(true)
    expect(restartedRecently(pts, now, 60_000)).toBe(false)
  })
  it('computes delta and sparkline honestly', () => {
    expect(deltaVsPrevious([pt('a', 15)], [pt('b', 10)])).toBe(50)
    expect(deltaVsPrevious([pt('a', 1)], undefined)).toBeNull()
    expect(sparklinePath([pt('a', 1)])).toBe('')
  })
})
