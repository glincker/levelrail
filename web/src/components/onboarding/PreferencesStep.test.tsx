import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Wrap } from './onboardingTestKit'

const savePrefs = vi.fn()
const setTheme = vi.fn()

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('../../queries/accountSelf', () => ({
  preferencesQueryOptions: () => ({
    queryKey: ['prefs-step-test'],
    queryFn: () =>
      Promise.resolve({
        timezone: '',
        language: '',
        theme: '',
        landing_page: '',
        gravatar: false,
      }),
  }),
  useSavePreferences: () => ({
    mutate: savePrefs,
    isPending: false,
    isSuccess: false,
  }),
}))
vi.mock('../ThemeProvider', () => ({
  useTheme: () => ({ theme: 'system', setTheme }),
}))

import { PreferencesStep } from './PreferencesStep'

beforeEach(() => {
  savePrefs.mockClear()
  setTheme.mockClear()
})

describe('PreferencesStep', () => {
  it('saves the theme on change and applies it', async () => {
    render(<PreferencesStep />, { wrapper: Wrap })
    fireEvent.click(await screen.findByRole('radio', { name: 'Dark' }))
    expect(setTheme).toHaveBeenCalledWith('dark')
    expect(savePrefs.mock.calls[0]?.[0]).toMatchObject({ theme: 'dark' })
  })

  it('links to the full appearance page', async () => {
    render(<PreferencesStep />, { wrapper: Wrap })
    await screen.findByRole('radio', { name: 'Dark' })
    const link = screen
      .getAllByRole('link')
      .find((a) => a.getAttribute('href') === '/settings/appearance')
    expect(link).toBeTruthy()
  })
})
