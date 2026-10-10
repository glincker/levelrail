import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AddDomainsDialog } from './AddDomainsDialog'
import domainsEn from '../locales/en/domains.json'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

function res(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderDialog(claimed: Record<string, string> = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <AddDomainsDialog
          open
          onOpenChange={() => undefined}
          apps={[{ name: 'web' }]}
          claimedBy={new Map(Object.entries(claimed))}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('AddDomainsDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('previews parsed, repeated, invalid and claimed domains', async () => {
    const user = userEvent.setup()
    renderDialog({ 'taken.com': 'api' })
    await user.click(screen.getByRole('textbox', { name: 'Domains' }))
    await user.paste('a.com, b.com\nA.com nope taken.com')
    expect(screen.getByText('2 domains ready to add')).toBeInTheDocument()
    expect(screen.getByText('Not valid: nope')).toBeInTheDocument()
    expect(
      screen.getByText('Repeated, counted once: a.com'),
    ).toBeInTheDocument()
    expect(
      screen.getByText('taken.com is already routed to api.'),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add 2 domains' })).toBeEnabled()
  })

  it('adds each domain on its own and keeps going after a failure', async () => {
    const fetchMock = vi.fn(
      (_input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
        if (init?.method === 'PATCH') {
          const body = JSON.parse(init.body as string) as { add: string[] }
          return Promise.resolve(
            body.add[0] === 'bad.com'
              ? res({ error: { message: 'domain already in use' } }, 409)
              : res({ app: 'web', domains: body.add, changed: true }),
          )
        }
        return Promise.resolve(
          res({ domain: 'x', status: 'not_resolving', resolved: false }),
        )
      },
    )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    renderDialog()
    await user.click(screen.getByRole('textbox', { name: 'Domains' }))
    await user.paste('ok.com, bad.com, fine.com')
    await user.click(screen.getByRole('button', { name: 'Add 3 domains' }))

    await waitFor(() => {
      expect(screen.getByText('2 added, 1 not added.')).toBeInTheDocument()
    })
    const patches = fetchMock.mock.calls.filter(
      ([, init]) => init?.method === 'PATCH',
    )
    expect(patches).toHaveLength(3)
    expect(screen.getByText('bad.com')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Retry failed' }),
    ).toBeInTheDocument()
  })
})
