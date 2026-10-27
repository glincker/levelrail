import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { EnableHttpsCard } from './EnableHttpsCard'
import httpsEn from '../locales/en/https.json'
import type { HttpsStatus } from '../queries/httpsStatus'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['https'],
  defaultNS: 'https',
  resources: { en: { https: httpsEn } },
  interpolation: { escapeValue: false },
})

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderCard(compact = false) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <EnableHttpsCard compact={compact} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

function stubStatus(status: HttpsStatus) {
  const fetchMock = vi.fn<
    (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
  >(() => Promise.resolve(json(status)))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('EnableHttpsCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('collapses to one line with a single button in compact mode and expands on demand', async () => {
    const user = userEvent.setup()
    stubStatus({
      state: 'off',
      suggested_domain: '203-0-113-5.sslip.io',
      staging: false,
    })
    renderCard(true)
    const setUp = await screen.findByRole('button', { name: 'Set up' })
    expect(screen.queryByText(/sslip\.io/)).not.toBeInTheDocument()
    expect(screen.getByText(/not secure/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/email for certificate/i)).toBeNull()
    await user.click(setUp)
    expect(
      await screen.findByLabelText(/email for certificate/i),
    ).toBeInTheDocument()
  })

  it('offers one-click enable for the suggested sslip.io hostname', async () => {
    const user = userEvent.setup()
    const fetchMock = stubStatus({
      state: 'off',
      suggested_domain: '203-0-113-5.sslip.io',
      staging: false,
    })
    renderCard()
    await user.type(
      await screen.findByLabelText(/email for certificate/i),
      'a@example.com',
    )
    await user.click(screen.getByRole('button', { name: 'Enable HTTPS' }))
    await waitFor(() => {
      const post = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'POST',
      )
      expect(post?.[1]?.body).toBe(
        JSON.stringify({ email: 'a@example.com', staging: false }),
      )
    })
  })

  it('prefills the contact email from the admin account and enables in one click', async () => {
    const user = userEvent.setup()
    const fetchMock = stubStatus({
      state: 'off',
      suggested_domain: '203-0-113-5.sslip.io',
      staging: false,
      default_email: 'admin@glincker.com',
    })
    renderCard()
    expect(await screen.findByLabelText(/email for certificate/i)).toHaveValue(
      'admin@glincker.com',
    )
    expect(screen.getByText(/plain HTTP stops working/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Enable HTTPS' }))
    await waitFor(() => {
      const post = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'POST',
      )
      expect(post?.[1]?.body).toBe(
        JSON.stringify({ email: 'admin@glincker.com', staging: false }),
      )
    })
  })

  it('offers a re-check when issuance failed', async () => {
    stubStatus({
      state: 'failed',
      domain: '203-0-113-5.sslip.io',
      staging: false,
      hint: 'dns',
    })
    renderCard()
    expect(
      await screen.findByRole('button', { name: 'Re-check' }),
    ).toBeInTheDocument()
  })

  it('explains a missing public IP instead of showing a form', async () => {
    stubStatus({ state: 'off', staging: false })
    renderCard()
    expect(await screen.findByText(/APP_PUBLIC_HOST/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Enable HTTPS' })).toBeNull()
  })

  it('shows the CA error with localized guidance when issuance fails', async () => {
    stubStatus({
      state: 'failed',
      domain: '203-0-113-5.sslip.io',
      staging: false,
      hint: 'unreachable',
      error: 'Timeout during connect',
    })
    renderCard()
    expect(
      await screen.findByText(/could not reach this server on port 80/),
    ).toBeInTheDocument()
    expect(screen.getByText('Timeout during connect')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Try again' }),
    ).toBeInTheDocument()
  })
})
