import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import signInEn from '../locales/en/signIn.json'
import { TwoFactorVerifyForm } from './TwoFactorVerifyForm'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useRouter: () => ({ history: { push: vi.fn() } }),
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

afterEach(() => {
  vi.unstubAllGlobals()
})

function stubFetch() {
  const bodies: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string, init?: RequestInit) => {
      if (input === '/api/v1/auth/login-options') {
        return Promise.resolve(
          new Response(
            JSON.stringify({ code_login: true, trusted_device_days: 30 }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        )
      }
      bodies.push(typeof init?.body === 'string' ? init.body : '')
      return Promise.resolve(new Response('{}', { status: 401 }))
    }),
  )
  return bodies
}

function renderForm() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <TwoFactorVerifyForm mfaToken="m1" onBack={vi.fn()} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('TwoFactorVerifyForm remember this browser', () => {
  const cases = [
    { name: 'is off unless ticked', tick: false, want: false },
    { name: 'is sent when ticked', tick: true, want: true },
  ]
  for (const c of cases) {
    it(c.name, async () => {
      const bodies = stubFetch()
      const user = userEvent.setup()
      renderForm()
      const box = await screen.findByRole('checkbox', {
        name: 'Remember this browser for 30 days',
      })
      expect(box).not.toBeChecked()
      if (c.tick) {
        await user.click(box)
      }
      await user.type(screen.getByLabelText('Authenticator code'), '123456')
      await user.click(screen.getByRole('button', { name: 'Verify' }))
      await waitFor(() => {
        expect(bodies.length).toBe(1)
      })
      const sent = JSON.parse(bodies[0] ?? '{}') as {
        remember_device: boolean
      }
      expect(sent.remember_device).toBe(c.want)
    })
  }
})
