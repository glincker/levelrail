import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NodesI18n } from '../../test/nodesI18n'
import type { NodeProvisionResource } from '../../types/nodeProvision'
import { DestroyServerAction } from './DestroyServerAction'

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

const provision: NodeProvisionResource = {
  id: 'np_1',
  provider: 'hetzner',
  region: 'fsn1',
  size: 'cx22',
  name: 'edge-1',
  role: 'general',
  status: 'ready',
  node_id: 'nd_1',
  created_at: '2026-11-16T00:00:00Z',
  updated_at: '2026-11-16T00:00:00Z',
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function setup(fetchMock: typeof fetch) {
  vi.stubGlobal('fetch', fetchMock)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <NodesI18n>
        <DestroyServerAction nodeId="nd_1" />
      </NodesI18n>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DestroyServerAction', () => {
  it('requires the server name, then offers force after a 409', async () => {
    const calls: string[] = []
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === 'DELETE') {
        calls.push(url)
        return Promise.resolve(
          url.includes('force=true')
            ? json(200, { ...provision, status: 'destroyed' })
            : json(409, { error: 'the node still runs workloads' }),
        )
      }
      return Promise.resolve(json(200, [provision]))
    }) as unknown as typeof fetch
    setup(fetchMock)
    const user = userEvent.setup()

    await user.click(
      await screen.findByRole('button', { name: 'Destroy server' }),
    )
    const confirm = screen.getByRole('button', { name: 'Destroy server' })
    expect((confirm as HTMLButtonElement).disabled).toBe(true)
    await user.type(screen.getByRole('textbox'), 'edge-1')
    expect((confirm as HTMLButtonElement).disabled).toBe(false)
    await user.click(confirm)

    expect(
      await screen.findByText('This node still runs workloads'),
    ).toBeTruthy()
    const force = screen.getByRole('checkbox')
    expect((confirm as HTMLButtonElement).disabled).toBe(true)
    await user.click(force)
    await user.click(confirm)

    await waitFor(() => {
      expect(calls).toEqual([
        '/api/v1/node-provisions/np_1',
        '/api/v1/node-provisions/np_1?force=true',
      ])
    })
  })
})
