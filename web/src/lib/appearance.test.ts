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

  it('parses legacy objects without the personalization fields to defaults', () => {
    const parsed = parseAppearance(JSON.stringify({ darkBase: 'black' }))
    expect(parsed.radius).toBe('normal')
    expect(parsed.contrast).toBe('system')
    expect(parsed.codeFont).toBe('system-mono')
    expect(parsed.codeSize).toBe(13)
    expect(parsed.contentWidth).toBe(1440)
  })

  it('clamps and snaps the new fields', () => {
    const parsed = parseAppearance(
      JSON.stringify({
        radius: 'round',
        contrast: 'maybe',
        codeFont: 'comic',
        codeSize: 99,
        contentWidth: 5000,
      }),
    )
    expect(parsed).toMatchObject({
      radius: 'normal',
      contrast: 'system',
      codeFont: 'system-mono',
      codeSize: 16,
      contentWidth: 1920,
    })
    expect(parseAppearance(JSON.stringify({ codeSize: 2 })).codeSize).toBe(11)
    expect(
      parseAppearance(JSON.stringify({ contentWidth: '745' })).contentWidth,
    ).toBe(760)
    expect(
      parseAppearance(JSON.stringify({ contentWidth: 10 })).contentWidth,
    ).toBe(720)
  })

  it('applies the new data attributes and CSS variables, and clears them at default', () => {
    setAppearance({
      radius: 'soft',
      contrast: 'on',
      codeFont: 'fira',
      codeSize: 15,
      contentWidth: 1200,
    })
    expect(root.getAttribute('data-radius')).toBe('soft')
    expect(root.getAttribute('data-contrast')).toBe('on')
    expect(root.getAttribute('data-code-font')).toBe('fira')
    expect(root.getAttribute('data-code-size')).toBe('15')
    expect(root.style.getPropertyValue('--code-size')).toBe('15px')
    expect(root.style.getPropertyValue('--glinr-custom-width')).toBe('1200px')
    resetAppearance()
    expect(root.hasAttribute('data-radius')).toBe(false)
    expect(root.hasAttribute('data-contrast')).toBe(false)
    expect(root.hasAttribute('data-code-font')).toBe(false)
    expect(root.style.getPropertyValue('--code-size')).toBe('')
    expect(root.style.getPropertyValue('--glinr-custom-width')).toBe('')
  })

  it('presets keep the new defaults and reset restores them', () => {
    setAppearance({ radius: 'sharp', codeSize: 12 })
    applyPreset('default')
    expect(getAppearance()).toEqual(APPEARANCE_DEFAULTS)
  })
})
