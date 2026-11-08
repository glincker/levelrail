import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/mailServer.json'
import { MailCertCard } from './MailCertCard'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { mailServer: en } },
  interpolation: { escapeValue: false },
})

const cert = {
  app: 'mail',
  hostname: 'mail.example.com',
  source: 'uploaded',
  served: true,
  issuer: 'Example CA',
  days_left: 60,
  self_signed: false,
  restart_window: '',
  next_step: '',
}

afterEach(() => vi.unstubAllGlobals())

function renderCard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <MailCertCard installed />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('MailCertCard', () => {
  it('shows the served certificate and posts the obtain request', async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) =>
      Promise.resolve(
        init?.method === 'POST'
          ? new Response(JSON.stringify({ error: 'not yet' }), { status: 501 })
          : new Response(JSON.stringify(cert), { status: 200 }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderCard()
    expect(await screen.findByText('mail.example.com')).toBeTruthy()
    expect(screen.getByText('60 days left')).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'Obtain certificate' }),
    )
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/mail/cert/obtain',
        expect.objectContaining({ method: 'POST' }),
      ),
    )
  })
})
