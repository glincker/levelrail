import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import signInEn from '../locales/en/signIn.json'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    useRouter: () => ({ invalidate: vi.fn() }),
  }
})
vi.mock('../hooks/useBrand', () => ({
  useBrand: () => ({ BinaryName: 'testplatform' }),
}))

import { ResumedApprovalWait } from './ResumedApprovalWait'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['signIn'],
  defaultNS: 'signIn',
  resources: { en: { signIn: signInEn } },
  interpolation: { escapeValue: false },
})

function renderWait(poll: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify(poll), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    ),
  )
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={client}>
        <ResumedApprovalWait
          approvalId="la_1"
          onBack={vi.fn()}
          onUseCode={vi.fn()}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('ResumedApprovalWait', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the number from the first poll after an OAuth sign-in was held', async () => {
    renderWait({
      status: 'pending',
      match_number: 42,
      expires_at: new Date(Date.now() + 60_000).toISOString(),
    })
    expect(await screen.findByTestId('approval-match')).toHaveTextContent('42')
  })

  it('says so when the held sign-in was denied', async () => {
    renderWait({ status: 'denied' })
    expect(
      await screen.findByText(signInEn.approval.denied),
    ).toBeInTheDocument()
  })
})
