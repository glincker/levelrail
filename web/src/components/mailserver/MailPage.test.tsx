import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/mailServer.json'
import { MailPage } from './MailPage'

vi.mock('@tanstack/react-router', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-router')>()),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('@/components/shell/PageTemplate', () => ({
  PageTemplate: ({
    title,
    actions,
    children,
  }: {
    title: string
    actions: React.ReactNode
    children: React.ReactNode
  }) => (
    <div>
      <h1>{title}</h1>
      {actions}
      {children}
    </div>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { mailServer: en } },
  defaultNS: 'mailServer',
  interpolation: { escapeValue: false },
})

const jsonRes = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <MailPage />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

function statusFetch(body: unknown) {
  return vi.fn((url: string) =>
    Promise.resolve(url.endsWith('/mail/status') ? jsonRes(body) : jsonRes([])),
  )
}

describe('MailPage status', () => {
  it('offers install and a prefilled CLI line when not installed', async () => {
    vi.stubGlobal(
      'fetch',
      statusFetch({
        installed: false,
        container_state: 'none',
        admin_reachable: false,
        tls: 'unknown',
        next_step: 'Install the mail-mox template.',
      }),
    )
    renderPage()
    expect(await screen.findByText('Install the mail server')).toBeTruthy()
    expect(
      screen.getByText(
        /lrcli templates install mail-mox --env MAIL_FIRST_DOMAIN=/,
      ),
    ).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Install' })).toBeTruthy()
  })

  it('warns with the server text when installed but stopped', async () => {
    vi.stubGlobal(
      'fetch',
      statusFetch({
        installed: true,
        container_state: 'stopped',
        admin_reachable: false,
        tls: 'self-signed',
        next_step: 'The mail container is not running.',
      }),
    )
    renderPage()
    expect(await screen.findByText('Start the mail container')).toBeTruthy()
    expect(
      screen.getAllByText('The mail container is not running.').length,
    ).toBeGreaterThan(0)
    expect(screen.getByText('TLS: self-signed')).toBeTruthy()
  })

  it('marks the server done when running and reachable', async () => {
    vi.stubGlobal(
      'fetch',
      statusFetch({
        installed: true,
        container_state: 'running',
        admin_reachable: true,
        tls: 'provided',
        next_step: 'Mail is up.',
      }),
    )
    renderPage()
    expect(await screen.findByText('Add your DNS records')).toBeTruthy()
    expect(screen.getByText('Running and answering')).toBeTruthy()
  })

  it('does not call a running but unreachable server done', async () => {
    vi.stubGlobal(
      'fetch',
      statusFetch({
        installed: true,
        container_state: 'running',
        admin_reachable: false,
        tls: 'self-signed',
        next_step: 'Admin API does not answer yet.',
      }),
    )
    renderPage()
    expect(
      await screen.findByText('Wait for the mail server to answer'),
    ).toBeTruthy()
    expect(screen.queryByText('Running and answering')).toBeNull()
  })
})

describe('MailPage', () => {
  it('shows every unchecked step as not checked and lists the coming panels', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonRes([]))),
    )
    renderPage()
    expect(screen.getAllByText('Not checked').length).toBeGreaterThanOrEqual(8)
    expect(await screen.findAllByText('Coming next')).toHaveLength(5)
    expect(
      screen.getByRole('link', { name: /Install mail server/ }),
    ).toBeTruthy()
  })

  it('plans, then applies with the plan hash, and reflects a check', async () => {
    const calls: string[] = []
    let applyBody = ''
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push(url)
        if (url.endsWith('/plan'))
          return jsonRes({
            domain: 'example.com',
            hash: 'abc123def4567890',
            blocked: false,
            summary: { add: 1 },
            changes: [
              {
                action: 'add',
                kind: 'SPF',
                set: {
                  name: 'example.com',
                  type: 'TXT',
                  ttl: 300,
                  values: ['v=spf1 -all'],
                },
              },
            ],
          })
        if (url.endsWith('/apply')) {
          applyBody = typeof init?.body === 'string' ? init.body : ''
          return jsonRes({
            applied: 1,
            plan: {
              domain: 'example.com',
              hash: 'x',
              blocked: false,
              summary: {},
              changes: [],
            },
          })
        }
        if (url.includes('/check'))
          return jsonRes({
            domain: 'example.com',
            items: [{ id: 'spf', status: 'pass', message: 'SPF ok' }],
          })
        return jsonRes([])
      }),
    )
    const user = userEvent.setup()
    renderPage()
    await user.type(screen.getByLabelText('Domain'), 'example.com')
    await user.click(screen.getByRole('button', { name: 'Preview records' }))
    await user.click(
      await screen.findByRole('button', { name: 'Apply 1 records' }),
    )
    expect(applyBody).toContain('"plan_hash":"abc123def4567890"')
    expect(applyBody).toContain('"confirm":true')
    await user.click(screen.getByRole('button', { name: 'Check' }))
    expect(await screen.findByText('SPF ok')).toBeTruthy()
  })
})
