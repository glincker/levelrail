import { describe, expect, it } from 'vitest'
import { buildSleepSegments, formatSleepDuration } from './sleepTimeline'

const HOUR = 3_600_000
const NOW = Date.parse('2026-10-15T12:00:00Z')
const iso = (hoursAgo: number) => new Date(NOW - hoursAgo * HOUR).toISOString()

describe('buildSleepSegments', () => {
  it('is one awake run with no events and no current sleep', () => {
    expect(buildSleepSegments([], NOW, 10 * HOUR, null)).toEqual([
      { asleep: false, startPct: 0, widthPct: 100 },
    ])
  })

  it('is one asleep run when the app has slept since before the window', () => {
    expect(buildSleepSegments([], NOW, 10 * HOUR, NOW - 99 * HOUR)).toEqual([
      { asleep: true, startPct: 0, widthPct: 100 },
    ])
  })

  it('splits at slept and woke events and keeps an open sleep to now', () => {
    const segs = buildSleepSegments(
      [
        { kind: 'slept', at: iso(8) },
        { kind: 'woke', at: iso(6) },
        { kind: 'slept', at: iso(2) },
      ],
      NOW,
      10 * HOUR,
      NOW - 2 * HOUR,
    )
    expect(segs.map((s) => [s.asleep, Math.round(s.widthPct)])).toEqual([
      [false, 20],
      [true, 20],
      [false, 40],
      [true, 20],
    ])
  })

  it('treats a first woke inside the window as asleep before it', () => {
    const segs = buildSleepSegments(
      [{ kind: 'woke', at: iso(5) }],
      NOW,
      10 * HOUR,
      null,
    )
    expect(segs[0]).toMatchObject({ asleep: true, startPct: 0 })
    expect(Math.round(segs[0]?.widthPct ?? 0)).toBe(50)
    expect(segs[1]?.asleep).toBe(false)
  })

  it('ignores enabled and failed events', () => {
    const segs = buildSleepSegments(
      [{ kind: 'wake_failed', at: iso(3) }],
      NOW,
      10 * HOUR,
      null,
    )
    expect(segs).toHaveLength(1)
  })
})

describe('formatSleepDuration', () => {
  it('formats units', () => {
    expect(formatSleepDuration(30)).toBe('30s')
    expect(formatSleepDuration(2 * 3600 + 14 * 60)).toBe('2h 14m')
    expect(formatSleepDuration(3 * 86400 + 4 * 3600)).toBe('3d 4h')
  })
})
