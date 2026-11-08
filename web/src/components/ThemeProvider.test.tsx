import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { ThemeProvider, useTheme } from './ThemeProvider'

describe('ThemeProvider oled', () => {
  afterEach(() => {
    window.localStorage.clear()
    document.documentElement.classList.remove('dark', 'oled')
  })

  it('sets dark and oled classes and persists the choice', () => {
    const { result } = renderHook(() => useTheme(), { wrapper: ThemeProvider })
    act(() => result.current.setTheme('oled'))
    const cls = document.documentElement.classList
    expect(cls.contains('dark')).toBe(true)
    expect(cls.contains('oled')).toBe(true)
    expect(window.localStorage.getItem('levelrail-theme')).toBe('oled')
    act(() => result.current.setTheme('light'))
    expect(cls.contains('oled')).toBe(false)
    expect(cls.contains('dark')).toBe(false)
  })

  it('restores a stored oled theme on mount', () => {
    window.localStorage.setItem('levelrail-theme', 'oled')
    const { result } = renderHook(() => useTheme(), { wrapper: ThemeProvider })
    expect(result.current.theme).toBe('oled')
  })
})
