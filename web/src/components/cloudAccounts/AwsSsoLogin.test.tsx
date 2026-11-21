import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AwsSsoLogin } from './AwsSsoLogin'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

function reply(body: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

describe('AwsSsoLogin', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows the device code and keeps tokens out of the page', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation((input) => {
      const url = String(input instanceof Request ? input.url : input)
      if (url.endsWith('/aws/sso/start')) {
        return reply({
          session_id: 's1',
          user_code: 'ABCD-EFGH',
          verification_uri: 'https://v',
          verification_uri_complete: 'https://v?c=1',
          interval: 60,
          expires_in: 600,
        })
      }
      return reply({ status: 'pending', interval: 60 })
    })
    const qc = new QueryClient()
    render(
      <QueryClientProvider client={qc}>
        <AwsSsoLogin />
      </QueryClientProvider>,
    )
    const start = screen.getByRole('button', { name: 'sso.start' })
    expect(start).toBeDisabled()
    await userEvent.type(
      screen.getByPlaceholderText('sso.regionExample'),
      'eu-west-1',
    )
    await userEvent.type(
      screen.getByPlaceholderText('sso.startUrlExample'),
      'https://d-1.awsapps.com/start',
    )
    await userEvent.click(start)
    await waitFor(() => {
      expect(screen.getByText('ABCD-EFGH')).toBeInTheDocument()
    })
    expect(
      screen.getByRole('link', { name: 'sso.openBrowser' }),
    ).toHaveAttribute('href', 'https://v?c=1')
  })
})
