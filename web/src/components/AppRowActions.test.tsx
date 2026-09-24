import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppRowActions } from './AppRowActions'
import type { AppListEntry } from '../types/appDetail'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      params,
      ...rest
    }: {
      children?: ReactNode
      to?: string
      params?: Record<string, string>
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to?.replace('$name', params?.name ?? '')} {...rest}>
        {children}
      </a>
    ),
  }
})

function makeApp(overrides: Partial<AppListEntry> = {}): AppListEntry {
  return {
    name: 'web',
    image: 'nginx:1.27',
    port: 80,
    suspended: false,
    domains: [],
    status: { label: 'Healthy', variant: 'success' },
    ...overrides,
  } as AppListEntry
}

function calls(fetchMock: ReturnType<typeof vi.fn>): string[] {
  return fetchMock.mock.calls.map(
    ([url, init]) =>
      `${(init as RequestInit | undefined)?.method ?? 'GET'} ${String(url)}`,
  )
}

function stubFetch() {
  const fetchMock = vi.fn(() =>
    Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ name: 'web', image: 'nginx:1.27' }),
    } as unknown as Response),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderActions(app: AppListEntry) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <AppRowActions app={app} />
    </QueryClientProvider>,
  )
}

async function openMenu() {
  await userEvent.click(screen.getByRole('button', { name: 'Actions for web' }))
}

describe('AppRowActions', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows Stop (not Start) for a running app and hides Open domain without a domain', async () => {
    renderActions(makeApp())
    await openMenu()
    expect(
      await screen.findByRole('menuitem', { name: 'Stop' }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('menuitem', { name: 'Start' }),
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Open domain')).not.toBeInTheDocument()
    expect(
      screen.getByRole('menuitem', { name: 'View logs' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('menuitem', { name: 'View deploys' }),
    ).toBeInTheDocument()
  })

  it('shows Start for a suspended app and Open domain when a domain exists', async () => {
    renderActions(makeApp({ suspended: true, domains: ['app.example.com'] }))
    await openMenu()
    expect(
      await screen.findByRole('menuitem', { name: 'Start' }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('menuitem', { name: 'Stop' }),
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('menuitem', { name: 'Open domain' }),
    ).toHaveAttribute('href', 'https://app.example.com')
  })

  it('restarts the app via the API', async () => {
    const fetchMock = stubFetch()
    renderActions(makeApp())
    await openMenu()
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Restart' }),
    )
    await waitFor(() =>
      expect(calls(fetchMock)).toContain('POST /api/v1/apps/web/restart'),
    )
  })

  it('redeploys the current image via the deploys endpoint', async () => {
    const fetchMock = stubFetch()
    renderActions(makeApp())
    await openMenu()
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Redeploy' }),
    )
    await waitFor(() =>
      expect(calls(fetchMock)).toContain('POST /api/v1/apps/web/deploys'),
    )
    const init = (
      fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    )[1]
    expect(JSON.parse(init.body as string)).toMatchObject({
      image: 'nginx:1.27',
    })
  })

  it('asks for confirmation before stopping', async () => {
    const fetchMock = stubFetch()
    renderActions(makeApp())
    await openMenu()
    await userEvent.click(await screen.findByRole('menuitem', { name: 'Stop' }))
    expect(await screen.findByText(/Stop .*web.*\?/)).toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Stop app' }))
    await waitFor(() =>
      expect(calls(fetchMock)).toContain('POST /api/v1/apps/web/stop'),
    )
  })
})
