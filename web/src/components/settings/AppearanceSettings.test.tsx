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
