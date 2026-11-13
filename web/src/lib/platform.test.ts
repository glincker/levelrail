import { describe, expect, it } from 'vitest'
import { isMacPlatform, modifierLabel, shortcutLabel } from './platform'

describe('platform', () => {
  it('detects Mac from the modern hint, then the legacy field', () => {
    expect(isMacPlatform({ userAgentData: { platform: 'macOS' } })).toBe(true)
    expect(isMacPlatform({ platform: 'MacIntel' })).toBe(true)
    expect(isMacPlatform({ userAgentData: { platform: 'Windows' } })).toBe(
      false,
    )
    expect(isMacPlatform({ platform: 'Linux x86_64' })).toBe(false)
    expect(isMacPlatform({})).toBe(false)
  })

  it('labels Option and Command on Mac, Alt and Ctrl elsewhere', () => {
    expect(modifierLabel('alt', true)).toBe('⌥')
    expect(modifierLabel('mod', true)).toBe('⌘')
    expect(modifierLabel('alt', false)).toBe('Alt')
    expect(modifierLabel('mod', false)).toBe('Ctrl')
    expect(shortcutLabel('alt', '1', true)).toBe('⌥1')
    expect(shortcutLabel('alt', '1', false)).toBe('Alt+1')
    expect(shortcutLabel('mod', 'K', false)).toBe('Ctrl+K')
  })
})
