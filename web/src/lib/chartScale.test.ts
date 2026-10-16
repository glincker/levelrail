import { describe, expect, it } from 'vitest'
import {
  computeNiceYDomain,
  formatPercentWithAdaptivePrecision,
  generateUniqueTicks,
} from './chartScale'

describe('formatPercentWithAdaptivePrecision', () => {
  it('formats 0 as 0% regardless of axis max', () => {
    expect(formatPercentWithAdaptivePrecision(0, 100)).toBe('0%')
    expect(formatPercentWithAdaptivePrecision(0, 0.001)).toBe('0%')
  })

  it('uses 0 decimals when axis max >= 10', () => {
    expect(formatPercentWithAdaptivePrecision(5.6, 100)).toBe('6%')
    expect(formatPercentWithAdaptivePrecision(5.4, 100)).toBe('5%')
  })

  it('uses 1 decimal when axis max is 1 to 10', () => {
    expect(formatPercentWithAdaptivePrecision(5.67, 5)).toBe('5.7%')
    expect(formatPercentWithAdaptivePrecision(0.95, 2)).toBe('0.9%')
  })

  it('uses 2 decimals when axis max is 0.1 to 1', () => {
    expect(formatPercentWithAdaptivePrecision(0.567, 0.5)).toBe('0.57%')
    expect(formatPercentWithAdaptivePrecision(0.095, 0.1)).toBe('0.10%')
  })

  it('uses 3 decimals when axis max < 0.1', () => {
    expect(formatPercentWithAdaptivePrecision(0.0567, 0.05)).toBe('0.057%')
  })

  it('shows <threshold for values below the smallest representable digit', () => {
    expect(formatPercentWithAdaptivePrecision(0.0001, 0.01)).toBe('<0.001%')
    expect(formatPercentWithAdaptivePrecision(0.00001, 0.001)).toBe('<0.001%')
  })

  it('handles non-finite values', () => {
    expect(formatPercentWithAdaptivePrecision(Number.NaN, 100)).toBe('-')
    expect(
      formatPercentWithAdaptivePrecision(Number.POSITIVE_INFINITY, 100),
    ).toBe('-')
  })
})

describe('computeNiceYDomain', () => {
  it('returns [0, 0.1] for empty data in percent unit', () => {
    expect(computeNiceYDomain([], 'percent')).toEqual([0, 0.1])
  })

  it('returns [0, 100] for empty data in bytes/count unit', () => {
    expect(computeNiceYDomain([], 'bytes')).toEqual([0, 100])
    expect(computeNiceYDomain([], 'count')).toEqual([0, 100])
  })

  it('handles flat data: expands around the single value', () => {
    const domain = computeNiceYDomain([5], 'percent')
    expect(domain[0]).toBeLessThanOrEqual(5)
    expect(domain[1]).toBeGreaterThan(5)
  })

  it('handles zero flat data: uses a small range', () => {
    const domainPercent = computeNiceYDomain([0], 'percent')
    expect(domainPercent).toEqual([0, 0.1])
  })

  it('adds 10% headroom above max and below max for varied data', () => {
    const domain = computeNiceYDomain([10, 50], 'percent')
    const expectedHeadroom = (50 - 10) * 0.1
    expect(domain[1]).toBe(50 + expectedHeadroom)
    expect(domain[0]).toBeLessThanOrEqual(10)
  })

  it('never drops below 0 for the minimum', () => {
    const domain = computeNiceYDomain([0.1, 0.2], 'percent')
    expect(domain[0]).toBeGreaterThanOrEqual(0)
  })

  it('ignores non-finite values', () => {
    const domain = computeNiceYDomain([1, Number.NaN, 2], 'percent')
    expect(domain).toBeDefined()
    expect(domain[0]).toBeGreaterThanOrEqual(0)
    expect(domain[1]).toBeGreaterThan(domain[0])
  })
})

describe('generateUniqueTicks', () => {
  it('generates ticks with unique percent labels, avoiding dupes like 0.0%', () => {
    const domain: [number, number] = [0, 0.1]
    const ticks = generateUniqueTicks(domain, 0.1, 'percent', 5)
    const labels = ticks.map((t) => formatPercentWithAdaptivePrecision(t, 0.1))
    const uniqueLabels = new Set(labels)
    expect(uniqueLabels.size).toBe(labels.length)
  })

  it('generates at least 2 ticks even for narrow ranges', () => {
    const domain: [number, number] = [0, 0.0001]
    const ticks = generateUniqueTicks(domain, 0.0001, 'percent', 5)
    expect(ticks.length).toBeGreaterThanOrEqual(2)
  })

  it('respects the requested count approximately', () => {
    const domain: [number, number] = [0, 100]
    const ticks = generateUniqueTicks(domain, 100, 'percent', 5)
    expect(ticks.length).toBeGreaterThanOrEqual(3)
    expect(ticks.length).toBeLessThanOrEqual(5)
  })

  it('generates ticks spanning the full domain', () => {
    const domain: [number, number] = [0, 50]
    const ticks = generateUniqueTicks(domain, 50, 'percent', 5)
    expect(Math.min(...ticks)).toBeLessThanOrEqual(domain[0] + 0.1)
    expect(Math.max(...ticks)).toBeGreaterThanOrEqual(domain[1] - 0.1)
  })

  it('handles bytes unit ticks', () => {
    const domain: [number, number] = [0, 1024 * 1024 * 10]
    const ticks = generateUniqueTicks(domain, 1024 * 1024 * 10, 'bytes', 5)
    expect(ticks.length).toBeGreaterThanOrEqual(2)
    const min = Math.min(...ticks)
    expect(min).toBeGreaterThanOrEqual(0)
  })
})
