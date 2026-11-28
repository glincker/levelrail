import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import '../../i18n'
import RemoteNodesCard from './RemoteNodesCard'

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('RemoteNodesCard', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('lists the last preview pass and the disk budget finding', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.endsWith('/api/v1/nodes'))
          return Promise.resolve(json([{ id: 'n1', name: 'edge-1' }]))
        if (url.endsWith('/cleanup-runs'))
          return Promise.resolve(
            json([
              {
                id: 'r1',
                started_at: '2026-11-27T10:00:00Z',
                trigger: 'janitor-dry',
                actor: 'janitor',
                freed_bytes: 0,
                classes: [
                  {
                    name: 'dangling_images',
                    deletable: true,
                    bytes: 1048576,
                    items: [],
                  },
                ],
              },
            ]),
          )
        return Promise.resolve(
          json({
            ok: false,
            checks: [
              {
                code: 'node_disk_budget',
                name: 'x',
                status: 'warn',
                message: 'edge-1 over budget',
              },
            ],
          }),
        )
      }),
    )
    render(
      <QueryClientProvider client={new QueryClient()}>
        <RemoteNodesCard />
      </QueryClientProvider>,
    )
    expect(await screen.findByText('edge-1')).toBeInTheDocument()
    expect(await screen.findByText(/Last preview/)).toBeInTheDocument()
    expect(await screen.findByTestId('disk-budget-message')).toHaveTextContent(
      'edge-1 over budget',
    )
  })
})
