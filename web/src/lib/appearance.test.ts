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
    expect(parsed.textSize).toBe('100')
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
