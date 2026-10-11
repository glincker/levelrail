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

import { NewDeviceApprovalWait } from './NewDeviceApprovalWait'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['signIn'],
  defaultNS: 'signIn',
  resources: { en: { signIn: signInEn } },
  interpolation: { escapeValue: false },
})

function renderWait(expiresAt: string, matchNumber = 47) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => new Promise<Response>(() => undefined)),
  )
  const client = new QueryClient()
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={client}>
        <NewDeviceApprovalWait
          approvalId="la_1"
          expiresAt={expiresAt}
          matchNumber={matchNumber}
          onBack={vi.fn()}
          onUseCode={vi.fn()}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('NewDeviceApprovalWait', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('makes the match number the labelled focal point with a countdown', () => {
    renderWait(new Date(Date.now() + 272_000).toISOString())
    const match = screen.getByTestId('approval-match')
    expect(match).toHaveTextContent('47')
    expect(match).toHaveAttribute('aria-label', 'Match number 47')
    expect(screen.getByTestId('approval-countdown')).toHaveTextContent(
      /Expires in 04:3\d|Expires in 04:2\d/,
    )
    expect(screen.getByRole('status')).toHaveAttribute('aria-live', 'polite')
    expect(
      screen.getByRole('heading', { name: 'Approve this sign-in' }),
    ).toBeInTheDocument()
  })

  it('offers a code and a way back, and has no sign-in alert while waiting', () => {
    renderWait(new Date(Date.now() + 60_000).toISOString())
    expect(
      screen.getByRole('button', { name: 'Sign in with a code instead' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Back to sign in' }),
    ).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('announces expiry with next-step copy once the time has passed', () => {
    renderWait(new Date(Date.now() - 1000).toISOString())
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Go back and sign in again',
    )
    expect(screen.queryByTestId('approval-match')).not.toBeInTheDocument()
  })
})
