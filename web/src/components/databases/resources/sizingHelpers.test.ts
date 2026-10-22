import { describe, expect, it } from 'vitest'
import {
  headroom,
  impactKind,
  latestValue,
  peakValue,
  shrinksBelowPeak,
} from './sizingHelpers'

const series = {
  metric: 'memory_usage_bytes',
  points: [
    { timestamp: 'a', value: 10, count: 1 },
    { timestamp: 'b', value: 90, count: 1 },
    { timestamp: 'c', value: 40, count: 1 },
  ],
}

describe('sizingHelpers', () => {
  it('reads latest and peak', () => {
    expect(latestValue(series)).toBe(40)
    expect(peakValue(series)).toBe(90)
    expect(latestValue(undefined)).toBeNull()
    expect(peakValue({ metric: 'x', points: [] })).toBeNull()
  })

  it('classifies headroom', () => {
    expect(headroom(90, 100)).toBe('tight')
    expect(headroom(50, 100)).toBe('ok')
    expect(headroom(10, 100)).toBe('ample')
    expect(headroom(null, 100)).toBe('unknown')
    expect(headroom(10, 0)).toBe('unknown')
  })

  it('derives impact', () => {
    const a = { memoryBytes: 100, nanoCpus: 0 }
    expect(impactKind(false, a, a)).toBe('none')
    expect(impactKind(false, a, { ...a, memoryBytes: 200 })).toBe('live')
    expect(impactKind(true, a, { ...a, nanoCpus: 5 })).toBe('stopped')
  })

  it('flags shrinking below observed peak', () => {
    expect(shrinksBelowPeak(100, 50)).toBe(true)
    expect(shrinksBelowPeak(100, 200)).toBe(false)
    expect(shrinksBelowPeak(100, 0)).toBe(false)
    expect(shrinksBelowPeak(null, 50)).toBe(false)
  })
})
