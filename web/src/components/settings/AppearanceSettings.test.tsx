import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import { ThemeProvider } from '@/components/ThemeProvider'
import { getAppearance, resetAppearance } from '@/lib/appearance'
import { getDensity } from '@/lib/density'
import { AppearanceSettings } from './AppearanceSettings'

function setup() {
  return render(
    <AllI18n>
      <ThemeProvider>
        <AppearanceSettings />
      </ThemeProvider>
    </AllI18n>,
  )
}

beforeEach(() => {
  localStorage.clear()
  resetAppearance()
})

describe('AppearanceSettings presets', () => {
  it('marks the default preset active on a fresh profile', () => {
    setup()
    expect(
      screen
        .getByRole('button', { name: /^Default/ })
        .getAttribute('aria-pressed'),
    ).toBe('true')
    expect(screen.queryByText('Custom')).toBeNull()
  })

  it('applies a preset on click and marks it pressed', () => {
    setup()
    fireEvent.click(screen.getByRole('button', { name: /^OLED/ }))
    expect(getAppearance().darkBase).toBe('black')
    expect(
      screen
        .getByRole('button', { name: /^OLED/ })
        .getAttribute('aria-pressed'),
    ).toBe('true')
    expect(
      screen
        .getByRole('button', { name: /^Default/ })
        .getAttribute('aria-pressed'),
    ).toBe('false')
  })

  it('shows Custom after changing one value, and Reset restores default', () => {
    setup()
    fireEvent.click(screen.getByRole('button', { name: /^OLED/ }))
    fireEvent.click(screen.getAllByRole('radio', { name: /^Rounded/ })[0]!)
    expect(screen.getByText('Custom')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Reset to defaults' }))
    expect(getDensity()).toBe('compact')
    expect(screen.queryByText('Custom')).toBeNull()
  })
})

describe('AppearanceSettings personalization', () => {
  it('moves text size with the slider and the quick picks', () => {
    setup()
    fireEvent.change(screen.getByRole('slider', { name: 'Text size' }), {
      target: { value: '125' },
    })
    expect(getAppearance().textSize).toBe(125)
    for (const n of [90, 100, 110]) {
      fireEvent.click(screen.getByRole('button', { name: `${n}%` }))
      expect(getAppearance().textSize).toBe(n)
    }
  })

  it('rejects an invalid custom hex and keeps the stored color', () => {
    setup()
    fireEvent.click(screen.getByRole('radio', { name: /^Custom/ }))
    const before = getAppearance().accentCustom
    const hex = screen.getByRole('textbox', { name: 'Hex color' })
    fireEvent.change(hex, { target: { value: '#12zz' } })
    expect(hex.getAttribute('aria-invalid')).toBe('true')
    expect(screen.getByRole('alert')).toBeTruthy()
    expect(getAppearance().accentCustom).toBe(before)
  })

  it('adjusts a low contrast custom accent and says so', () => {
    setup()
    fireEvent.click(screen.getByRole('radio', { name: /^Custom/ }))
    fireEvent.change(screen.getByRole('textbox', { name: 'Hex color' }), {
      target: { value: '#ffff66' },
    })
    expect(getAppearance().accentCustom).toBe('#ffff66')
    expect(screen.getByText('Adjusted for readability')).toBeTruthy()
    expect(
      document.documentElement.style.getPropertyValue('--glinr-accent-light'),
    ).toMatch(/^oklch\(0\.[0-5]/)
  })

  it('writes the reduce motion attribute', () => {
    setup()
    fireEvent.click(screen.getByRole('radio', { name: /^Reduce/ }))
    expect(document.documentElement.getAttribute('data-motion')).toBe('on')
  })
})
