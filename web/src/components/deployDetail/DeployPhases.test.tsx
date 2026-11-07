import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DeployDetailI18n } from '../../test/deployDetailI18n'
import { DeployPhases } from './DeployPhases'

const body = {
  deploy_id: 'd1',
  status: 'running',
  elapsed_ms: 240_000,
  eta_ms: 180_000,
  typical_total_ms: 420_000,
  samples: 5,
  phases: [
    {
      phase: 'build',
      status: 'done',
      started_at: null,
      finished_at: null,
      duration_ms: 90_000,
      typical_ms: null,
    },
    {
      phase: 'export_image',
      status: 'running',
      started_at: null,
      finished_at: null,
      duration_ms: null,
      typical_ms: null,
    },
  ],
}

afterEach(() => vi.unstubAllGlobals())

describe('DeployPhases', () => {
  it('shows chips, the eta line and the quiet export note', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response(JSON.stringify(body)))),
    )
    render(
      <QueryClientProvider client={new QueryClient()}>
        <DeployDetailI18n>
          <DeployPhases
            deployId="d1"
            running
            lastEventAt={Date.now() - 5 * 60_000}
          />
        </DeployDetailI18n>
      </QueryClientProvider>,
    )
    expect(await screen.findByText('Export image')).toBeInTheDocument()
    expect(
      screen.getByText('Usually takes about 7 min, 3 min left'),
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Exporting the image can be quiet/),
    ).toBeInTheDocument()
  })
})

describe('DeployPhases refetch', () => {
  const renderIt = (latestLine: string | null, streamHealthy: boolean) => (
    <QueryClientProvider client={client}>
      <DeployDetailI18n>
        <DeployPhases
          deployId="d1"
          running
          lastEventAt={Date.now()}
          streamHealthy={streamHealthy}
          latestLine={latestLine}
        />
      </DeployDetailI18n>
    </QueryClientProvider>
  )
  const client = new QueryClient()

  it('refetches once when a log line advances the stage, not on repeats', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(new Response(JSON.stringify(body))),
    )
    vi.stubGlobal('fetch', fetchMock)
    const { rerender } = render(renderIt('hello', true))
    await screen.findByText('Export image')
    const base = fetchMock.mock.calls.length
    rerender(renderIt('#6 RUN npm run build', true))
    await waitFor(() => expect(fetchMock.mock.calls.length).toBe(base + 1))
    rerender(renderIt('#6 RUN npm run build again', true))
    rerender(renderIt('more output', true))
    expect(fetchMock.mock.calls.length).toBe(base + 1)
  })

  it('does not refetch on lines when the stream is not healthy', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(new Response(JSON.stringify(body))),
    )
    vi.stubGlobal('fetch', fetchMock)
    client.clear()
    const { rerender } = render(renderIt('hello', false))
    await screen.findByText('Export image')
    const base = fetchMock.mock.calls.length
    rerender(renderIt('exporting to image', false))
    expect(fetchMock.mock.calls.length).toBe(base)
  })
})
