import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import jobsEn from '../locales/en/jobs.json'
import { JobRunsPanel } from './JobRunsPanel'
import type { JobRun } from '../types/scheduledTasks'
import { createTestQueryClient } from '@/test/testQueryClient'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['jobs'],
  defaultNS: 'jobs',
  resources: { en: { jobs: jobsEn } },
  interpolation: { escapeValue: false },
})

function run(over: Partial<JobRun>): JobRun {
  return {
    id: 'jr_1',
    task_id: 'sct_1',
    job_name: 'nightly',
    service_name: 'demo-app',
    trigger: 'schedule',
    status: 'success',
    started_at: '2026-08-15T03:00:00Z',
    duration_ms: 1500,
    attempts: 1,
    ...over,
  }
}

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderPanel() {
  const queryClient = createTestQueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <JobRunsPanel appName="demo-app" />
    </QueryClientProvider>,
  )
}

describe('JobRunsPanel', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('lists runs with status, trigger and exit code and draws the duration trend', async () => {
    const runs = [
      run({ id: 'jr_3', status: 'failed', exit_code: 2, trigger: 'manual' }),
      run({ id: 'jr_2', started_at: '2026-08-14T03:00:00Z', duration_ms: 900 }),
      run({ id: 'jr_1', started_at: '2026-08-13T03:00:00Z' }),
    ]
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse(runs))),
    )

    renderPanel()

    expect(await screen.findByText('Failed')).toBeInTheDocument()
    expect(screen.getByText('Manual')).toBeInTheDocument()
    expect(screen.getAllByText('nightly')).toHaveLength(3)
    expect(
      screen.getByRole('img', { name: /last 3 finished runs/i }),
    ).toBeInTheDocument()
  })

  it('asks the server for the chosen status', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      void input
      return Promise.resolve(jsonResponse([]))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPanel()
    await screen.findByText(/no runs yet/i)

    await user.click(screen.getByRole('combobox', { name: /status/i }))
    await user.click(await screen.findByRole('option', { name: 'Failed' }))

    await waitFor(() => {
      const urls = fetchMock.mock.calls.map(([input]) => input as string)
      expect(urls.some((u) => u.includes('status=failed'))).toBe(true)
    })
  })
})
