import { beforeEach, describe, expect, it } from 'vitest'
import {
  APPEARANCE_DEFAULTS,
  STORAGE_KEY,
  applyAppearance,
  applyPreset,
  getAppearance,
  parseAppearance,
  resetAppearance,
  setAppearance,
} from './appearance'
import { getPageWidth } from './pageWidth'
import { getDensity } from './density'

const root = document.documentElement

beforeEach(() => {
  localStorage.clear()
  resetAppearance()
})

describe('appearance', () => {
  it('defaults set no attributes', () => {
    expect(getAppearance()).toEqual(APPEARANCE_DEFAULTS)
    expect(root.hasAttribute('data-dark-base')).toBe(false)
    expect(root.hasAttribute('data-font')).toBe(false)
  })

  it('persists a choice and stamps the attribute', () => {
    setAppearance({ darkBase: 'black', font: 'serif' })
    expect(root.getAttribute('data-dark-base')).toBe('black')
    expect(root.getAttribute('data-font')).toBe('serif')
    expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')).toMatchObject(
      { darkBase: 'black', font: 'serif' },
    )
  })

  it('falls back per field on invalid stored values', () => {
    const parsed = parseAppearance(
      JSON.stringify({ darkBase: 'neon', font: 'mono', textSize: 7 }),
    )
    expect(parsed.darkBase).toBe('soft')
    expect(parsed.font).toBe('mono')
    expect(parsed.textSize).toBe(80)
  })

  it('accepts legacy string sizes and clamps the rest into range', () => {
    const size = (v: unknown) =>
      parseAppearance(JSON.stringify({ textSize: v })).textSize
    expect(size('90')).toBe(90)
    expect(size('110')).toBe(110)
    expect(size(133)).toBe(135)
    expect(size(500)).toBe(140)
    expect(size('abc')).toBe(100)
  })

  it('stamps motion and text scale, and clears them at defaults', () => {
    setAppearance({ motion: 'on', textSize: 125 })
    expect(root.getAttribute('data-motion')).toBe('on')
    expect(root.style.getPropertyValue('--glinr-text-scale')).toBe('125')
    setAppearance({ motion: 'system', textSize: 100 })
    expect(root.hasAttribute('data-motion')).toBe(false)
    expect(root.style.getPropertyValue('--glinr-text-scale')).toBe('')
  })

  it('falls back to defaults on malformed or missing storage', () => {
    expect(parseAppearance('{nope')).toEqual(APPEARANCE_DEFAULTS)
    expect(parseAppearance(null)).toEqual(APPEARANCE_DEFAULTS)
    expect(parseAppearance('42')).toEqual(APPEARANCE_DEFAULTS)
  })

  it('applies a preset across appearance, width and density', () => {
    const preset = applyPreset('oled')
    expect(preset.theme).toBe('dark')
    expect(getAppearance().darkBase).toBe('black')
    expect(getPageWidth()).toBe('wide')
    expect(getDensity()).toBe('compact')
  })

  it('reset restores every default including width and density', () => {
    applyPreset('paper')
    resetAppearance()
    expect(getAppearance()).toEqual(APPEARANCE_DEFAULTS)
    expect(getPageWidth()).toBe('comfortable')
    expect(getDensity()).toBe('compact')
    expect(root.hasAttribute('data-light-base')).toBe(false)
  })

  it('applyAppearance clears an attribute when the value returns to default', () => {
    applyAppearance({ ...APPEARANCE_DEFAULTS, gutter: 'roomy' })
    expect(root.getAttribute('data-gutter')).toBe('roomy')
    applyAppearance(APPEARANCE_DEFAULTS)
    expect(root.hasAttribute('data-gutter')).toBe(false)
  })
})
