import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { PipelineSyncStatus } from '../types/pipelines'
import { PipelineSyncBar } from './PipelineSyncBar'

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

interface Sent {
  method: string
  body?: unknown
}

function stubFetch(status: PipelineSyncStatus, sent: Sent[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(
      (_input, init) => {
        const method = init?.method ?? 'GET'
        sent.push({
          method,
          body: init?.body ? JSON.parse(init.body as string) : undefined,
        })
        if (method === 'POST') {
          return Promise.resolve(
            jsonResponse({
              sha: 'def4567890',
              items: [
                {
                  file: 'ci.yaml',
                  name: 'ci',
                  outcome: 'diverged',
                  message: 'edited since the last sync',
                },
              ],
            }),
          )
        }
        if (method === 'PUT') {
          return Promise.resolve(
            jsonResponse({
              ...status,
              repo_is_truth: (
                JSON.parse(init?.body as string) as {
                  repo_is_truth: boolean
                }
              ).repo_is_truth,
            }),
          )
        }
        return Promise.resolve(jsonResponse(status))
      },
    ),
  )
}

function renderBar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <PipelineSyncBar appName="web" />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PipelineSyncBar', () => {
  it('shows the synced commit and runs a sync on demand', async () => {
    const sent: Sent[] = []
    stubFetch(
      {
        connected: true,
        repo_is_truth: false,
        last_sha: 'abc1234def',
        last_synced_at: '2026-01-01T00:00:00Z',
      },
      sent,
    )
    renderBar()
    expect(await screen.findByText('synced from abc1234')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /Sync now/ }))
    expect(await screen.findByText('diverged')).toBeInTheDocument()
    expect(screen.getByText('edited since the last sync')).toBeInTheDocument()
    expect(sent.some((s) => s.method === 'POST')).toBe(true)
  })

  it('saves the repository-is-source-of-truth setting', async () => {
    const sent: Sent[] = []
    stubFetch({ connected: true, repo_is_truth: false }, sent)
    renderBar()
    await userEvent.click(
      await screen.findByRole('switch', {
        name: 'Repository is source of truth',
      }),
    )
    await waitFor(() =>
      expect(sent.find((s) => s.method === 'PUT')?.body).toEqual({
        repo_is_truth: true,
      }),
    )
  })

  it('explains itself when no repository is connected', async () => {
    stubFetch({ connected: false, repo_is_truth: false }, [])
    renderBar()
    expect(
      await screen.findByText(/Connect a git repository/),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Sync now/ }),
    ).not.toBeInTheDocument()
  })
})
