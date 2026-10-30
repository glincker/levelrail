import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import jobsEn from '../locales/en/jobs.json'
import { JobDialog } from './JobDialog'
import type { ScheduledTask } from '../types/scheduledTasks'
import { createTestQueryClient } from '@/test/testQueryClient'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['jobs'],
  defaultNS: 'jobs',
  resources: { en: { jobs: jobsEn } },
  interpolation: { escapeValue: false },
})

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

function renderDialog(task?: ScheduledTask) {
  const queryClient = createTestQueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <JobDialog appName="demo-app" task={task} />
    </QueryClientProvider>,
  )
}

const BASE_TASK: ScheduledTask = {
  id: 'task_1',
  service_name: 'demo-app',
  name: 'nightly',
  mode: 'fresh',
  command: ['sh', '-c', 'echo hi'],
  schedule: '0 3 * * *',
  enabled: true,
  concurrency_policy: 'forbid',
  consecutive_failures: 0,
  created_at: '2026-01-01T00:00:00.000000000Z',
  updated_at: '2026-01-01T00:00:00.000000000Z',
}

describe('JobDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('submits the schedule, mode and timezone the form describes', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((input) => {
      const url = requestUrlOf(input)
      if (url === '/api/v1/apps/demo-app/jobs') {
        return Promise.resolve(fakeJsonResponse(BASE_TASK, 201))
      }
      throw new Error(`unexpected fetch: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderDialog()

    await user.click(screen.getByRole('button', { name: /create job/i }))
    await user.type(screen.getByLabelText('Name'), 'nightly')
    await user.type(screen.getByLabelText('Command'), 'echo hi')
    await user.click(screen.getByRole('combobox', { name: /frequency/i }))
    await user.click(await screen.findByRole('option', { name: 'Weekly' }))
    await user.clear(screen.getByLabelText('Time zone'))
    await user.type(screen.getByLabelText('Time zone'), 'Europe/Berlin')

    await user.click(screen.getByRole('button', { name: /^create job$/i }))

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(
        (c) => requestUrlOf(c[0]) === '/api/v1/apps/demo-app/jobs',
      )
      expect(call).toBeDefined()
      const body = JSON.parse((call?.[1] as RequestInit).body as string) as {
        schedule: string
        name: string
        mode: string
        timezone: string
        command: string[]
      }
      expect(body.schedule).toBe('0 3 * * 0')
      expect(body.name).toBe('nightly')
      expect(body.mode).toBe('fresh')
      expect(body.timezone).toBe('Europe/Berlin')
      expect(body.command).toEqual(['sh', '-c', 'echo hi'])
    })
  })

  it('falls back to Custom when editing a job whose cron is not daily or weekly', async () => {
    renderDialog({ ...BASE_TASK, schedule: '*/15 * * * *' })

    await userEvent.setup().click(screen.getByRole('button', { name: /edit/i }))

    expect(
      screen.getByRole('combobox', { name: /frequency/i }),
    ).toHaveTextContent('Custom cron expression')
    expect(screen.getByLabelText('Cron expression')).toHaveValue('*/15 * * * *')
  })

  it('disables editing for a job declared in app.yaml', () => {
    renderDialog({ ...BASE_TASK, source: 'spec' })

    expect(screen.getByRole('button', { name: /edit/i })).toBeDisabled()
  })
})
