import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BuildCacheCard } from './BuildCacheCard'

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

function renderCard(app: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <BuildCacheCard appName={app} />
    </QueryClientProvider>,
  )
}

const destination = {
  id: 'bkt_1',
  name: 'cache-bucket',
  preset: 'r2',
  provider: 'r2',
  bucket: 'cache',
  path_style: true,
  created_at: '2026-09-24T00:00:00Z',
  archive_policies: 0,
}

const setting = {
  app_name: 'web',
  target_id: 'bkt_1',
  enabled: true,
  mode: 'max',
  key_prefix: 'build-cache/web/',
  last_build_at: '2026-09-24T01:00:00Z',
  last_warning: 'build cache unavailable, continuing without it',
  updated_at: '2026-09-24T00:00:00Z',
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('BuildCacheCard', () => {
  it('explains that a destination is needed first', async () => {
    mockFetch({
      'GET /api/v1/storage/destinations': () => jsonResponse([]),
      'GET /api/v1/build-cache': () => jsonResponse([]),
    })
    renderCard('web')
    expect(
      await screen.findByText('No storage destination yet'),
    ).toBeInTheDocument()
  })

  it('reports that build cache is not configured on a 501', async () => {
    mockFetch({
      'GET /api/v1/storage/destinations': () =>
        jsonResponse({ error: 'not configured' }, 501),
      'GET /api/v1/build-cache': () =>
        jsonResponse({ error: 'not configured' }, 501),
    })
    renderCard('web')
    expect(
      await screen.findByText('Build cache is not configured on this server'),
    ).toBeInTheDocument()
  })

  it('saves a setting for the app', async () => {
    const fetchMock = mockFetch({
      'GET /api/v1/storage/destinations': () => jsonResponse([destination]),
      'GET /api/v1/build-cache': () => jsonResponse([]),
      'PUT /api/v1/build-cache': () => jsonResponse(setting),
    })
    renderCard('web')
    fireEvent.click(await screen.findByRole('button', { name: 'Save' }))
    await waitFor(() => {
      const put = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'PUT',
      )
      const rawBody = put?.[1]?.body
      const body = JSON.parse(typeof rawBody === 'string' ? rawBody : '{}') as {
        app_name: string
        target_id: string
        mode: string
        enabled: boolean
      }
      expect(body).toEqual({
        app_name: 'web',
        target_id: 'bkt_1',
        mode: 'max',
        enabled: true,
      })
    })
  })

  it('shows the last warning and usage, and clears after confirming', async () => {
    const fetchMock = mockFetch({
      'GET /api/v1/storage/destinations': () => jsonResponse([destination]),
      'GET /api/v1/build-cache/stats': () =>
        jsonResponse({
          prefix: 'build-cache/web/',
          objects: 12,
          bytes: 2048,
          last_modified: '2026-09-24T00:30:00Z',
          truncated: false,
        }),
      'GET /api/v1/build-cache': () => jsonResponse([setting]),
      'POST /api/v1/build-cache/clear': () =>
        jsonResponse({ deleted: 12, more: false }),
    })
    renderCard('web')
    expect(
      await screen.findByText('Last build ran without the cache'),
    ).toBeInTheDocument()
    expect(await screen.findByText(/12 objects, 2.0 KiB/)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Clear cache' }))
    expect(
      fetchMock.mock.calls.some(([, init]) => init?.method === 'POST'),
    ).toBe(false)
    fireEvent.click(
      screen.getByRole('button', { name: 'Delete cached layers' }),
    )
    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(([, init]) => init?.method === 'POST'),
      ).toBe(true)
    })
  })
})
