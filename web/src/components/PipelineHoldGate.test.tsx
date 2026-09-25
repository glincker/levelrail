import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PipelineHoldGate } from './PipelineHoldGate'
import { PipelineTriggerLog } from './PipelineTriggerLog'

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function wrap(node: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={qc}>{node}</QueryClientProvider>)
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PipelineHoldGate', () => {
  it('sends the approver decision for the held run', async () => {
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >(() => Promise.resolve(jsonResponse({ decision: 'approved' })))
    vi.stubGlobal('fetch', fetchMock)
    wrap(
      <PipelineHoldGate
        app="web"
        runId="r1"
        hold={{
          state: 'pending',
          reason: 'pull request from a fork (mallory/app) needs an approver',
        }}
      />,
    )
    expect(screen.getByText(/mallory\/app/)).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: /Approve and run/ }),
    )
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
    const [url, init] = fetchMock.mock.calls[0] ?? []
    expect(url).toBe('/api/v1/apps/web/pipeline-runs/r1/hold')
    expect(JSON.parse(init?.body as string)).toEqual({ decision: 'approved' })
  })
})

describe('PipelineTriggerLog', () => {
  it('lists why events started nothing, and renders nothing when empty', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse([
            {
              id: 1,
              event: 'pull_request',
              pipeline: 'ci',
              decision: 'skipped',
              reason: 'pull request from a fork blocked by forks: block',
              created_at: '2026-01-01T00:00:00Z',
            },
          ]),
        ),
      ),
    )
    const { unmount } = wrap(<PipelineTriggerLog appName="web" />)
    expect(await screen.findByText('skipped')).toBeInTheDocument()
    expect(screen.getByText(/blocked by forks: block/)).toBeInTheDocument()
    unmount()

    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse([]))),
    )
    const empty = wrap(<PipelineTriggerLog appName="web" />)
    await waitFor(() => expect(fetch).toHaveBeenCalled())
    expect(empty.container).toBeEmptyDOMElement()
  })
})
