import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { DeployAttempt } from '../types/deployAttempt'
import { AppHealthTimeline } from './AppHealthTimeline'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
}))

let attempts: DeployAttempt[] = []
let attemptsState: 'ok' | 'error' | 'pending' = 'ok'
vi.mock('../queries/deployAttempts', () => ({
  deployAttemptsQueryOptions: () => ({
    queryKey: ['attempts'],
    queryFn: async () => {
      if (attemptsState === 'error') {
        throw new Error('boom')
      }
      if (attemptsState === 'pending') {
        return new Promise<DeployAttempt[]>(() => undefined)
      }
      return attempts
    },
    retry: false,
  }),
}))
vi.mock('../queries/metrics', () => ({
  useMetricSeries: () => ({
    data: {
      metric: 'container_restart_count',
      points: [
        {
          timestamp: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
          value: 1,
          count: 1,
        },
      ],
    },
    isError: false,
  }),
}))

function renderIt() {
  const qc = new QueryClient()
  return render(
    <QueryClientProvider client={qc}>
      <AppHealthTimeline appName="web" />
    </QueryClientProvider>,
  )
}

function att(id: string, status: DeployAttempt['status']): DeployAttempt {
  return {
    id,
    service_name: 'web',
    image: 'img',
    status,
    started_at: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
  }
}

describe('AppHealthTimeline', () => {
  beforeEach(() => {
    navigate.mockClear()
    attemptsState = 'ok'
    attempts = []
  })

  it('renders focusable labeled markers and navigates on Enter', async () => {
    attempts = [att('dep-ok-1', 'succeeded'), att('dep-bad-2', 'failed')]
    renderIt()
    const ok = await screen.findByRole('link', { name: /Deploy succeeded/ })
    expect(screen.getByRole('link', { name: /Failed deploy/ })).toBeTruthy()
    expect(
      screen.getByRole('link', { name: /1 container restart/ }),
    ).toBeTruthy()
    fireEvent.focus(ok)
    expect(screen.getByTestId('timeline-caption').textContent).toMatch(
      /Deploy succeeded/,
    )
    fireEvent.keyDown(ok, { key: 'Enter' })
    expect(navigate).toHaveBeenCalledWith({
      to: '/apps/$name/deploys/$deployId/logs',
      params: { name: 'web', deployId: 'dep-ok-1' },
    })
  })

  it('shows the error state', async () => {
    attemptsState = 'error'
    renderIt()
    expect(await screen.findByRole('alert')).toBeTruthy()
  })

  it('shows the loading state', () => {
    attemptsState = 'pending'
    renderIt()
    expect(screen.getByText(/Loading timeline/)).toBeTruthy()
  })

  it('switches range with aria-pressed', async () => {
    renderIt()
    const sevenDay = await screen.findByRole('button', { name: '7d' })
    fireEvent.click(sevenDay)
    expect(sevenDay.getAttribute('aria-pressed')).toBe('true')
  })
})
