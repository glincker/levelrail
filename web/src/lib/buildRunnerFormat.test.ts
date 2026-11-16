import { describe, expect, it } from 'vitest'
import {
  capacityPercent,
  formatWaitMs,
  parseLabels,
  runnerState,
} from './buildRunnerFormat'

describe('capacityPercent', () => {
  it('rounds slot use to a whole percent', () => {
    expect(capacityPercent(1, 3)).toBe(33)
    expect(capacityPercent(2, 4)).toBe(50)
  })
  it('never divides by zero and clamps to the range', () => {
    expect(capacityPercent(0, 0)).toBe(0)
    expect(capacityPercent(5, 0)).toBe(0)
    expect(capacityPercent(9, 3)).toBe(100)
    expect(capacityPercent(-1, 3)).toBe(0)
  })
})

describe('runnerState', () => {
  it('maps runner statuses onto the shared status shapes', () => {
    expect(runnerState('ready')).toBe('ready')
    expect(runnerState('busy')).toBe('building')
    expect(runnerState('pressure')).toBe('degraded')
    expect(runnerState('draining')).toBe('draining')
    expect(runnerState('offline')).toBe('offline')
    expect(runnerState('unknown')).toBe('pending')
  })
})

describe('formatWaitMs and parseLabels', () => {
  it('formats waits and splits labels', () => {
    expect(formatWaitMs(4200)).toBe('4s')
    expect(formatWaitMs(120000)).toBe('2m')
    expect(formatWaitMs(95000)).toBe('1m 35s')
    expect(parseLabels('gpu, arch=arm64  gpu')).toEqual(['gpu', 'arch=arm64'])
    expect(parseLabels('')).toEqual([])
  })
})
