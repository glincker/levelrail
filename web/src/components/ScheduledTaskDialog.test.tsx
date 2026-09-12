import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ScheduledTaskDialog } from './ScheduledTaskDialog'
import type { ScheduledTask } from '../types/scheduledTasks'

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
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ScheduledTaskDialog appName="demo-app" task={task} />
    </QueryClientProvider>,
  )
}

const BASE_TASK: ScheduledTask = {
  id: 'task_1',
  service_name: 'demo-app',
  command: ['sh', '-c', 'echo hi'],
  schedule: '0 3 * * *',
  enabled: true,
  consecutive_failures: 0,
  created_at: '2026-01-01T00:00:00.000000000Z',
  updated_at: '2026-01-01T00:00:00.000000000Z',
}

describe('ScheduledTaskDialog cron builder', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('switching frequency to weekly changes the submitted cron value', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(
      (input) => {
        const url = requestUrlOf(input)
        if (url === '/api/v1/apps/demo-app/scheduled-tasks') {
          return Promise.resolve(fakeJsonResponse(BASE_TASK, 201))
        }
        throw new Error(`unexpected fetch: ${url}`)
      },
    )
    vi.stubGlobal('fetch', fetchMock)

    renderDialog()

    await user.click(screen.getByRole('button', { name: /create task/i }))
    await user.type(screen.getByLabelText('Command'), 'echo hi')

    await user.click(screen.getByRole('combobox', { name: /frequency/i }))
    await user.click(await screen.findByRole('option', { name: 'Weekly' }))

    await user.click(screen.getByRole('button', { name: /create task/i }))

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(
        (c) => requestUrlOf(c[0]) === '/api/v1/apps/demo-app/scheduled-tasks',
      )
      expect(call).toBeDefined()
      const body = JSON.parse((call?.[1] as RequestInit).body as string) as {
        schedule: string
      }
      // Daily would produce "0 3 * * *"; weekly at the default Sunday
      // weekday produces a "0" in the day-of-week field instead of "*",
      // proving the frequency switch actually drove the computed cron.
      expect(body.schedule).toBe('0 3 * * 0')
    })
  })

  it('falls back to Custom when editing a task whose cron does not match the daily/weekly shape', async () => {
    renderDialog({ ...BASE_TASK, schedule: '*/15 * * * *' })

    await userEvent.setup().click(screen.getByRole('button', { name: /edit/i }))

    expect(
      screen.getByRole('combobox', { name: /frequency/i }),
    ).toHaveTextContent('custom')
    expect(screen.getByLabelText('Cron expression')).toHaveValue('*/15 * * * *')
  })
})
