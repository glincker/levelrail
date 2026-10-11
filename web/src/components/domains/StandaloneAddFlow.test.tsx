import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AddDomainsDialog } from '../AddDomainsDialog'
import domainsEn from '../../locales/en/domains.json'

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

const serverAddress = {
  ipv4: '203.0.113.4',
  ipv6: '2001:db8::4',
  source: 'detected',
  detected: { ipv4: '203.0.113.4' },
  override: {},
  behind_nat: true,
  tls_terminated_upstream: false,
  records: [{ type: 'A', name: 'test.example.com', value: '203.0.113.4' }],
}

function stubApi(posts: unknown[]) {
  const fetchMock = vi.fn(
    (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
      const url = input as string
      if (url.startsWith('/api/v1/system/server-address')) {
        return Promise.resolve(res(serverAddress))
      }
      if (url.endsWith('/standalone/candidates')) {
        return Promise.resolve(
          res({
            host_ports: [
              { port: 3000 },
              { port: 2019, label: 'ingress admin API', blocked: true },
            ],
            containers: [],
            apps: [{ name: 'web', port: 8080 }],
          }),
        )
      }
      if (url.endsWith('/standalone/validate')) {
        return Promise.resolve(
          res({
            target_url: 'http://127.0.0.1:3000',
            assessment: {
              internal: true,
              loopback: true,
              needs_confirmation: ['this server will connect to 127.0.0.1'],
            },
          }),
        )
      }
      if (url === '/api/v1/domains/standalone' && init?.method === 'POST') {
        posts.push(JSON.parse(init.body as string))
        return Promise.resolve(
          res(
            {
              domain: 'test.example.com',
              kind: 'standalone',
              target: '127.0.0.1:3000',
              target_url: 'http://127.0.0.1:3000',
              websocket: true,
            },
            201,
          ),
        )
      }
      return Promise.resolve(res({ steps: [] }))
    },
  )
  vi.stubGlobal('fetch', fetchMock)
}

function renderDialog(apps: { name: string }[] = [{ name: 'web' }]) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <AddDomainsDialog
          open
          onOpenChange={() => undefined}
          apps={apps}
          claimedBy={new Map()}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('AddDomainsDialog proxy mode', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('starts in app mode and switches to proxy mode', async () => {
    stubApi([])
    const user = userEvent.setup()
    renderDialog()
    expect(
      screen.getByRole('radio', { name: 'Route to an app' }),
    ).toHaveAttribute('aria-checked', 'true')
    expect(await screen.findByText('203.0.113.4')).toBeInTheDocument()
    expect(screen.getByText('2001:db8::4')).toBeInTheDocument()
    await user.click(
      screen.getByRole('radio', { name: 'Proxy to a port or URL' }),
    )
    expect(screen.getByRole('textbox', { name: 'Domain' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add domain' })).toBeDisabled()
  })

  it('opens in proxy mode when there are no apps', () => {
    stubApi([])
    renderDialog([])
    expect(
      screen.getByRole('radio', { name: 'Proxy to a port or URL' }),
    ).toHaveAttribute('aria-checked', 'true')
  })

  it('needs confirmation for a loopback target, then shows the record to create', async () => {
    const posts: unknown[] = []
    stubApi(posts)
    const user = userEvent.setup()
    renderDialog()
    await user.click(
      screen.getByRole('radio', { name: 'Proxy to a port or URL' }),
    )
    await user.type(
      screen.getByRole('textbox', { name: 'Domain' }),
      'test.example.com',
    )
    await user.click(await screen.findByRole('button', { name: /^3000/ }))
    expect(screen.getByRole('button', { name: /^2019/ })).toBeDisabled()
    expect(screen.getByText('https://test.example.com')).toBeInTheDocument()

    expect(
      await screen.findByText('this server will connect to 127.0.0.1'),
    ).toBeInTheDocument()
    const submit = screen.getByRole('button', { name: 'Add domain' })
    expect(submit).toBeDisabled()
    await user.click(screen.getByRole('checkbox'))
    await waitFor(() => {
      expect(submit).toBeEnabled()
    })
    await user.click(submit)

    await waitFor(() => {
      expect(posts).toHaveLength(1)
    })
    expect(posts[0]).toMatchObject({
      domain: 'test.example.com',
      target: '127.0.0.1:3000',
      confirm: true,
      websocket: true,
    })
    expect(await screen.findByText('Make it live')).toBeInTheDocument()
    expect(screen.getByText('DNS record to create')).toBeInTheDocument()
  })
})
