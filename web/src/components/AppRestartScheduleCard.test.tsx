import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NewSurfacesProviders } from '../test/NewSurfacesProviders'
import { jsonResponse } from '../test/newSurfacesI18n'
import { AppRestartScheduleCard } from './AppRestartScheduleCard'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AppRestartScheduleCard', () => {
  it('shows a skipped run and saves a new schedule', async () => {
    const body = {
      app: 'web',
      schedule: {
        schedule: '0 3 * * *',
        timezone: 'UTC',
        next_run_at: '2026-12-01T03:00:00Z',
        last_run_at: '2026-11-30T03:00:00Z',
        last_status: 'skipped',
        last_reason: 'a deploy freeze is active',
        failures: 0,
      },
      min_interval: '1h0m0s',
    }
    const fetchMock = vi.fn<
      (url: string, init?: RequestInit) => Promise<Response>
    >(() => Promise.resolve(jsonResponse(body)))
    vi.stubGlobal('fetch', fetchMock)
    render(
      <NewSurfacesProviders>
        <AppRestartScheduleCard appName="web" />
      </NewSurfacesProviders>,
    )
    expect(
      await screen.findByText(/a deploy freeze is active/),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Clear schedule' })).toBeEnabled()
    fireEvent.change(screen.getByLabelText('Cron or @every'), {
      target: { value: '@every 24h' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save schedule' }))
    await waitFor(() => {
      const put = fetchMock.mock.calls.find((c) => c[1]?.method === 'PUT')
      expect(put).toBeDefined()
      expect(put?.[1]?.body).toContain('@every 24h')
    })
  })
})
