import { describe, expect, it } from 'vitest'
import { formatMetricValue } from './metricChart'

describe('request chart units', () => {
  it('formats request rate per second', () => {
    expect(formatMetricValue('rate', 2.5)).toBe('2.50/s')
    expect(formatMetricValue('rate', 120)).toBe('120/s')
  })

  it('formats latency in milliseconds', () => {
    expect(formatMetricValue('ms', 4.25)).toBe('4.3 ms')
    expect(formatMetricValue('ms', 180)).toBe('180 ms')
  })
})
