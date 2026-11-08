import { Suspense, lazy } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import { createTestQueryClient } from '@/test/testQueryClient'

const OtelDestinationsCard = lazy(() => import('./OtelDestinationsCard'))

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

const sink = {
  id: 'tex_1',
  name: 'Collector',
  kind: 'otlp',
  enabled: true,
  target_host: 'otel.example.com',
  fields: [],
  ip_truncate: true,
  sample: 1,
  token_set: true,
  identity_enabled: false,
}

function stub(calls: Array<{ method: string; url: string }>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.toString()
            : input.url
      const method = init?.method ?? 'GET'
      calls.push({ method, url })
      if (method === 'DELETE') return Promise.resolve(json(undefined, 204))
      if (url.endsWith('/status')) return Promise.resolve(json({ sinks: [] }))
      if (url.endsWith('/test'))
        return Promise.resolve(json({ ok: true, duration_ms: 2 }))
      if (url.endsWith('/sinks')) return Promise.resolve(json({ sinks: [sink] }))
      return Promise.resolve(json({}, 404))
    }),
  )
}

function renderCard() {
  return render(
    <AllI18n>
      <QueryClientProvider client={createTestQueryClient()}>
        <Suspense fallback={null}>
          <OtelDestinationsCard />
        </Suspense>
      </QueryClientProvider>
    </AllI18n>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('OtelDestinationsCard', () => {
  it('opens the add panel', async () => {
    stub([])
    renderCard()
    await screen.findByText('Collector')
    await userEvent.click(screen.getByRole('button', { name: /Add/ }))
    expect(await screen.findByText('Add export destination')).toBeVisible()
  })

  it('sends a test delivery', async () => {
    const calls: Array<{ method: string; url: string }> = []
    stub(calls)
    renderCard()
    const row = (await screen.findAllByRole('listitem'))[0]
    if (!row) throw new Error('no row')
    await userEvent.click(within(row).getByRole('button', { name: 'Send test' }))
    await waitFor(() =>
      expect(calls.some((c) => c.url.endsWith('/sinks/tex_1/test'))).toBe(true),
    )
  })

  it('removes a destination after confirming', async () => {
    const calls: Array<{ method: string; url: string }> = []
    stub(calls)
    renderCard()
    const row = (await screen.findAllByRole('listitem'))[0]
    if (!row) throw new Error('no row')
    await userEvent.click(within(row).getByRole('button', { name: 'Remove' }))
    await userEvent.click(
      await screen.findByRole('button', { name: 'Remove destination' }),
    )
    await waitFor(() =>
      expect(
        calls.some(
          (c) => c.method === 'DELETE' && c.url.endsWith('/sinks/tex_1'),
        ),
      ).toBe(true),
    )
  })
})
