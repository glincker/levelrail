import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../../test/allI18n'
import { ThemeProvider } from '@/components/ThemeProvider'
import { AppearanceHub } from './AppearanceHub'

function setup(onReset = vi.fn()) {
  render(
    <AllI18n>
      <ThemeProvider>
        <AppearanceHub onReset={onReset} />
      </ThemeProvider>
    </AllI18n>,
  )
  return onReset
}

beforeEach(() => {
  localStorage.clear()
})

describe('AppearanceHub', () => {
  it('selects a theme by click and with arrow keys', () => {
    setup()
    const dark = screen.getByRole('radio', { name: /^Dark/ })
    expect(dark.getAttribute('aria-checked')).toBe('true')
    fireEvent.click(screen.getByRole('radio', { name: /^Light/ }))
    expect(
      screen
        .getByRole('radio', { name: /^Light/ })
        .getAttribute('aria-checked'),
    ).toBe('true')
    fireEvent.keyDown(screen.getByRole('radio', { name: /^Light/ }), {
      key: 'ArrowRight',
    })
    expect(
      screen.getByRole('radio', { name: /^Dark/ }).getAttribute('aria-checked'),
    ).toBe('true')
    expect(screen.getByRole('status').textContent).toBe('Saved')
  })

  it('resets and announces it', () => {
    const onReset = setup()
    fireEvent.click(screen.getByRole('button', { name: 'Reset to defaults' }))
    expect(onReset).toHaveBeenCalledTimes(1)
    expect(
      screen.getByText('Reset to defaults', { selector: 'p' }),
    ).toBeTruthy()
  })
})
