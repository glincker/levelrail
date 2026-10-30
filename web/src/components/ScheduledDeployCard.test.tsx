import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import i18next from 'i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import scheduledDeployEn from '../locales/en/scheduledDeploy.json'
import { ScheduledDeployCard } from './ScheduledDeployCard'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['scheduledDeploy'],
  defaultNS: 'scheduledDeploy',
  resources: { en: { scheduledDeploy: scheduledDeployEn } },
  interpolation: { escapeValue: false },
})

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
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
    </I18nextProvider>,
  )
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

  describe('one-time mode', () => {
    interface Stub {
      schedule?: Record<string, unknown> | null
      history?: unknown[]
      putStatus?: number
      putError?: string
    }
    let sent: Record<string, unknown>[]

    function stub(opts: Stub) {
      sent = []
      vi.stubGlobal(
        'fetch',
        vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
          const url = urlOf(input)
          const method = init?.method ?? 'GET'
          if (url === '/api/v1/apps/web/schedule' && method === 'GET') {
            return Promise.resolve(
              opts.schedule
                ? jsonResponse(opts.schedule)
                : jsonResponse({ error: 'not found' }, 404),
            )
          }
          if (url === '/api/v1/apps/web/schedule' && method === 'PUT') {
            const body = JSON.parse(init?.body as string) as Record<
              string,
              unknown
            >
            sent.push(body)
            if (opts.putStatus && opts.putStatus >= 400) {
              return Promise.resolve(
                jsonResponse({ error: opts.putError }, opts.putStatus),
              )
            }
            return Promise.resolve(
              jsonResponse({ service_name: 'web', ...body }),
            )
          }
          if (url === '/api/v1/apps/web/schedule/history?limit=10') {
            return Promise.resolve(jsonResponse(opts.history ?? []))
          }
          return Promise.resolve(jsonResponse({ error: 'unexpected' }, 500))
        }),
      )
    }

    // Local-time clock so the expected strings do not depend on the host zone.
    function freezeNow() {
      vi.useFakeTimers({ toFake: ['Date'] })
      vi.setSystemTime(new Date(2026, 0, 5, 10, 20))
    }

    afterEach(() => {
      vi.useRealTimers()
    })

    it('prefills the next top of the hour and chips only fill the field', async () => {
      freezeNow()
      stub({ schedule: null })
      const user = userEvent.setup()
      renderWithClient(<ScheduledDeployCard appName="web" />)

      await user.click(await screen.findByRole('tab', { name: 'One time' }))
      const field = screen.getByLabelText('Run at')
      expect(field).toHaveValue('2026-01-05T12:00')

      await user.click(screen.getByRole('button', { name: 'In 1 hour' }))
      expect(field).toHaveValue('2026-01-05T11:20')
      await user.click(screen.getByRole('button', { name: 'Tonight 03:00' }))
      expect(field).toHaveValue('2026-01-06T03:00')
      await user.click(screen.getByRole('button', { name: 'Tomorrow 09:00' }))
      expect(field).toHaveValue('2026-01-06T09:00')
      expect(sent).toEqual([])
    })

    it('sends run_at and never cron, as RFC 3339', async () => {
      freezeNow()
      stub({ schedule: null })
      const user = userEvent.setup()
      renderWithClient(<ScheduledDeployCard appName="web" />)

      await user.click(await screen.findByRole('tab', { name: 'One time' }))
      await user.click(screen.getByRole('button', { name: 'Save' }))

      await waitFor(() => expect(sent).toHaveLength(1))
      expect(sent[0]).toEqual({
        run_at: new Date(2026, 0, 5, 12, 0).toISOString(),
        branch: 'main',
        enabled: true,
      })
      expect(sent[0]).not.toHaveProperty('cron')
    })

    it('sends cron only for recurring', async () => {
      stub({ schedule: null })
      const user = userEvent.setup()
      renderWithClient(<ScheduledDeployCard appName="web" />)

      await user.click(await screen.findByRole('button', { name: 'Save' }))
      await waitFor(() => expect(sent).toHaveLength(1))
      expect(sent[0]).not.toHaveProperty('run_at')
      expect(sent[0]?.cron).toBe('0 3 * * *')
    })

    it('opens on One time for a saved run_at and shows the pending row', async () => {
      freezeNow()
      stub({
        schedule: {
          service_name: 'web',
          cron: '',
          branch: 'main',
          timezone: 'UTC',
          enabled: true,
          run_at: new Date(2026, 0, 5, 12, 20).toISOString(),
        },
      })
      renderWithClient(<ScheduledDeployCard appName="web" />)

      expect(await screen.findByLabelText('Run at')).toHaveValue(
        '2026-01-05T12:20',
      )
      expect(screen.getByRole('tab', { name: 'One time' })).toHaveAttribute(
        'aria-selected',
        'true',
      )
      expect(
        screen.getByText(/Runs once at .* \(in 2 hours\)/),
      ).toBeInTheDocument()
    })

    it('shows the server validation error inline on the field', async () => {
      freezeNow()
      stub({
        schedule: null,
        putStatus: 400,
        putError: 'run_at must be at least 5m in the future',
      })
      const user = userEvent.setup()
      renderWithClient(<ScheduledDeployCard appName="web" />)

      await user.click(await screen.findByRole('tab', { name: 'One time' }))
      await user.click(screen.getByRole('button', { name: 'Save' }))

      const alert = await screen.findByRole('alert')
      expect(alert).toHaveTextContent('run_at must be at least 5m')
      const field = screen.getByLabelText('Run at')
      expect(field).toHaveAttribute('aria-invalid', 'true')
      expect(field).toHaveAttribute('aria-describedby', alert.id)
    })

    it('shows completed state and Schedule again refills the field', async () => {
      freezeNow()
      stub({
        schedule: {
          service_name: 'web',
          cron: '',
          branch: 'main',
          timezone: 'UTC',
          enabled: true,
          run_at: '2026-01-04T03:00:00Z',
          completed_at: '2026-01-04T03:00:05Z',
        },
      })
      const user = userEvent.setup()
      renderWithClient(<ScheduledDeployCard appName="web" />)

      expect(await screen.findByText(/^Completed /)).toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: 'Schedule again' }))
      const field = screen.getByLabelText('Run at')
      expect(field).toHaveValue('2026-01-05T12:00')
      expect(field).toHaveFocus()
    })

    it('labels skipped history entries with readable tips', async () => {
      stub({
        schedule: {
          service_name: 'web',
          cron: '0 3 * * *',
          branch: 'main',
          timezone: 'UTC',
          enabled: true,
        },
        history: [
          {
            id: 'a',
            scheduled_for: '2026-01-01T03:00:00Z',
            fired_at: '2026-01-01T03:00:01Z',
            status: 'skipped_missed',
          },
          {
            id: 'b',
            scheduled_for: '2026-01-02T03:00:00Z',
            fired_at: '2026-01-02T03:00:01Z',
            status: 'skipped_freeze',
          },
        ],
      })
      renderWithClient(<ScheduledDeployCard appName="web" />)

      expect(
        await screen.findByText(
          /Skipped: the server was down past the catch-up window/,
        ),
      ).toBeInTheDocument()
      expect(
        screen.getByText(/Skipped: deploys were frozen at that time/),
      ).toBeInTheDocument()
    })
  })
})
