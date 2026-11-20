import { describe, expect, it } from 'vitest'
import {
  builderState,
  durationBarPercent,
  durationSeries,
  formatDurationMs,
  pressureTone,
} from './builderFormat'

describe('builderFormat', () => {
  it('maps every builder status to a status state', () => {
    expect(builderState('ready')).toBe('ready')
    expect(builderState('busy')).toBe('building')
    expect(builderState('paused')).toBe('paused')
    expect(builderState('offline')).toBe('offline')
    expect(builderState('pressure')).toBe('degraded')
    expect(builderState('draining')).toBe('draining')
    expect(builderState('mystery')).toBe('pending')
  })

  it('maps pressure to a tone', () => {
    expect(pressureTone('ok')).toBe('success')
    expect(pressureTone('warn')).toBe('warning')
    expect(pressureTone('critical')).toBe('danger')
    expect(pressureTone('x')).toBe('neutral')
  })

  it('formats durations and series', () => {
    expect(formatDurationMs(0)).toBe('-')
    expect(formatDurationMs(42_000)).toBe('42s')
    expect(formatDurationMs(185_000)).toBe('3m 05s')
    expect(durationBarPercent(50, 100)).toBe(50)
    expect(durationBarPercent(1, 100)).toBe(4)
    const b = (started_at: string, duration_ms: number) => ({
      app: 'a',
      tag: 't',
      started_at,
      duration_ms,
      result: 'success' as const,
    })
    expect(durationSeries([b('2026-01-02', 2), b('2026-01-01', 1)])).toEqual([
      1, 2,
    ])
  })
})
