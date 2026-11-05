import { QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '../test/commonI18n'
import { createTestQueryClient } from '@/test/testQueryClient'
import { SsoAutoRedirect } from './SsoAutoRedirect'
import { SSO_GUARD_KEY, SSO_REDIRECT_DELAY_MS } from '../lib/ssoAutoRedirect'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))

function stubOptions(auto: boolean) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            code_login: false,
            trusted_device_days: 0,
            email_configured: false,
            sso_auto_redirect: auto,
            sso_provider: auto ? 'github' : undefined,
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    ),
  )
}

const navigate = vi.fn()

async function mount(props: {
  passwordMode?: boolean
  sessionEnded?: boolean
}) {
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <CommonI18n>
        <SsoAutoRedirect
          passwordMode={props.passwordMode ?? false}
          sessionEnded={props.sessionEnded ?? false}
          navigate={navigate}
        />
      </CommonI18n>
    </QueryClientProvider>,
  )
  await act(async () => {
    await Promise.resolve()
  })
}

async function elapse() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(SSO_REDIRECT_DELAY_MS + 50)
  })
}

describe('SsoAutoRedirect', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    window.localStorage.clear()
    window.sessionStorage.clear()
    window.history.pushState(null, '', '/login')
    navigate.mockClear()
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('starts the provider after the delay when enabled', async () => {
    stubOptions(true)
    await mount({})
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Redirecting you to GitHub...',
    )
    await elapse()
    expect(navigate).toHaveBeenCalledWith('/api/v1/auth/oauth/github/start')
  })

  it('does nothing when the setting is off', async () => {
    stubOptions(false)
    await mount({})
    await elapse()
    expect(navigate).not.toHaveBeenCalled()
  })

  it('skips in password mode', async () => {
    stubOptions(true)
    await mount({ passwordMode: true })
    await elapse()
    expect(navigate).not.toHaveBeenCalled()
  })

  it('skips after an error param and offers the password link', async () => {
    window.history.pushState(null, '', '/login?oauth_error=provider_denied')
    stubOptions(true)
    await mount({})
    await elapse()
    expect(navigate).not.toHaveBeenCalled()
    expect(
      screen.getByText('Sign in with a password instead'),
    ).toBeInTheDocument()
  })

  it('skips after sign out', async () => {
    stubOptions(true)
    await mount({ sessionEnded: true })
    await elapse()
    expect(navigate).not.toHaveBeenCalled()
  })

  it('skips when already signed in', async () => {
    window.localStorage.setItem('levelrail.username', 'ada')
    const authStore = await import('../lib/authStore')
    vi.spyOn(authStore, 'getStoredUsername').mockReturnValue('ada')
    stubOptions(true)
    await mount({})
    await elapse()
    expect(navigate).not.toHaveBeenCalled()
  })

  it('skips when an attempt happened in the last minute', async () => {
    window.sessionStorage.setItem(SSO_GUARD_KEY, String(Date.now()))
    stubOptions(true)
    await mount({})
    await elapse()
    expect(navigate).not.toHaveBeenCalled()
  })

  it('records the attempt so a second visit is guarded', async () => {
    stubOptions(true)
    await mount({})
    await screen.findByRole('status')
    await elapse()
    expect(window.sessionStorage.getItem(SSO_GUARD_KEY)).not.toBeNull()
  })

  it('stops when Cancel is pressed', async () => {
    stubOptions(true)
    await mount({})
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    await user.click(await screen.findByRole('button', { name: 'Cancel' }))
    await elapse()
    expect(navigate).not.toHaveBeenCalled()
    expect(
      screen.getByText('Sign in with a password instead'),
    ).toBeInTheDocument()
  })
})
