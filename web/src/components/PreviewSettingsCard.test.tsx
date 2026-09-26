import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PreviewSettingsCard } from './PreviewSettingsCard'
import type { PreviewStatus } from '../types/preview'

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <PreviewSettingsCard appName="web" />
    </QueryClientProvider>,
  )
}

describe('PreviewSettingsCard', () => {
  let status: PreviewStatus
  let puts: unknown[]
  let prunes: number

  beforeEach(() => {
    puts = []
    prunes = 0
    status = {
      app: 'web',
      enabled: true,
      mode: 'metadata',
      default_mode: 'metadata',
      path: '/',
      wait_ms: 0,
      server_enabled: true,
      capturing: false,
      image: 'docker.io/example/browser:1',
      keep_per_app: 5,
      ttl_days: 30,
      max_total_mb: 200,
      storage: {
        app_bytes: 2048,
        app_count: 2,
        total_bytes: 4096,
        total_count: 3,
      },
    }
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = urlOf(input)
        const method = init?.method ?? 'GET'
        if (url === '/api/v1/apps/web/preview' && method === 'GET') {
          return Promise.resolve(jsonResponse(status))
        }
        if (url === '/api/v1/apps/web/preview' && method === 'PUT') {
          const body = JSON.parse(
            init?.body as string,
          ) as Partial<PreviewStatus>
          puts.push(body)
          status = { ...status, ...body }
          return Promise.resolve(jsonResponse(status))
        }
        if (url === '/api/v1/apps/web/preview/prune' && method === 'POST') {
          prunes += 1
          return Promise.resolve(
            jsonResponse({ removed: 2, freed_bytes: 2048 }),
          )
        }
        return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('loads in the default metadata mode and shows storage and retention', async () => {
    renderCard()
    const group = await screen.findByRole('radiogroup', {
      name: 'Deploy preview mode',
    })
    expect(group).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: /^Metadata/ })).toBeChecked()
    expect(screen.getByRole('radio', { name: /^Off/ })).not.toBeChecked()
    expect(screen.getByRole('radio', { name: /^Screenshot/ })).not.toBeChecked()
    expect(screen.getByText('(default)')).toBeInTheDocument()
    expect(screen.queryByText('docker.io/example/browser:1')).toBeNull()
    expect(document.getElementById('preview-wait')).toBeNull()
    expect(screen.getByText(/2\.0 KiB in 2\s+previews/)).toBeInTheDocument()
    expect(screen.getByText('Last 5 plus live, 30 days')).toBeInTheDocument()
  })

  it('states the cost of each mode honestly', async () => {
    renderCard()
    await screen.findByRole('radiogroup')
    expect(
      screen.getByText('No browser. Costs about nothing.'),
    ).toBeInTheDocument()
    expect(
      screen.getByText('Runs a browser for about 10 s per deploy.'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'About Screenshot previews' }),
    ).toBeInTheDocument()
  })

  it('switches to screenshot mode with a PUT and reveals the browser options', async () => {
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('radio', { name: /^Screenshot/ }))
    await waitFor(() => expect(puts).toEqual([{ mode: 'screenshot' }]))
    await waitFor(() =>
      expect(document.getElementById('preview-wait')).not.toBeNull(),
    )
    expect(screen.getByText('docker.io/example/browser:1')).toBeInTheDocument()
  })

  it('turns previews off with a PUT', async () => {
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('radio', { name: /^Off/ }))
    await waitFor(() => expect(puts).toEqual([{ mode: 'off' }]))
    expect(
      await screen.findByRole('button', { name: /Recapture now/ }),
    ).toBeDisabled()
  })

  it('saves a new capture path', async () => {
    const user = userEvent.setup()
    renderCard()
    const input = await screen.findByPlaceholderText('/')
    await user.clear(input)
    await user.type(input, '/pricing')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(puts).toEqual([{ path: '/pricing', wait_ms: 0 }]),
    )
  })

  it('prunes on demand', async () => {
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('button', { name: /Prune now/ }))
    await waitFor(() => expect(prunes).toBe(1))
  })

  it('tells the operator when the server has previews switched off', async () => {
    status = { ...status, server_enabled: false }
    renderCard()
    expect(
      await screen.findByText(/APP_PREVIEW_ENABLED=true/),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Recapture now/ })).toBeDisabled()
  })
})
