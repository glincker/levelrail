import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PlatformImportCard } from './PlatformImportCard'
import type { PlatformImportReport } from '../queries/platformImport'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    params,
  }: {
    children: ReactNode
    params?: { name: string }
  }) => <a href={`/x/${params?.name ?? ''}`}>{children}</a>,
}))

const discoverReport: PlatformImportReport = {
  platform: 'coolify',
  counts: { mapped: 1, 'needs-attention': 1, unsupported: 1 },
  notes: ['Databases are created empty.'],
  items: [
    {
      kind: 'app',
      source_id: 'a1',
      source_name: 'web',
      target: 'web',
      status: 'mapped',
    },
    {
      kind: 'database',
      source_id: 'd1',
      source_name: 'main-db',
      target: 'main-db',
      status: 'needs-attention',
      reasons: ['created empty, data is not migrated'],
      manual: ['dump and restore'],
    },
    {
      kind: 'compose',
      source_id: 'c1',
      source_name: 'stack',
      status: 'unsupported',
      reasons: ['multi-service'],
    },
  ],
}

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <PlatformImportCard />
    </QueryClientProvider>,
  )
}

function fillConnect() {
  fireEvent.change(screen.getByLabelText('Source URL'), {
    target: { value: 'https://coolify.example.com' },
  })
  fireEvent.change(screen.getByLabelText('API token'), {
    target: { value: 'tok-fixture' },
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PlatformImportCard', () => {
  it('uses a password field for the token', () => {
    renderCard()
    expect(screen.getByLabelText('API token')).toHaveAttribute(
      'type',
      'password',
    )
  })

  it('discovers, lets the operator deselect, applies only the selection, and clears the token', async () => {
    const calls: { url: string; body: Record<string, unknown> }[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init: RequestInit) => {
        calls.push({
          url,
          body: JSON.parse(init.body as string) as Record<string, unknown>,
        })
        if (url.endsWith('/discover'))
          return Promise.resolve(jsonResponse(discoverReport))
        return Promise.resolve(
          jsonResponse({
            ...discoverReport,
            counts: { created: 1 },
            items: [{ ...discoverReport.items[0], status: 'created' }],
          }),
        )
      }),
    )
    renderCard()
    fillConnect()
    fireEvent.click(screen.getByRole('button', { name: 'Discover' }))

    expect(
      await screen.findByText('Review what will be imported'),
    ).toBeInTheDocument()
    expect(
      screen.getByText('created empty, data is not migrated'),
    ).toBeInTheDocument()
    expect(screen.getByText('Databases are created empty.')).toBeInTheDocument()
    expect(screen.getByLabelText('Import stack')).toHaveAttribute(
      'aria-disabled',
      'true',
    )
    expect(
      screen.getByRole('button', { name: 'Import 2 selected' }),
    ).toBeEnabled()

    fireEvent.click(screen.getByLabelText('Import main-db'))
    fireEvent.click(screen.getByRole('button', { name: 'Import 1 selected' }))

    expect(await screen.findByText('Import summary')).toBeInTheDocument()
    expect(calls[0]?.body.token).toBe('tok-fixture')
    expect(calls[1]?.url).toBe('/api/v1/imports/platform/apply')
    expect(calls[1]?.body.only).toEqual(['a1'])
    expect(screen.getByRole('link', { name: 'Open web' })).toBeInTheDocument()

    fireEvent.click(
      screen.getByRole('button', { name: 'Start another import' }),
    )
    expect(screen.getByLabelText('API token')).toHaveValue('')
  })

  it('shows the server error when discovery fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse(
            { error: 'reading the source platform: unauthorized' },
            502,
          ),
        ),
      ),
    )
    renderCard()
    fillConnect()
    fireEvent.click(screen.getByRole('button', { name: 'Discover' }))
    await waitFor(() =>
      expect(screen.getByText('Could not read the source')).toBeInTheDocument(),
    )
  })

  it('shows an empty state when the source has nothing', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse({ platform: 'coolify', items: [], counts: {} }),
        ),
      ),
    )
    renderCard()
    fillConnect()
    fireEvent.click(screen.getByRole('button', { name: 'Discover' }))
    expect(
      await screen.findByText(/Nothing was found on the source/),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Import 0 selected' }),
    ).toBeDisabled()
  })
})
