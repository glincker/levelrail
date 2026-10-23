import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import jobsEn from '../locales/en/jobs.json'
import { CronPreview } from './CronPreview'
import { JobDialog } from './JobDialog'
import type { ScheduledTask } from '../types/scheduledTasks'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['jobs'],
  defaultNS: 'jobs',
  resources: { en: { jobs: jobsEn } },
  interpolation: { escapeValue: false },
})

const NOW = Date.UTC(2026, 2, 4, 10, 20)

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

const HTTP_TASK: ScheduledTask = {
  id: 'task_1',
  service_name: 'demo-app',
  name: 'digest',
  mode: 'http',
  command: [],
  http: { method: 'POST', path: '/cron/digest', background: true },
  schedule: '@hourly',
  enabled: true,
  concurrency_policy: 'queue',
  consecutive_failures: 0,
  created_at: '2026-01-01T00:00:00.000000000Z',
  updated_at: '2026-01-01T00:00:00.000000000Z',
}

function renderDialog(task?: ScheduledTask) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <JobDialog appName="demo-app" task={task} />
    </QueryClientProvider>,
  )
}

describe('CronPreview', () => {
  it('lists five upcoming runs for a valid schedule', () => {
    render(<CronPreview cron="@hourly" timezone="UTC" now={NOW} />)
    expect(screen.getAllByRole('listitem')).toHaveLength(5)
  })

  it('explains an invalid schedule instead of listing runs', () => {
    render(<CronPreview cron="nope" timezone="UTC" now={NOW} />)
    expect(screen.getByText(/valid schedule and time zone/i)).toBeVisible()
    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
  })

  it('explains an unknown time zone', () => {
    render(<CronPreview cron="@daily" timezone="Mars/Base" now={NOW} />)
    expect(screen.getByText(/valid schedule and time zone/i)).toBeVisible()
  })
})

describe('JobDialog http mode', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('submits an http job without a command', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((input) => {
      const url = requestUrlOf(input)
      if (url === '/api/v1/apps/demo-app/jobs') {
        return Promise.resolve(fakeJsonResponse(HTTP_TASK, 201))
      }
      throw new Error(`unexpected fetch: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderDialog()
    await user.click(screen.getByRole('button', { name: /create job/i }))
    await user.click(screen.getByRole('combobox', { name: /where it runs/i }))
    await user.click(
      await screen.findByRole('option', { name: /http call to the app/i }),
    )
    await user.type(screen.getByLabelText('Path'), '/cron/digest')
    await user.click(screen.getByRole('switch', { name: /background job/i }))
    await user.click(screen.getByRole('button', { name: /^create job$/i }))

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(
        (c) => requestUrlOf(c[0]) === '/api/v1/apps/demo-app/jobs',
      )
      expect(call).toBeDefined()
      const body = JSON.parse((call?.[1] as RequestInit).body as string) as {
        mode: string
        command: string[]
        http: { method: string; path: string; background: boolean }
      }
      expect(body.mode).toBe('http')
      expect(body.command).toEqual([])
      expect(body.http).toMatchObject({
        method: 'POST',
        path: '/cron/digest',
        background: true,
      })
    })
  })

  it('rejects a path that is not absolute', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    renderDialog()
    await user.click(screen.getByRole('button', { name: /create job/i }))
    await user.click(screen.getByRole('combobox', { name: /where it runs/i }))
    await user.click(
      await screen.findByRole('option', { name: /http call to the app/i }),
    )
    await user.type(screen.getByLabelText('Path'), 'cron/digest')
    await user.click(screen.getByRole('button', { name: /^create job$/i }))

    expect(await screen.findByText(/start with a single/i)).toBeVisible()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('shows the stored request when editing an http job', async () => {
    renderDialog(HTTP_TASK)
    await userEvent.setup().click(screen.getByRole('button', { name: /edit/i }))

    expect(screen.getByLabelText('Path')).toHaveValue('/cron/digest')
    expect(
      screen.getByRole('switch', { name: /background job/i }),
    ).toBeChecked()
  })
})
