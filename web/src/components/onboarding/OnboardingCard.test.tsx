import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { MeOnboarding } from '../../queries/meOnboarding'
import { Wrap } from './onboardingTestKit'

let current: MeOnboarding | null | undefined

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('../../queries/meOnboarding', async (orig) => ({
  ...(await orig<typeof import('../../queries/meOnboarding')>()),
  useMeOnboarding: () => ({ data: current }),
  useSetOnboarding: () => ({ mutate: vi.fn(), isPending: false }),
}))

import { OnboardingCard } from './OnboardingCard'

const flow = (over: Partial<MeOnboarding>): MeOnboarding => ({
  state: 'not_started',
  current_step: 'welcome',
  flow_version: 1,
  enabled: true,
  ...over,
})

describe('OnboardingCard', () => {
  it('shows for a new user', () => {
    current = flow({})
    render(<OnboardingCard />, { wrapper: Wrap })
    expect(screen.getByText('Set up your dashboard')).toBeTruthy()
  })

  it('offers to continue when in progress', () => {
    current = flow({ state: 'in_progress', current_step: 'profile' })
    render(<OnboardingCard />, { wrapper: Wrap })
    expect(screen.getByText('You stopped at step 3 of 5.')).toBeTruthy()
  })

  it.each([
    ['missing route', null],
    ['loading', undefined],
    ['disabled', flow({ enabled: false })],
    ['completed', flow({ state: 'completed' })],
    ['dismissed', flow({ state: 'dismissed' })],
    ['snoozed', flow({ state: 'snoozed' })],
  ])('renders nothing when %s', (_name, value) => {
    current = value
    const { container } = render(<OnboardingCard />, { wrapper: Wrap })
    expect(container.textContent).toBe('')
  })
})
