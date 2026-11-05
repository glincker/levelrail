import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import jobsEn from '../locales/en/jobs.json'
import { RunScheduledTaskButton } from './RunScheduledTaskButton'
import { scheduledTaskKeys } from '../queries/scheduledTasks'
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

const task = {
  id: 'sct_1',
  command: ['true'],
  schedule: '* * * * *',
  enabled: true,
} as ScheduledTask

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('RunScheduledTaskButton', () => {
  it('posts the run and invalidates the app job queries', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 202,
      json: () => Promise.resolve({ ...task, run_id: 'jr_9' }),
    })
    vi.stubGlobal('fetch', fetchMock)
    const client = createTestQueryClient()
    const spy = vi.spyOn(client, 'invalidateQueries')
    render(
      <QueryClientProvider client={client}>
        <RunScheduledTaskButton appName="demo" task={task} />
      </QueryClientProvider>,
    )
    await userEvent.click(screen.getByRole('button', { name: 'Run now' }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/apps/demo/jobs/sct_1/run',
        { method: 'POST' },
      )
    })
    await waitFor(() => {
      expect(spy).toHaveBeenCalledWith({
        queryKey: scheduledTaskKeys.all('demo'),
      })
    })
  })

  it('is disabled while a run is in progress', () => {
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <RunScheduledTaskButton
          appName="demo"
          task={{ ...task, running_since: '2026-11-02T10:00:00Z' }}
        />
      </QueryClientProvider>,
    )
    expect(screen.getByRole('button', { name: 'Running...' })).toBeDisabled()
  })
})
