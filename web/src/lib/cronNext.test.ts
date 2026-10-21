import { describe, expect, it } from 'vitest'
import { isValidTimeZone, nextRuns, parseCron } from './cronNext'

const iso = (ms: number) => new Date(ms).toISOString()
const FROM = Date.UTC(2026, 2, 4, 10, 20)

describe('parseCron', () => {
  it.each([
    ['* * * * *', true],
    ['*/15 0-6 1,15 * 1-5', true],
    ['@hourly', true],
    ['@Daily', true],
    ['@sometimes', false],
    ['* * * *', false],
    ['60 * * * *', false],
    ['*/0 * * * *', false],
    ['5-1 * * * *', false],
  ])('%s -> valid %s', (expr, ok) => {
    expect(parseCron(expr) !== null).toBe(ok)
  })
})

describe('nextRuns', () => {
  it('lists upcoming hourly runs in UTC', () => {
    const spec = parseCron('@hourly')!
    expect(nextRuns(spec, 'UTC', FROM, 2).map(iso)).toEqual([
      '2026-03-04T11:00:00.000Z',
      '2026-03-04T12:00:00.000Z',
    ])
  })

  it('reads the schedule on the time zone wall clock', () => {
    const spec = parseCron('0 9 * * *')!
    const from = Date.UTC(2026, 2, 7, 12, 0)
    expect(nextRuns(spec, 'America/New_York', from, 1).map(iso)).toEqual([
      '2026-03-07T14:00:00.000Z',
    ])
  })

  it('runs a wall time skipped by spring forward once, an hour later', () => {
    const spec = parseCron('30 2 * * *')!
    const from = Date.UTC(2026, 2, 7, 12, 0)
    expect(nextRuns(spec, 'America/New_York', from, 2).map(iso)).toEqual([
      '2026-03-08T07:30:00.000Z',
      '2026-03-09T06:30:00.000Z',
    ])
  })

  it('runs a repeated fall-back wall time once', () => {
    const spec = parseCron('30 1 * * *')!
    const from = Date.UTC(2026, 10, 1, 0, 0)
    const runs = nextRuns(spec, 'America/New_York', from, 2)
    expect(runs).toHaveLength(2)
    expect(runs[1]! - runs[0]!).toBeGreaterThan(23 * 3_600_000)
  })

  it('uses day-of-month OR day-of-week when both are restricted', () => {
    const spec = parseCron('0 0 1 * 5')!
    const got = nextRuns(spec, 'UTC', Date.UTC(2026, 2, 1, 1, 0), 2).map(iso)
    expect(got).toEqual([
      '2026-03-06T00:00:00.000Z',
      '2026-03-13T00:00:00.000Z',
    ])
  })

  it('returns nothing for an impossible date', () => {
    const spec = parseCron('0 0 31 2 *')!
    expect(nextRuns(spec, 'UTC', FROM, 3)).toEqual([])
  })
})

describe('isValidTimeZone', () => {
  it('accepts IANA names and rejects junk', () => {
    expect(isValidTimeZone('Europe/Berlin')).toBe(true)
    expect(isValidTimeZone('Mars/Base')).toBe(false)
  })
})
