import { describe, expect, it } from 'vitest'
import { dedupeReason } from './upgradeAttempts'

describe('dedupeReason', () => {
  it('prints an identical detail and error once', () => {
    expect(dedupeReason('disk full', 'disk full')).toEqual(['disk full'])
  })
  it('keeps the longer text when one contains the other', () => {
    expect(dedupeReason('disk full', 'upgrade refused: disk full')).toEqual([
      'upgrade refused: disk full',
    ])
  })
  it('keeps both when they differ, and handles blanks', () => {
    expect(dedupeReason('timeout', 'not healthy')).toEqual([
      'timeout',
      'not healthy',
    ])
    expect(dedupeReason('', 'x')).toEqual(['x'])
    expect(dedupeReason('', '')).toEqual([])
  })
})
