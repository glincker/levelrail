import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LogArchivePanel } from './LogArchivePanel'
import { StorageProviderPicker } from './StorageProviderPicker'
import type { StorageProvider } from '../types/storage'

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

function renderPanel(app: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <LogArchivePanel appName={app} />
    </QueryClientProvider>,
  )
}

const destination = {
  id: 'bkt_1',
  name: 'archive-bucket',
  preset: 'r2',
  provider: 'r2',
  bucket: 'logs',
  path_style: true,
  created_at: '2026-09-24T00:00:00Z',
  archive_policies: 0,
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('LogArchivePanel', () => {
  it('explains that a destination is needed first', async () => {
    mockFetch({
      'GET /api/v1/storage/destinations': () => jsonResponse([]),
      'GET /api/v1/log-archive/policies': () => jsonResponse([]),
    })
    renderPanel('web')
    expect(
      await screen.findByText('No storage destination yet'),
    ).toBeInTheDocument()
  })

  it('reports that storage is not configured on a 501', async () => {
    mockFetch({
      'GET /api/v1/storage/destinations': () =>
        jsonResponse({ error: 'not configured' }, 501),
      'GET /api/v1/log-archive/policies': () =>
        jsonResponse({ error: 'not configured' }, 501),
    })
    renderPanel('web')
    expect(
      await screen.findByText('Log archive is not configured on this server'),
    ).toBeInTheDocument()
  })

  it('saves a policy for the app and lists archived objects', async () => {
    const fetchMock = mockFetch({
      'GET /api/v1/storage/destinations': () => jsonResponse([destination]),
      'GET /api/v1/log-archive/policies': () => jsonResponse([]),
      'GET /api/v1/log-archive/runs': () => jsonResponse([]),
      'GET /api/v1/log-archive/objects': () =>
        jsonResponse({
          objects: [
            {
              key: 'log-archive/service/web/2026/09/24/00/1.ndjson.gz',
              size: 2048,
              last_modified: '2026-09-24T00:10:00Z',
            },
          ],
        }),
      'PUT /api/v1/log-archive/policy': () =>
        jsonResponse({ id: 'lap_1', app_name: 'web', target_id: 'bkt_1' }),
    })
    renderPanel('web')

    fireEvent.click(
      await screen.findByRole('button', { name: 'Start archiving' }),
    )
    await waitFor(() => {
      const put = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'PUT',
      )
      expect(put).toBeDefined()
      const rawBody = put?.[1]?.body
      const body = JSON.parse(typeof rawBody === 'string' ? rawBody : '{}') as {
        app_name: string
        target_id: string
        interval: string
      }
      expect(body).toMatchObject({
        app_name: 'web',
        target_id: 'bkt_1',
        interval: '1h',
      })
    })
    expect(await screen.findByText(/1\.ndjson\.gz/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Download/ })).toHaveAttribute(
      'href',
      expect.stringContaining('/api/v1/log-archive/objects/download'),
    )
  })
})

describe('StorageProviderPicker', () => {
  const providers: StorageProvider[] = (
    ['aws', 'r2', 'b2', 'minio', 'wasabi', 'custom'] as const
  ).map((id) => ({
    id,
    label: id,
    path_style: true,
    needs_account_id: id === 'r2',
    needs_region: id === 'aws',
    needs_endpoint: id === 'custom',
  }))

  it('marks the selected preset and reports changes', () => {
    const onChange = vi.fn()
    render(
      <StorageProviderPicker
        providers={providers}
        value="r2"
        onChange={onChange}
      />,
    )
    expect(screen.getAllByRole('radio')).toHaveLength(6)
    expect(
      screen.getByRole('radio', { name: /Cloudflare R2/ }),
    ).toHaveAttribute('aria-checked', 'true')
    fireEvent.click(screen.getByRole('radio', { name: /Backblaze B2/ }))
    expect(onChange).toHaveBeenCalledWith('b2')
  })
})
