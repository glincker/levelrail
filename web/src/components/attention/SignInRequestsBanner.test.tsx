import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import signInEn from '../../locales/en/signIn.json'
import { SignInRequestsBanner } from './SignInRequestsBanner'
import { createTestQueryClient } from '@/test/testQueryClient'

let mockPath = '/'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="/security">{children}</a>
  ),
  useRouterState: ({
    select,
  }: {
    select: (s: { location: { pathname: string } }) => string
  }) => select({ location: { pathname: mockPath } }),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['signIn'],
  defaultNS: 'signIn',
  resources: { en: { signIn: signInEn } },
  interpolation: { escapeValue: false },
})

type Routes = Record<string, unknown>

function stubFetch(routes: Routes, bodies: Record<string, string> = {}) {
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string, init?: RequestInit) => {
      const key = `${init?.method ?? 'GET'} ${input}`
      calls.push(key)
      if (typeof init?.body === 'string') {
        bodies[key] = init.body
      }
      const body = routes[key]
      if (body === undefined) {
        return Promise.resolve(new Response(null, { status: 204 }))
      }
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
  return calls
}

function renderBanner() {
  const client = createTestQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <SignInRequestsBanner />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

const now = new Date().toISOString()
const later = new Date(Date.now() + 300_000).toISOString()
const waiting = {
  codes: [
    {
      id: 'lc_1',
      requester_ip: '203.0.113.7',
      user_agent: 'Firefox',
      created_at: now,
      expires_at: now,
      revealable: true,
    },
  ],
  approvals: [
    {
      id: 'la_1',
      requester_ip: '198.51.100.2',
      user_agent: 'Safari on macOS',
      created_at: now,
      expires_at: later,
    },
  ],
}

afterEach(() => {
  mockPath = '/'
  vi.unstubAllGlobals()
})

describe('SignInRequestsBanner', () => {
  it('renders nothing when no sign-in is waiting', async () => {
    const calls = stubFetch({
      'GET /api/v1/auth/sign-in-requests': { codes: [], approvals: [] },
    })
    const { container } = renderBanner()
    await waitFor(() => expect(calls.length).toBeGreaterThan(0))
    expect(container).toBeEmptyDOMElement()
  })

  it('hides on pages that already show the sign-in card', async () => {
    mockPath = '/approve-sign-in'
    const calls = stubFetch({
      'GET /api/v1/auth/sign-in-requests': waiting,
    })
    const { container } = renderBanner()
    await waitFor(() => expect(calls.length).toBeGreaterThan(0))
    expect(container).toBeEmptyDOMElement()
  })

  it('shows requester context but no code until asked', async () => {
    const calls = stubFetch({
      'GET /api/v1/auth/sign-in-requests': waiting,
      'POST /api/v1/auth/sign-in-requests/codes/lc_1/reveal': {
        code: 'ABCD-EF12',
        expires_at: now,
      },
    })
    renderBanner()
    expect(
      await screen.findByText(/203\.0\.113\.7, Firefox/),
    ).toBeInTheDocument()
    expect(screen.getByText('Safari, macOS')).toBeInTheDocument()
    expect(screen.getByText('198.51.100.2')).toBeInTheDocument()
    expect(screen.queryByText('ABCD-EF12')).not.toBeInTheDocument()
    expect(calls.some((c) => c.includes('/reveal'))).toBe(false)
    fireEvent.click(screen.getByRole('button', { name: 'Show code' }))
    expect(await screen.findByText('ABCD-EF12')).toBeInTheDocument()
  })

  it('approves only after the approver types the shown number', async () => {
    const bodies: Record<string, string> = {}
    const calls = stubFetch(
      { 'GET /api/v1/auth/sign-in-requests': waiting },
      bodies,
    )
    renderBanner()
    fireEvent.click(await screen.findByRole('button', { name: 'Approve' }))
    expect(calls.some((c) => c.includes('/approve'))).toBe(false)
    const approve = screen.getByRole('button', { name: 'Approve' })
    const first = screen.getByRole('textbox', { name: 'Digit 1 of 2' })
    fireEvent.change(first, { target: { value: '4' } })
    expect(approve).toBeDisabled()
    fireEvent.change(screen.getByRole('textbox', { name: 'Digit 2 of 2' }), {
      target: { value: '2' },
    })
    fireEvent.click(approve)
    const key = 'POST /api/v1/auth/login-approvals/la_1/approve'
    await waitFor(() => expect(calls).toContain(key))
    expect(JSON.parse(bodies[key] ?? '{}')).toEqual({ match: 42 })
  })

  it('offers the password only for a sign-in from this computer', async () => {
    stubFetch({ 'GET /api/v1/auth/sign-in-requests': waiting })
    renderBanner()
    fireEvent.click(await screen.findByRole('button', { name: 'Approve' }))
    expect(screen.queryByText('No code? Approve with your password')).toBeNull()
  })

  it('approves with the password and shows the code recommendation', async () => {
    const local = {
      codes: [],
      approvals: [{ ...waiting.approvals[0], requester_ip: '127.0.0.1' }],
    }
    const bodies: Record<string, string> = {}
    const calls = stubFetch(
      { 'GET /api/v1/auth/sign-in-requests': local },
      bodies,
    )
    renderBanner()
    fireEvent.click(await screen.findByRole('button', { name: 'Approve' }))
    fireEvent.click(screen.getByText('No code? Approve with your password'))
    expect(screen.getByText(/A code is recommended/)).toBeInTheDocument()
    const approve = screen.getByRole('button', { name: 'Approve' })
    expect(approve).toBeDisabled()
    fireEvent.change(screen.getByLabelText('Your account password'), {
      target: { value: 'hunter2-long-enough' },
    })
    fireEvent.click(approve)
    const key = 'POST /api/v1/auth/login-approvals/la_1/approve'
    await waitFor(() => expect(calls).toContain(key))
    expect(JSON.parse(bodies[key] ?? '{}')).toEqual({
      step_up: { method: 'password', password: 'hunter2-long-enough' },
    })
  })
})

describe('SignInRequestsBanner states', () => {
  it('shows the expired state with no buttons', async () => {
    stubFetch({
      'GET /api/v1/auth/sign-in-requests': {
        codes: [],
        approvals: [{ ...waiting.approvals[0], expires_at: now }],
      },
    })
    renderBanner()
    expect(await screen.findByText('This request expired')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Approve' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Deny' })).toBeNull()
  })

  it('denies without a number and shows the result', async () => {
    const calls = stubFetch({ 'GET /api/v1/auth/sign-in-requests': waiting })
    renderBanner()
    fireEvent.click(await screen.findByRole('button', { name: 'Deny' }))
    await waitFor(() =>
      expect(calls).toContain('POST /api/v1/auth/login-approvals/la_1/deny'),
    )
    expect(
      await screen.findByRole('link', {
        name: 'Review your security settings',
      }),
    ).toBeInTheDocument()
  })
})
