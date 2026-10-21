import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SignInApprovalCard } from './SignInApprovalCard'
import { renderWithProviders, stubFetch } from './signInTestUtils'

const now = new Date().toISOString()
const pending = {
  codes: [],
  approvals: [
    {
      id: 'la_1',
      requester_ip: '198.51.100.2',
      user_agent: 'Terminal CLI',
      created_at: now,
      expires_at: now,
    },
  ],
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('SignInApprovalCard', () => {
  it('shows the device, location and Approve/Deny for a waiting sign-in', async () => {
    stubFetch({ 'GET /api/v1/auth/sign-in-requests': { body: pending } })
    renderWithProviders(<SignInApprovalCard />)
    expect(
      await screen.findByRole('heading', { name: 'Approve this sign-in?' }),
    ).toBeInTheDocument()
    expect(
      screen.getByText(/198\.51\.100\.2, Terminal CLI/),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Approve' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Deny' })).toBeInTheDocument()
  })

  it('keeps an announcing live region and renders no card when nothing waits', async () => {
    const calls = stubFetch({
      'GET /api/v1/auth/sign-in-requests': {
        body: { codes: [], approvals: [] },
      },
    })
    renderWithProviders(<SignInApprovalCard />)
    await vi.waitFor(() => {
      expect(calls.length).toBeGreaterThan(0)
    })
    expect(screen.getByLabelText('Sign-in approvals')).toHaveAttribute(
      'aria-live',
      'polite',
    )
    expect(screen.queryByRole('heading')).toBeNull()
  })

  it('explains the empty state on the approval page', async () => {
    stubFetch({
      'GET /api/v1/auth/sign-in-requests': {
        body: { codes: [], approvals: [] },
      },
    })
    renderWithProviders(<SignInApprovalCard showEmpty />)
    expect(
      await screen.findByText(/Nothing is waiting for approval/),
    ).toBeInTheDocument()
  })

  it('moves focus to the card when an approval arrives on the page', async () => {
    stubFetch({ 'GET /api/v1/auth/sign-in-requests': { body: pending } })
    renderWithProviders(<SignInApprovalCard focusOnArrival />)
    const card = await screen.findByRole('region', {
      name: 'Approve this sign-in?',
    })
    await vi.waitFor(() => {
      expect(card).toHaveFocus()
    })
  })
})
