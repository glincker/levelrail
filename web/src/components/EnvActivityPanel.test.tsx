import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { EnvActivityPanel } from './EnvActivityPanel'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: { children?: ReactNode; to?: string } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

function requestUrlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function fakeJsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <EnvActivityPanel appName="demo-app" />
    </QueryClientProvider>,
  )
}

describe('EnvActivityPanel', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('requests only this app PUT trail and renders it generically, never a value diff', async () => {
    const fetchMock = vi.fn<(input: RequestInfo | URL) => Promise<Response>>(
      () =>
        Promise.resolve(
          fakeJsonResponse([
            {
              id: 'aud_1',
              actor_type: 'session',
              actor_id: 'user_1',
              actor_name: 'admin',
              ability: 'write',
              method: 'PUT',
              path: '/api/v1/apps/demo-app',
              status_code: 200,
              remote_addr: '127.0.0.1',
              created_at: '2026-01-01T00:00:00.000000000Z',
            },
          ]),
        ),
    )
    vi.stubGlobal('fetch', fetchMock)

    renderPanel()

    await screen.findByText('admin')
    expect(screen.getByText('Configuration updated')).toBeInTheDocument()
    expect(screen.queryByText(/FOO/)).not.toBeInTheDocument()

    const call = fetchMock.mock.calls[0]
    expect(call).toBeDefined()
    const url = requestUrlOf(call?.[0] ?? '')
    expect(url).toContain('path=%2Fapi%2Fv1%2Fapps%2Fdemo-app')
    expect(url).toContain('method=PUT')
  })

  it('shows an empty state when there is no recorded activity', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(fakeJsonResponse([]))),
    )
    renderPanel()
    await screen.findByText('No recorded configuration changes yet.')
  })

  it('fails quietly (renders nothing) when the audit log is unreachable, e.g. a non-root actor', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(fakeJsonResponse({ error: 'forbidden' }, 403))),
    )
    const { container } = render(
      <QueryClientProvider
        client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      >
        <EnvActivityPanel appName="demo-app" />
      </QueryClientProvider>,
    )
    await waitFor(() => {
      expect(container).toBeEmptyDOMElement()
    })
  })

  it('links out to the full audit log page', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(fakeJsonResponse([]))),
    )
    renderPanel()
    const link = await screen.findByRole('link', { name: /audit log/i })
    expect(link).toHaveAttribute('href', '/settings/audit-log')
  })
})
