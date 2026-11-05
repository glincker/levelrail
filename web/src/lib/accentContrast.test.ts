import { describe, expect, it } from 'vitest'
import {
  contrastRatio,
  deriveAccent,
  hexToRgb,
  normalizeHex,
  nudgeLightness,
  oklchToRgb,
  rgbToOklch,
} from './accentContrast'

describe('accentContrast', () => {
  it('computes the WCAG extremes', () => {
    expect(contrastRatio([0, 0, 0], [1, 1, 1])).toBeCloseTo(21, 1)
    expect(contrastRatio([1, 1, 1], [1, 1, 1])).toBeCloseTo(1, 5)
  })

  it('validates and normalizes hex input', () => {
    expect(normalizeHex('#ABCDEF')).toBe('#abcdef')
    expect(normalizeHex('0e7490')).toBe('#0e7490')
    expect(normalizeHex('#abc')).toBeNull()
    expect(normalizeHex('#12zz45')).toBeNull()
  })

  it('round trips a color through OKLCH', () => {
    const rgb = hexToRgb('#3b82f6')
    const back = oklchToRgb(rgbToOklch(rgb))
    rgb.forEach((c, i) => expect(back[i]).toBeCloseTo(c, 2))
  })

  it('leaves a passing color alone and darkens a failing one', () => {
    const white = [1, 1, 1] as const
    const dark = rgbToOklch(hexToRgb('#1e3a8a'))
    expect(nudgeLightness(dark, [white], 'darker').adjusted).toBe(false)
    const pale = rgbToOklch(hexToRgb('#ffff66'))
    const out = nudgeLightness(pale, [white], 'darker')
    expect(out.adjusted).toBe(true)
    expect(out.color[0]).toBeLessThan(pale[0])
    expect(contrastRatio(oklchToRgb(out.color), white)).toBeGreaterThanOrEqual(
      4.5,
    )
  })

  it('lightens a dark color against a dark background', () => {
    const bg = oklchToRgb([0.145, 0, 0])
    const out = nudgeLightness(rgbToOklch(hexToRgb('#1e3a8a')), [bg], 'lighter')
    expect(out.adjusted).toBe(true)
    expect(contrastRatio(oklchToRgb(out.color), bg)).toBeGreaterThanOrEqual(4.5)
  })

  it('derives readable variants for both modes', () => {
    const d = deriveAccent('#ffff66', [1, 0, 0], [0.145, 0, 0])
    expect(d.adjusted).toBe(true)
    expect(d.light).toMatch(/^oklch\(/)
    expect(d.dark).toMatch(/^oklch\(/)
  })
})
