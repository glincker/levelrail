import { fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ObservabilityExportSection } from './ObservabilityExportSection'
import { renderWithQueryI18n } from '@/test/renderWithQuery'

const presets = {
  presets: [
    {
      id: 'datadog',
      name: 'Datadog',
      description: 'Logs and metrics to Datadog',
      docs_anchor: 'datadog',
      fields: [
        {
          key: 'site',
          label: 'Site',
          default: 'datadoghq.com',
          choices: [
            { value: 'datadoghq.com', label: 'US1' },
            { value: 'datadoghq.eu', label: 'EU' },
          ],
        },
        {
          key: 'api_key',
          label: 'API key',
          secret: true,
          required: true,
          help: 'Found under Organization Settings',
        },
      ],
      signals: [
        { signal: 'logs', default: true },
        { signal: 'metrics', default: false },
      ],
    },
  ],
}

const created = {
  id: 'exp1',
  preset: 'datadog',
  preset_name: 'Datadog',
  signals: [],
}

function json(body: unknown, status = 200): Response {
  return {
    ok: status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function mockApi(test: unknown, list: unknown[] = []) {
  const fn = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : ((input as URL).href ?? '')
    const method = init?.method ?? 'GET'
    if (url.endsWith('/presets')) return Promise.resolve(json(presets))
    if (url.endsWith('/test')) return Promise.resolve(json(test))
    if (method === 'POST') return Promise.resolve(json(created, 201))
    if (method === 'DELETE') return Promise.resolve(json(null, 204))
    return Promise.resolve(json(list))
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

afterEach(() => {
  vi.unstubAllGlobals()
})

async function openDatadog() {
  fireEvent.click(await screen.findByRole('button', { name: /Datadog/ }))
  return screen.findByLabelText(/^API key/)
}

describe('ObservabilityExportSection', () => {
  it('renders dialog fields from the catalog with defaults and a masked secret', async () => {
    mockApi({})
    renderWithQueryI18n(<ObservabilityExportSection />)
    const key = await openDatadog()
    expect(key).toHaveAttribute('type', 'password')
    expect(screen.getByLabelText('Site')).toHaveValue('datadoghq.com')
    expect(screen.getByRole('switch', { name: 'Logs' })).toBeChecked()
    expect(screen.getByRole('switch', { name: 'Metrics' })).not.toBeChecked()
    expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled()
  })

  it('shows a root-required state when the exports list is forbidden', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = typeof input === 'string' ? input : (input as URL).href
        if (url.endsWith('/presets')) return Promise.resolve(json(presets))
        return Promise.resolve(json({ error: 'forbidden' }, 403))
      }),
    )
    renderWithQueryI18n(<ObservabilityExportSection />)
    expect(
      await screen.findByText(/Exports need a root token/),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Datadog/ })).toBeDisabled()
  })

  it('creates the export then shows a passing test per signal', async () => {
    const fn = mockApi({
      id: 'exp1',
      ok: true,
      results: [{ signal: 'logs', ok: true }],
    })
    renderWithQueryI18n(<ObservabilityExportSection />)
    fireEvent.change(await openDatadog(), { target: { value: 'k' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))
    expect(await screen.findByText('Delivered')).toBeInTheDocument()
    const post = fn.mock.calls.find(([, i]) => i?.method === 'POST')
    expect(JSON.parse(post?.[1]?.body as string)).toMatchObject({
      preset: 'datadog',
      fields: { site: 'datadoghq.com', api_key: 'k' },
      signals: ['logs'],
    })
  })

  it('shows the hint and fix label for a failed signal', async () => {
    mockApi({
      id: 'exp1',
      ok: false,
      results: [
        {
          signal: 'logs',
          ok: false,
          hint: 'The key was rejected',
          fix: 'bad_token',
        },
      ],
    })
    renderWithQueryI18n(<ObservabilityExportSection />)
    fireEvent.change(await openDatadog(), { target: { value: 'k' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))
    expect(await screen.findByText(/The key was rejected/)).toBeInTheDocument()
    expect(screen.getByText('Replace the token')).toBeInTheDocument()
  })

  it('lists exports with status and deletes after confirm', async () => {
    const fn = mockApi({}, [
      {
        ...created,
        signals: [
          {
            signal: 'logs',
            destination_id: 'd',
            kind: 'otlp',
            endpoint: 'e',
            enabled: true,
            header_names: [],
            has_credential: true,
            last_error: 'boom',
            sent_total: 5,
            circuit_open: false,
          },
        ],
      },
    ])
    renderWithQueryI18n(<ObservabilityExportSection />)
    expect(
      await screen.findByRole('img', { name: 'Failing' }),
    ).toBeInTheDocument()
    expect(screen.getByText('5 sent')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    const buttons = await screen.findAllByRole('button', { name: 'Delete' })
    fireEvent.click(buttons[buttons.length - 1] as HTMLElement)
    await waitFor(() =>
      expect(fn.mock.calls.some(([, i]) => i?.method === 'DELETE')).toBe(true),
    )
  })
})
