import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    useRouter: () => ({ invalidate: vi.fn() }),
    Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
  }
})

import { SignInFlow } from './SignInFlow'

function mockFetch(handler: (url: string) => Response) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input)
      return Promise.resolve(handler(url))
    }),
  )
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function renderFlow(username: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <SignInFlow username={username} onUsernameChange={vi.fn()} />
    </QueryClientProvider>,
  )
}

describe('SignInFlow', () => {
  const originalCredential = window.PublicKeyCredential

  afterEach(() => {
    vi.unstubAllGlobals()
    window.PublicKeyCredential = originalCredential
  })

  it('offers a passkey prompt when the account has one registered', async () => {
    // jsdom has no WebAuthn implementation; stub presence to simulate a supporting browser.
    // @ts-expect-error minimal stub, only presence is checked
    window.PublicKeyCredential = class {}
    const user = userEvent.setup()
    mockFetch((url) =>
      url.includes('/passkey-login/begin')
        ? json({
            session_id: 'sess-1',
            options: { publicKey: { challenge: 'abc', rpId: 'localhost' } },
          })
        : json({}, 404),
    )
    renderFlow('alice')
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    expect(
      await screen.findByRole('button', { name: 'Sign in as alice' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Use your password instead' }),
    ).toBeInTheDocument()
  })

  it('falls back to a password field when the account has no passkey', async () => {
    // @ts-expect-error minimal stub, only presence is checked
    window.PublicKeyCredential = class {}
    const user = userEvent.setup()
    mockFetch((url) =>
      url.includes('/passkey-login/begin')
        ? json({ error: 'no passkey available for this account' }, 401)
        : json({}, 404),
    )
    renderFlow('bob')
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() => {
      expect(screen.getByLabelText('Password')).toBeInTheDocument()
    })
    expect(screen.getByText(/Signing in as/)).toHaveTextContent('bob')
    expect(screen.queryByLabelText('Username')).not.toBeInTheDocument()
  })

  it('does not call the passkey endpoint when the browser has no WebAuthn support', async () => {
    // jsdom has no WebAuthn implementation by default, this is the real baseline.
    delete (window as { PublicKeyCredential?: unknown }).PublicKeyCredential
    const fetchSpy = vi.fn(() => Promise.resolve(json({}, 404)))
    vi.stubGlobal('fetch', fetchSpy)
    const user = userEvent.setup()
    renderFlow('carol')
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    await waitFor(() => {
      expect(screen.getByLabelText('Password')).toBeInTheDocument()
    })
    expect(fetchSpy).not.toHaveBeenCalled()
  })
})
