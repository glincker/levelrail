import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import { TrafficExportCard } from './TrafficExportCard'
import { createTestQueryClient } from '@/test/testQueryClient'

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function rowAt(rows: HTMLElement[], i: number): HTMLElement {
  const row = rows[i]
  if (!row) throw new Error(`no row at ${String(i)}`)
  return row
}

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

const sinks = [
  {
    id: 'tex_1',
    name: 'Product analytics',
    kind: 'posthog',
    enabled: true,
    target_host: 'eu.i.posthog.com',
    fields: [],
    ip_truncate: true,
    sample: 1,
    token_set: true,
    identity_enabled: false,
  },
  {
    id: 'tex_2',
    name: 'Warehouse',
    kind: 'webhook',
    enabled: true,
    target_host: 'hooks.example.com',
    fields: ['client_ip'],
    ip_truncate: true,
    sample: 1,
    url_set: true,
    identity_enabled: true,
  },
]

const status = {
  sinks: [
    {
      id: 'tex_1',
      name: 'Product analytics',
      kind: 'posthog',
      enabled: true,
      active: true,
      last_success: '2026-10-22T10:00:00Z',
      queued: 3,
      sent: 10,
      dropped: 7,
      failed: 0,
      sampled_out: 0,
    },
    {
      id: 'tex_2',
      name: 'Warehouse',
      kind: 'webhook',
      enabled: true,
      active: true,
      last_success: '0001-01-01T00:00:00Z',
      last_error_kind: 'auth',
      queued: 0,
      sent: 0,
      dropped: 0,
      failed: 4,
      sampled_out: 0,
    },
  ],
}

function stubApi(calls: Array<{ method: string; url: string; body?: string }>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = urlOf(input)
      calls.push({
        method: init?.method ?? 'GET',
        url,
        body: init?.body as string,
      })
      if (url.endsWith('/status')) return Promise.resolve(json(status))
      if (url.endsWith('/test'))
        return Promise.resolve(json({ ok: true, duration_ms: 3 }))
      if (url.endsWith('/sinks') && init?.method === 'POST') {
        return Promise.resolve(json({ ...sinks[0], id: 'tex_3' }, 201))
      }
      if (url.endsWith('/sinks')) return Promise.resolve(json({ sinks }))
      return Promise.resolve(json({}, 404))
    }),
  )
}

function renderCard() {
  const client = createTestQueryClient()
  return render(
    <AllI18n>
      <QueryClientProvider client={client}>
        <TrafficExportCard />
      </QueryClientProvider>
    </AllI18n>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('TrafficExportCard', () => {
  it('shows one compact row per destination with status chips and no secrets', async () => {
    stubApi([])
    const { container } = renderCard()
    const rows = await screen.findAllByRole('listitem')
    expect(rows).toHaveLength(2)

    const first = within(rowAt(rows, 0))
    expect(first.getByText('Product analytics')).toBeInTheDocument()
    expect(first.getByText('Active')).toBeInTheDocument()
    expect(first.getByText('3 queued')).toBeInTheDocument()
    expect(first.getByText('7 dropped')).toBeInTheDocument()
    expect(first.getByText('No visitor identity')).toBeInTheDocument()

    const second = within(rowAt(rows, 1))
    expect(second.getByText('Failing')).toBeInTheDocument()
    expect(second.getByText('Credentials rejected')).toBeInTheDocument()
    expect(second.getByText('Identity fields on')).toBeInTheDocument()
    expect(second.getByText('No delivery yet')).toBeInTheDocument()

    expect(container.firstElementChild).toHaveClass(
      'shadow-card',
      'ring-1',
      'rounded-xl',
    )
    expect(screen.getByText(/never exported/)).toBeInTheDocument()
  })

  it('runs a test delivery from the row', async () => {
    const calls: Array<{ method: string; url: string }> = []
    stubApi(calls)
    renderCard()
    const rows = await screen.findAllByRole('listitem')
    await userEvent.click(
      within(rowAt(rows, 0)).getByRole('button', { name: 'Send test' }),
    )
    await waitFor(() =>
      expect(
        calls.some(
          (c) => c.method === 'POST' && c.url.endsWith('/sinks/tex_1/test'),
        ),
      ).toBe(true),
    )
  })

  it('warns when identity fields are ticked and sends the allowlist', async () => {
    const calls: Array<{ method: string; url: string; body?: string }> = []
    stubApi(calls)
    renderCard()
    await screen.findAllByRole('listitem')
    await userEvent.click(
      screen.getByRole('button', { name: 'Add destination' }),
    )

    expect(
      screen.queryByText('This destination will receive visitor identity'),
    ).not.toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Name'), 'Mine')
    await userEvent.type(screen.getByLabelText('Project token'), 'phc_x')
    await userEvent.click(
      screen.getByRole('checkbox', { name: 'Client IP address' }),
    )
    expect(
      screen.getByText('This destination will receive visitor identity'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('switch', { name: 'Truncate client IP' }),
    ).toBeInTheDocument()

    await userEvent.click(
      screen.getByRole('button', { name: 'Save destination' }),
    )
    await waitFor(() =>
      expect(
        calls.some((c) => c.method === 'POST' && c.url.endsWith('/sinks')),
      ).toBe(true),
    )
    const post = calls.find(
      (c) => c.method === 'POST' && c.url.endsWith('/sinks'),
    )
    const body = JSON.parse(post?.body ?? '{}') as Record<string, unknown>
    expect(body).toMatchObject({
      name: 'Mine',
      kind: 'posthog',
      fields: ['client_ip'],
      ip_truncate: true,
      token: 'phc_x',
    })
  })

  it('sends no identity fields by default', async () => {
    const calls: Array<{ method: string; url: string; body?: string }> = []
    stubApi(calls)
    renderCard()
    await screen.findAllByRole('listitem')
    await userEvent.click(
      screen.getByRole('button', { name: 'Add destination' }),
    )
    await userEvent.type(screen.getByLabelText('Name'), 'Plain')
    await userEvent.type(screen.getByLabelText('Project token'), 'phc_y')
    await userEvent.click(
      screen.getByRole('button', { name: 'Save destination' }),
    )
    await waitFor(() =>
      expect(calls.some((c) => c.method === 'POST')).toBe(true),
    )
    const post = calls.find(
      (c) => c.method === 'POST' && c.url.endsWith('/sinks'),
    )
    const body = JSON.parse(post?.body ?? '{}') as { fields: string[] }
    expect(body.fields).toEqual([])
  })

  it('blocks adding when the control plane has no master key', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = urlOf(input)
        if (url.endsWith('/status'))
          return Promise.resolve(json({ error: 'no key' }, 501))
        return Promise.resolve(json({ sinks: [] }))
      }),
    )
    renderCard()
    expect(await screen.findByText(/needs a master key/)).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Add destination' }),
    ).toBeDisabled()
  })
})
