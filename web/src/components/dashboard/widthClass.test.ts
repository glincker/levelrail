import { describe, expect, it } from 'vitest'
import { allocateWidths, widthClass } from './widthClass'

describe('allocateWidths', () => {
  it('sums to 100 when the parts cover the total', () => {
    const w = allocateWidths([1, 1, 1], 3)
    expect(w.reduce((a, b) => a + b, 0)).toBe(100)
  })

  it('returns zeros without a total', () => {
    expect(allocateWidths([5, 5], 0)).toEqual([0, 0])
  })
})

describe('widthClass', () => {
  it('clamps to a known class', () => {
    expect(widthClass(-5)).toBe('w-0')
    expect(widthClass(100)).toBe('w-full')
    expect(widthClass(37.4)).toBe('w-[37%]')
  })
})
