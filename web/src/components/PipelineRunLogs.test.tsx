import { render, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PipelineRunLogs } from './PipelineRunLogs'

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PipelineRunLogs', () => {
  it.each([
    { step: 2, want: '&step=2' },
    { step: 0, want: '&step=0' },
    { step: undefined, want: undefined },
  ])('asks the server to filter by step $step', async ({ step, want }) => {
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >(() => Promise.resolve(jsonResponse([])))
    vi.stubGlobal('fetch', fetchMock)
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={qc}>
        <PipelineRunLogs
          app="web"
          runId="r1"
          job="test"
          step={step}
          live={false}
        />
      </QueryClientProvider>,
    )
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const input = fetchMock.mock.calls[0]?.[0]
    const url = typeof input === 'string' ? input : ''
    expect(url).toContain('/pipeline-runs/r1/logs?limit=5000&job=test')
    if (want === undefined) {
      expect(url).not.toContain('step=')
    } else {
      expect(url).toContain(want)
    }
  })
})
