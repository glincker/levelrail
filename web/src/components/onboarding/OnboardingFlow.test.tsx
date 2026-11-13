import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { MeOnboarding } from '../../queries/meOnboarding'
import { Wrap } from './onboardingTestKit'

const mutate =
  vi.fn<(update: unknown, opts?: { onSuccess?: () => void }) => void>()
const navigate = vi.fn()
let current: MeOnboarding | null

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
  useNavigate: () => navigate,
}))
vi.mock('../../queries/meOnboarding', async (orig) => ({
  ...(await orig<typeof import('../../queries/meOnboarding')>()),
  useMeOnboarding: () => ({ data: current, isPending: false }),
  useSetOnboarding: () => ({ mutate, isPending: false }),
  useResetOnboarding: () => ({ mutate: vi.fn(), isPending: false }),
}))
vi.mock('./ProfileStep', () => ({ ProfileStep: () => <p>profile body</p> }))
vi.mock('./PreferencesStep', () => ({
  PreferencesStep: () => <p>preferences body</p>,
}))

import { OnboardingFlow } from './OnboardingFlow'

const onboarding = (over: Partial<MeOnboarding>): MeOnboarding => ({
  state: 'not_started',
  current_step: 'welcome',
  flow_version: 1,
  enabled: true,
  ...over,
})

const fetchMock =
  vi.fn<(url: string, init?: RequestInit) => Promise<Response>>()

beforeEach(() => {
  mutate.mockClear()
  navigate.mockClear()
  fetchMock.mockReset()
  fetchMock.mockImplementation((_url: string, init?: RequestInit) =>
    Promise.resolve(
      new Response(
        init?.method === 'PUT'
          ? typeof init.body === 'string'
            ? init.body
            : ''
          : JSON.stringify({
              timezone: '',
              language: '',
              theme: '',
              landing_page: '',
              gravatar: false,
            }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    ),
  )
  vi.stubGlobal('fetch', fetchMock)
})

describe('OnboardingFlow', () => {
  it('renders nothing of the flow when the server has none', () => {
    current = null
    render(<OnboardingFlow />, { wrapper: Wrap })
    expect(screen.getByText(/not available on this server/i)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Start' })).toBeNull()
  })

  it('opens a new account on the language step and saves the choice on Continue', async () => {
    current = onboarding({})
    render(<OnboardingFlow />, { wrapper: Wrap })
    expect(
      screen.getByRole('heading', { name: 'Choose your language' }),
    ).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Back' })).toBeNull()
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/auth/preferences',
        expect.anything(),
      ),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
    expect(mutate.mock.calls[0]?.[0]).toEqual({
      state: 'in_progress',
      current_step: 'welcome',
    })
    await waitFor(() => {
      const put = fetchMock.mock.calls.find(([, i]) => i?.method === 'PUT')
      expect(put?.[0]).toBe('/api/v1/auth/preferences')
    })
  })

  it('starts, goes back and records the step', () => {
    current = onboarding({ state: 'in_progress' })
    render(<OnboardingFlow />, { wrapper: Wrap })
    fireEvent.click(screen.getByRole('button', { name: 'Start' }))
    expect(mutate.mock.calls[0]?.[0]).toEqual({
      state: 'in_progress',
      current_step: 'profile',
    })
    expect(screen.getByText('profile body')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    expect(mutate.mock.calls[1]?.[0]).toEqual({
      state: 'in_progress',
      current_step: 'welcome',
    })
  })

  it('resumes at the stored step', () => {
    current = onboarding({ state: 'in_progress', current_step: 'preferences' })
    render(<OnboardingFlow />, { wrapper: Wrap })
    expect(screen.getByText('preferences body')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Back' })).toBeTruthy()
  })

  it('skip snoozes and leaves to the dashboard', () => {
    current = onboarding({ state: 'in_progress', current_step: 'profile' })
    render(<OnboardingFlow />, { wrapper: Wrap })
    fireEvent.click(screen.getByRole('button', { name: 'Skip for now' }))
    const call = mutate.mock.calls[0]
    expect(call?.[0]).toEqual({ state: 'snoozed', current_step: 'profile' })
    call?.[1]?.onSuccess?.()
    expect(navigate).toHaveBeenCalledWith({ to: '/' })
  })

  it('finishes with completed', () => {
    current = onboarding({ state: 'in_progress', current_step: 'finish' })
    render(<OnboardingFlow />, { wrapper: Wrap })
    fireEvent.click(screen.getByRole('button', { name: 'Finish' }))
    expect(mutate.mock.calls[0]?.[0]).toEqual({ state: 'completed' })
  })

  it('moves focus to the step heading after a keyboard Continue', async () => {
    current = onboarding({ state: 'in_progress' })
    const user = userEvent.setup()
    render(<OnboardingFlow />, { wrapper: Wrap })
    screen.getByRole('button', { name: 'Start' }).focus()
    await user.keyboard('{Enter}')
    expect(document.activeElement).toBe(
      screen.getByRole('heading', { name: 'Your profile' }),
    )
  })
})
