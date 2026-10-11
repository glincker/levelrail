import { afterEach, describe, expect, it, vi } from 'vitest'
import { attachPrevious, capRows, MAX_CHART_POINTS } from './chartSeries'
import type { ChartRow } from './metricChart'
import { fetchMetricSeries } from '../queries/metrics'
import { fetchRequestSeries } from '../queries/requests'

const STEP_MS = 15_000
const SEVEN_DAYS_MS = 7 * 24 * 60 * 60 * 1000

function rawRows(count: number, spikeAt?: number): ChartRow[] {
  return Array.from({ length: count }, (_, i) => ({
    t: i * STEP_MS,
    primary: i === spikeAt ? 9999 : 10 + (i % 7),
  }))
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('capRows', () => {
  it('leaves small series untouched', () => {
    const rows = rawRows(100)
    expect(capRows(rows)).toBe(rows)
  })

  it('bounds 7 days at 15s resolution and keeps the spike', () => {
    const count = SEVEN_DAYS_MS / STEP_MS
    expect(count).toBe(40320)
    const spikeAt = 12345
    const out = capRows(rawRows(count, spikeAt))
    expect(out.length).toBeLessThanOrEqual(MAX_CHART_POINTS)
    expect(out.some((r) => r.primary === 9999)).toBe(true)
    const times = out.map((r) => r.t)
    expect(times).toEqual([...times].sort((a, b) => a - b))
  })

  it.each([2, 3, 50, 599, 600])('never exceeds max=%i', (max) => {
    expect(capRows(rawRows(5000), max).length).toBeLessThanOrEqual(max)
  })
})

describe('attachPrevious', () => {
  const rows: ChartRow[] = [
    { t: Date.parse('2026-10-10T00:00:00Z'), primary: 1 },
    { t: Date.parse('2026-10-10T00:01:00Z'), primary: 2 },
  ]

  it('joins the shifted previous series on equal timestamps', () => {
    const out = attachPrevious(rows, [
      { timestamp: '2026-10-10T00:01:00Z', value: 7 },
    ])
    expect(out[0]?.previous).toBeUndefined()
    expect(out[1]).toMatchObject({ primary: 2, previous: 7 })
  })

  it('keeps previous points that have no current row, in order', () => {
    const out = attachPrevious(rows, [
      { timestamp: '2026-10-10T00:00:30Z', value: 5 },
    ])
    expect(out.map((r) => r.t)).toEqual([
      Date.parse('2026-10-10T00:00:00Z'),
      Date.parse('2026-10-10T00:00:30Z'),
      Date.parse('2026-10-10T00:01:00Z'),
    ])
    expect(out[1]).toEqual({
      t: Date.parse('2026-10-10T00:00:30Z'),
      previous: 5,
    })
  })

  it('returns the input when there is no previous period', () => {
    expect(attachPrevious(rows, undefined)).toBe(rows)
    expect(attachPrevious(rows, [])).toBe(rows)
  })
})

describe('long range requests', () => {
  it('asks the server for at most max_points and sends no step', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ metric: 'cpu_percent', points: [] }), {
        status: 200,
      }),
    )
    vi.stubGlobal('fetch', fetchMock)
    const to = new Date('2026-10-10T00:00:00Z')
    const from = new Date(to.getTime() - SEVEN_DAYS_MS)
    await fetchMetricSeries('web', 'cpu_percent', {
      from,
      to,
      maxPoints: MAX_CHART_POINTS,
      compare: true,
    })
    const url = new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://x')
    expect(url.searchParams.get('max_points')).toBe('600')
    expect(url.searchParams.get('compare')).toBe('previous')
    expect(url.searchParams.has('step')).toBe(false)
  })

  it('does the same for the request series', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ points: [] }), { status: 200 }),
      )
    vi.stubGlobal('fetch', fetchMock)
    const to = new Date('2026-10-10T00:00:00Z')
    await fetchRequestSeries('web', {
      from: new Date(to.getTime() - SEVEN_DAYS_MS),
      to,
      maxPoints: MAX_CHART_POINTS,
    })
    const url = new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://x')
    expect(url.searchParams.get('max_points')).toBe('600')
    expect(url.searchParams.has('compare')).toBe(false)
  })
})
