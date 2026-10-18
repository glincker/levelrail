import { describe, expect, it } from 'vitest'
import { forecastPoints } from './forecastPoints'

const band = {
  spent: 30,
  mid: 90,
  low: 60,
  high: 120,
  confidence: 'medium',
  samples: 5,
} as const

describe('forecastPoints', () => {
  it('accumulates actual spend then fans out to the band at month end', () => {
    const pts = forecastPoints([10, 10, 10], 6, band)
    expect(pts.map((p) => p.actual)).toEqual([
      10,
      20,
      30,
      undefined,
      undefined,
      undefined,
    ])
    expect(pts[2]?.band).toEqual([30, 30])
    expect(pts[5]?.mid).toBe(90)
    expect(pts[5]?.band).toEqual([60, 120])
  })

  it('has no projection once the month is complete', () => {
    const pts = forecastPoints([1, 1], 2, band)
    expect(pts.every((p) => p.mid === undefined)).toBe(true)
  })
})
