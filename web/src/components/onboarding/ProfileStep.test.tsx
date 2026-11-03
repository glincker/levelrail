import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Wrap } from './onboardingTestKit'

const updateName = vi.fn()
const savePrefs = vi.fn()

vi.mock('../../queries/security', () => ({
  useSession: () => ({
    data: { display_name: 'Ada Lovelace', username: 'ada@example.com' },
  }),
}))
vi.mock('../../queries/myAccess', () => ({
  useUpdateDisplayName: () => ({
    mutate: updateName,
    isPending: false,
    isError: false,
    isSuccess: false,
  }),
}))
vi.mock('../../queries/accountSelf', () => ({
  preferencesQueryOptions: () => ({
    queryKey: ['prefs-profile-test'],
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
  useTheme: () => ({ theme: 'system', setTheme: vi.fn() }),
}))

import { ProfileStep } from './ProfileStep'

beforeEach(() => {
  updateName.mockClear()
  savePrefs.mockClear()
})

describe('ProfileStep', () => {
  it('saves a changed display name on blur', async () => {
    render(<ProfileStep />, { wrapper: Wrap })
    const input = await screen.findByRole('textbox', { name: 'Display name' })
    fireEvent.change(input, { target: { value: 'Ada B' } })
    fireEvent.blur(input)
    expect(updateName).toHaveBeenCalledWith('Ada B')
  })

  it('does not save an unchanged or empty name', async () => {
    render(<ProfileStep />, { wrapper: Wrap })
    const input = await screen.findByRole('textbox', { name: 'Display name' })
    fireEvent.blur(input)
    fireEvent.change(input, { target: { value: '  ' } })
    fireEvent.blur(input)
    expect(updateName).not.toHaveBeenCalled()
  })

  it('keeps Gravatar off until switched on, then saves it', async () => {
    render(<ProfileStep />, { wrapper: Wrap })
    const toggle = await screen.findByRole('switch', { name: 'Use Gravatar' })
    expect(toggle.getAttribute('aria-checked')).toBe('false')
    fireEvent.click(toggle)
    expect(savePrefs.mock.calls[0]?.[0]).toMatchObject({ gravatar: true })
  })
})
