import { describe, expect, it } from 'vitest'
import { formatDurationValue, formatMetricValue } from './metricChart'

describe('formatDurationValue', () => {
  it('renders whole seconds under a minute with no decimal', () => {
    expect(formatDurationValue(0)).toBe('0s')
    expect(formatDurationValue(42)).toBe('42s')
    expect(formatDurationValue(59.4)).toBe('59s')
  })

  it('renders minutes and seconds under an hour', () => {
    expect(formatDurationValue(60)).toBe('1m 0s')
    expect(formatDurationValue(125)).toBe('2m 5s')
    expect(formatDurationValue(3599)).toBe('59m 59s')
  })

  it('renders hours and minutes at an hour or beyond', () => {
    expect(formatDurationValue(3600)).toBe('1h 0m')
    expect(formatDurationValue(3725)).toBe('1h 2m')
  })

  it('handles a non-finite value without throwing', () => {
    expect(formatDurationValue(Number.NaN)).toBe('-')
  })
})

describe('formatMetricValue', () => {
  it('formats a count unit as a whole number', () => {
    expect(formatMetricValue('count', 3)).toBe('3')
    expect(formatMetricValue('count', 0)).toBe('0')
  })

  it('formats a seconds unit via formatDurationValue', () => {
    expect(formatMetricValue('seconds', 90)).toBe('1m 30s')
  })

  it('still formats percent and bytes as before', () => {
    expect(formatMetricValue('percent', 12.34)).toBe('12.3%')
    expect(formatMetricValue('bytes', 2048)).toBe('2.0 KiB')
  })
})
