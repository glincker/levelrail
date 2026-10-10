import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import signInEn from '../../locales/en/signIn.json'
import { SignInRequestsBanner } from './SignInRequestsBanner'

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

function stubFetch(routes: Routes) {
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string, init?: RequestInit) => {
      const key = `${init?.method ?? 'GET'} ${input}`
      calls.push(key)
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
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <SignInRequestsBanner />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

const now = new Date().toISOString()
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
      user_agent: 'Safari',
      created_at: now,
      expires_at: now,
    },
  ],
}

afterEach(() => {
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
    expect(screen.getByText(/198\.51\.100\.2, Safari/)).toBeInTheDocument()
    expect(screen.queryByText('ABCD-EF12')).not.toBeInTheDocument()
    expect(calls.some((c) => c.includes('/reveal'))).toBe(false)
    fireEvent.click(screen.getByRole('button', { name: 'Show code' }))
    expect(await screen.findByText('ABCD-EF12')).toBeInTheDocument()
  })

  it('approves a new browser sign-in', async () => {
    const calls = stubFetch({ 'GET /api/v1/auth/sign-in-requests': waiting })
    renderBanner()
    fireEvent.click(await screen.findByRole('button', { name: 'Approve' }))
    await waitFor(() =>
      expect(calls).toContain('POST /api/v1/auth/login-approvals/la_1/approve'),
    )
  })
})
