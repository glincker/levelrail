import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ScheduledDeployCard } from './ScheduledDeployCard'

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

function renderWithClient(node: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(<QueryClientProvider client={queryClient}>{node}</QueryClientProvider>)
}

describe('ScheduledDeployCard', () => {
  let puts: unknown[]

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('offers defaults when no schedule is configured yet', async () => {
    puts = []
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = urlOf(input)
        if (
          url === '/api/v1/apps/web/schedule' &&
          (init?.method ?? 'GET') === 'GET'
        ) {
          return Promise.resolve(jsonResponse({ error: 'not found' }, 404))
        }
        return Promise.resolve(jsonResponse({ error: 'unexpected' }, 500))
      }),
    )

    renderWithClient(<ScheduledDeployCard appName="web" />)

    expect(await screen.findByText('Scheduled deploys')).toBeInTheDocument()
    expect(screen.getByDisplayValue('0 3 * * *')).toBeInTheDocument()
    expect(
      screen.getByText(/save a schedule to see its next run/),
    ).toBeInTheDocument()
  })

  it('shows the configured schedule, its next run, and history', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = urlOf(input)
        const method = init?.method ?? 'GET'
        if (url === '/api/v1/apps/web/schedule' && method === 'GET') {
          return Promise.resolve(
            jsonResponse({
              service_name: 'web',
              cron: '0 3 * * *',
              branch: 'main',
              timezone: 'UTC',
              enabled: true,
              next_run_at: '2026-01-02T03:00:00Z',
            }),
          )
        }
        if (url === '/api/v1/apps/web/schedule/history?limit=10') {
          return Promise.resolve(
            jsonResponse([
              {
                id: 'ash_1',
                scheduled_for: '2026-01-01T03:00:00Z',
                fired_at: '2026-01-01T03:00:01Z',
                status: 'fired',
                reason: 'deploy triggered: abc123',
              },
            ]),
          )
        }
        return Promise.resolve(jsonResponse({ error: 'unexpected' }, 500))
      }),
    )

    renderWithClient(<ScheduledDeployCard appName="web" />)

    expect(await screen.findByDisplayValue('main')).toBeInTheDocument()
    expect(
      screen.getByRole('switch', { name: 'Schedule enabled' }),
    ).toBeChecked()
    expect(await screen.findByText('Fired')).toBeInTheDocument()
  })

  it('saves the schedule', async () => {
    puts = []
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = urlOf(input)
        const method = init?.method ?? 'GET'
        if (url === '/api/v1/apps/web/schedule' && method === 'GET') {
          return Promise.resolve(jsonResponse({ error: 'not found' }, 404))
        }
        if (url === '/api/v1/apps/web/schedule' && method === 'PUT') {
          const body = JSON.parse(init?.body as string) as Record<
            string,
            unknown
          >
          puts.push(body)
          return Promise.resolve(jsonResponse({ service_name: 'web', ...body }))
        }
        return Promise.resolve(jsonResponse({ error: 'unexpected' }, 500))
      }),
    )

    const user = userEvent.setup()
    renderWithClient(<ScheduledDeployCard appName="web" />)

    await screen.findByText('Scheduled deploys')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(puts).toEqual([
        { cron: '0 3 * * *', branch: 'main', timezone: 'UTC', enabled: true },
      ])
    })
  })
})
