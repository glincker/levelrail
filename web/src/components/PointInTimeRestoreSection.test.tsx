import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PointInTimeRestoreSection } from './PointInTimeRestoreSection'
import type { DatabaseResource } from '../types/databaseDetail'

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

function renderWithClient(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

function makeDatabase(
  overrides: Partial<DatabaseResource> = {},
): DatabaseResource {
  return {
    name: 'main',
    engine: 'postgres',
    version: '16',
    ...overrides,
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PointInTimeRestoreSection', () => {
  it('renders nothing for a non-postgres database', () => {
    const { container } = renderWithClient(
      <PointInTimeRestoreSection
        database={makeDatabase({ engine: 'redis' })}
      />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('shows an Enable button and the going-forward-only caveat when PITR is not enabled', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<(input: RequestInfo | URL) => Promise<Response>>((input) => {
        const url = requestUrlOf(input)
        if (url.endsWith('/pitr')) {
          return Promise.resolve(
            fakeJsonResponse({ enabled: false, has_base_backup: false }),
          )
        }
        throw new Error(`unexpected fetch: ${url}`)
      }),
    )

    renderWithClient(<PointInTimeRestoreSection database={makeDatabase()} />)

    expect(await screen.findByText('Point-in-time restore')).toBeInTheDocument()
    expect(
      await screen.findByRole('button', { name: 'Enable' }),
    ).toBeInTheDocument()
    expect(
      screen.getByText(/only recoverable going forward/i),
    ).toBeInTheDocument()
  })

  it('shows the recoverable window and a Disable button once PITR is enabled', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<(input: RequestInfo | URL) => Promise<Response>>((input) => {
        const url = requestUrlOf(input)
        if (url.endsWith('/pitr')) {
          return Promise.resolve(
            fakeJsonResponse({
              enabled: true,
              enabled_at: '2026-08-14T00:00:00Z',
              has_base_backup: true,
              window_start: '2026-08-14T00:00:00Z',
              window_end: '2026-08-14T06:00:00Z',
            }),
          )
        }
        if (url.endsWith('/base-backups')) {
          return Promise.resolve(fakeJsonResponse([]))
        }
        if (url.endsWith('/pitr-restores')) {
          return Promise.resolve(fakeJsonResponse([]))
        }
        if (url.includes('/backup-targets')) {
          return Promise.resolve(
            fakeJsonResponse([
              {
                id: 'bkt_1',
                name: 'primary',
                provider: 'aws',
                bucket: 'backups',
              },
            ]),
          )
        }
        throw new Error(`unexpected fetch: ${url}`)
      }),
    )

    renderWithClient(<PointInTimeRestoreSection database={makeDatabase()} />)

    expect(
      await screen.findByRole('button', { name: 'Disable' }),
    ).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByText('Base backups')).toBeInTheDocument()
    })
  })
})
