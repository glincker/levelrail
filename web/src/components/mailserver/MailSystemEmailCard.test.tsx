import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/mailServer.json'
import { MailSystemEmailCard } from './MailSystemEmailCard'

vi.mock('../env/StepUpDialog', () => ({ StepUpDialog: () => null }))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { mailServer: en } },
  interpolation: { escapeValue: false },
})

const off = {
  enabled: false,
  active: false,
  certificate_pinned: false,
  has_previous_provider: false,
  domains: ['example.test'],
}
const on = {
  ...off,
  enabled: true,
  active: true,
  domain: 'example.test',
  from_address: 'noreply@example.test',
  submission: '10.0.0.5:587',
  dns: { spf: 'fail', dkim: 'pass', dmarc: 'warn', pass: false },
  warning: 'dns not ready',
}

function renderCard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <MailSystemEmailCard
          domains={[
            {
              domain: 'example.test',
              service: 'mail',
              team_id: 't',
              created_at: '',
            },
          ]}
        />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('MailSystemEmailCard', () => {
  it('enables for the chosen domain and shows the steps, never a password', async () => {
    const calls: { url: string; body?: string }[] = []
    let state: typeof off = off
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push({ url, body: init?.body as string | undefined })
        const json = url.endsWith('/enable')
          ? {
              state: (state = on),
              steps: [{ id: 'mailbox', ok: true, message: 'ready' }],
            }
          : state
        return Promise.resolve(new Response(JSON.stringify(json)))
      }),
    )
    renderCard()
    await userEvent.click(
      await screen.findByRole('button', { name: /use my mail server/i }),
    )
    expect(await screen.findByText(/Mailbox: ready/)).toBeTruthy()
    const enable = calls.find((c) => c.url.endsWith('/enable'))
    expect(JSON.parse(enable?.body ?? '{}')).toMatchObject({
      domain: 'example.test',
      replace_existing: false,
    })
    expect(await screen.findByText('noreply@example.test')).toBeTruthy()
    expect(
      screen.getByRole('link', { name: /dns panel/i }).getAttribute('href'),
    ).toBe('#mail-dns-panel')
    expect(document.body.textContent).not.toMatch(/password:/i)
  })
})
