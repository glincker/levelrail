import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DeployDetailI18n } from '@/test/deployDetailI18n'
import { BuildStagesPanel } from './BuildStagesPanel'

const streamUrls: string[] = []
vi.mock('../../hooks/useLogStream', () => ({
  useLogStream: (url: string) => {
    streamUrls.push(url)
    return {
      lines: [{ id: 1, line: 'RUN npm ci', stream: 'stdout' }],
      connectionState: 'open',
      isPaused: false,
      pause: vi.fn(),
      resume: vi.fn(),
    }
  },
}))
vi.mock('../LogTerminal', () => ({
  LogTerminal: () => <div data-testid="terminal" />,
}))

const body = {
  deploy_id: 'd1',
  status: 'failed',
  wall_ms: 9000,
  stages_ms: 7000,
  stages: [
    {
      stage: 'install',
      status: 'done',
      started_at: '2026-11-15T10:00:00Z',
      ended_at: '2026-11-15T10:00:02Z',
      duration_ms: 2000,
      cache: 'partial',
      vertices: 9,
      cached_vertices: 7,
      log_bytes: 10,
    },
    {
      stage: 'build',
      status: 'failed',
      started_at: '2026-11-15T10:00:02Z',
      ended_at: '2026-11-15T10:00:07Z',
      duration_ms: 5000,
      cache: 'miss',
      vertices: 2,
      cached_vertices: 0,
      log_bytes: 20,
    },
  ],
}

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <DeployDetailI18n>
        <BuildStagesPanel appName="web" deployId="d1" />
      </DeployDetailI18n>
    </QueryClientProvider>,
  )
}

describe('BuildStagesPanel', () => {
  beforeEach(() => {
    streamUrls.length = 0
  })
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('renders the graph, chips and failure hint, and opens a stage log', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response(JSON.stringify(body)))),
    )
    const user = userEvent.setup()
    renderPanel()
    expect(await screen.findByText('Cache 7 of 9')).toBeTruthy()
    expect(screen.getByText('Cache miss')).toBeTruthy()
    expect(screen.getByTestId('stage-failure-hint').textContent).toContain(
      'Build',
    )
    expect(screen.getByTestId('waterfall-totals').textContent).toContain(
      'Wall time 9s',
    )
    await user.click(screen.getByRole('button', { name: /^Build, failed/ }))
    expect(await screen.findByTestId('terminal')).toBeTruthy()
    expect(streamUrls.at(-1)).toContain('/logs?stage=build')
  })

  it('counts a running stage in the stages total instead of showing 0s', async () => {
    const start = new Date(Date.now() - 20000).toISOString()
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              deploy_id: 'd1',
              status: 'running',
              wall_ms: 0,
              stages_ms: 0,
              stages: [
                {
                  stage: 'detect',
                  status: 'running',
                  started_at: start,
                  duration_ms: 0,
                  vertices: 0,
                  cached_vertices: 0,
                  log_bytes: 0,
                },
              ],
            }),
          ),
        ),
      ),
    )
    renderPanel()
    const totals = await screen.findByTestId('waterfall-totals')
    expect(totals.textContent).toContain('Running for')
    expect(totals.textContent).not.toContain('stages so far 0s')
    expect(screen.getAllByRole('listitem')).toHaveLength(1)
  })

  it('shows an empty state for null stages and a 404', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(new Response(JSON.stringify({ stages: null }))),
      ),
    )
    renderPanel()
    expect(
      await screen.findByText('No build stages were recorded for this deploy.'),
    ).toBeTruthy()
  })

  it('shows an error state with retry', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response('boom', { status: 500 }))),
    )
    renderPanel()
    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeTruthy()
    })
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
  })
})
