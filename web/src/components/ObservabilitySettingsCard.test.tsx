import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ObservabilitySettingsCard,
  RemoteReadConnectionCard,
} from './ObservabilitySettingsCard'
import type { ObservabilitySettings } from '../queries/observabilitySettings'

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

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function mockFetch(routes: Record<string, () => Response>) {
  const fn = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.toString()
          : input.url
    const key = `${init?.method ?? 'GET'} ${url}`
    const handler = Object.entries(routes).find(([k]) => key.startsWith(k))
    return handler
      ? Promise.resolve(handler[1]())
      : Promise.reject(new Error(`unexpected fetch: ${key}`))
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

function renderWithClient(node: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>{node}</QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

const settings: ObservabilitySettings = {
  external_dashboard_url: '',
  remote_read_path: '/api/v1/prometheus/read',
}

describe('RemoteReadConnectionCard', () => {
  it('renders the real remote-read URL built from the current origin', () => {
    renderWithClient(<RemoteReadConnectionCard settings={settings} />)
    expect(
      screen.getByText(`${window.location.origin}/api/v1/prometheus/read`),
    ).toBeInTheDocument()
  })

  it('copies the remote-read URL to the clipboard', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    renderWithClient(<RemoteReadConnectionCard settings={settings} />)

    fireEvent.click(screen.getByRole('button', { name: 'Copy' }))

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith(
        `${window.location.origin}/api/v1/prometheus/read`,
      )
    })
    expect(await screen.findByText('Copied')).toBeInTheDocument()
  })
})

describe('ObservabilitySettingsCard', () => {
  it('saves a new external dashboard URL', async () => {
    const fetchMock = mockFetch({
      'PUT /api/v1/settings/observability': () =>
        jsonResponse({
          external_dashboard_url: 'https://grafana.example.com',
          remote_read_path: '/api/v1/prometheus/read',
        }),
    })

    renderWithClient(<ObservabilitySettingsCard settings={settings} />)

    fireEvent.change(screen.getByLabelText('Dashboard URL'), {
      target: { value: 'https://grafana.example.com' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      const put = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'PUT',
      )
      expect(put).toBeDefined()
      const rawBody = put?.[1]?.body
      const body = JSON.parse(typeof rawBody === 'string' ? rawBody : '{}') as {
        external_dashboard_url: string
      }
      expect(body.external_dashboard_url).toBe('https://grafana.example.com')
    })
  })

  it('rejects a non-URL value before submitting', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    renderWithClient(<ObservabilitySettingsCard settings={settings} />)

    fireEvent.change(screen.getByLabelText('Dashboard URL'), {
      target: { value: 'not-a-url' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(
      await screen.findByText('Must be an absolute http:// or https:// URL'),
    ).toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
