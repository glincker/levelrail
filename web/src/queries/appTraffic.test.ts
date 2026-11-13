import { describe, expect, it } from 'vitest'
import { percentChange } from './appTraffic'

describe('percentChange', () => {
  it('returns undefined without a baseline', () => {
    expect(percentChange(5, 0)).toBeUndefined()
  })

  it('computes the change when samples are large enough', () => {
    expect(
      percentChange(200, 100, { current: 60, previous: 40, minSample: 30 }),
    ).toBe(100)
  })

  it('returns undefined when either window is below the minimum sample', () => {
    expect(
      percentChange(0.5, 0.001, { current: 40, previous: 1, minSample: 30 }),
    ).toBeUndefined()
    expect(
      percentChange(5, 1, { current: 3, previous: 50, minSample: 30 }),
    ).toBeUndefined()
  })
})
