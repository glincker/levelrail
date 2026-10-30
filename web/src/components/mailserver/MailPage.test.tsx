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
