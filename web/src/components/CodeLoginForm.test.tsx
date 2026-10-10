import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import signInEn from '../locales/en/signIn.json'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigate,
    useRouter: () => ({ navigate: vi.fn() }),
  }
})
vi.mock('../hooks/useBrand', () => ({
  useBrand: () => ({ BinaryName: 'testbin' }),
}))

import { CodeLoginForm } from './CodeLoginForm'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['signIn'],
  defaultNS: 'signIn',
  resources: { en: { signIn: signInEn } },
  interpolation: { escapeValue: false },
})

function stubFetch(redeem: { status: number; body: unknown }) {
  const calls: { key: string; body: string }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string, init?: RequestInit) => {
      const key = `${init?.method ?? 'GET'} ${input}`
      calls.push({ key, body: typeof init?.body === 'string' ? init.body : '' })
      const res = key.endsWith('/login-code/redeem')
        ? redeem
        : { status: 202, body: { message: 'Generic message', expires_in: 600 } }
      return Promise.resolve(
        new Response(JSON.stringify(res.body), {
          status: res.status,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
  return calls
}

function renderForm() {
  const client = new QueryClient()
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <CodeLoginForm
          username="dev@example.com"
          onUsernameChange={vi.fn()}
          onBack={vi.fn()}
        />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

async function sendAndEnter(code: string) {
  fireEvent.click(screen.getByRole('button', { name: 'Send code' }))
  expect(await screen.findByText('Generic message')).toBeInTheDocument()
  fireEvent.change(screen.getByLabelText('Code'), { target: { value: code } })
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))
}

beforeEach(() => {
  navigate.mockReset()
})
afterEach(() => {
  vi.unstubAllGlobals()
})

describe('CodeLoginForm', () => {
  it('signs in after a valid code', async () => {
    const calls = stubFetch({
      status: 200,
      body: { username: 'dev@example.com' },
    })
    renderForm()
    await sendAndEnter('abcd-ef12')
    await waitFor(() => expect(navigate).toHaveBeenCalled())
    expect(calls.map((c) => c.key)).toEqual([
      'POST /api/v1/auth/login-code/request',
      'POST /api/v1/auth/login-code/redeem',
    ])
    expect(calls[1]?.body).toContain('abcd-ef12')
  })

  it('shows the server error for a wrong code and does not navigate', async () => {
    stubFetch({ status: 401, body: { error: 'invalid or expired code' } })
    renderForm()
    await sendAndEnter('WRONG123')
    expect(
      await screen.findByText('invalid or expired code'),
    ).toBeInTheDocument()
    expect(navigate).not.toHaveBeenCalled()
  })

  it('asks for the second factor when the account has TOTP', async () => {
    stubFetch({ status: 200, body: { mfa_required: true, mfa_token: 'm1' } })
    renderForm()
    await sendAndEnter('ABCDEF12')
    await waitFor(() =>
      expect(screen.queryByLabelText('Code')).not.toBeInTheDocument(),
    )
    expect(navigate).not.toHaveBeenCalled()
  })
})
