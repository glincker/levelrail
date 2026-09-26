import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ModelCacheCard } from './ModelCacheCard'
import type {
  CacheEntry,
  CachePruneResult,
  CacheReport,
} from '../types/modelPreflight'

vi.mock('../hooks/useIsRoot', () => ({ useIsRoot: () => true }))

const GIB = 1024 ** 3

function entry(over: Partial<CacheEntry>): CacheEntry {
  return {
    volume: 'lr-model-x-cache',
    size_bytes: GIB,
    last_used_at: '2026-06-01T00:00:00Z',
    last_used_source: 'volume_created',
    unused_days: 100,
    unused: true,
    in_use: false,
    configured: false,
    prunable: false,
    ...over,
  }
}

function report(entries: CacheEntry[]): CacheReport {
  return {
    unused_days: 30,
    note: 'Each model keeps its weights in its own Docker volume.',
    nodes: [
      {
        node_id: '',
        name: 'local',
        is_local: true,
        supported: true,
        entries,
        total_bytes: 3 * GIB,
        unique_bytes: 3 * GIB,
        reclaimable_bytes: GIB,
        disk_free_bytes: 50 * GIB,
      },
    ],
  }
}

function stubApi(cache: CacheReport) {
  const prunes: { dry_run: boolean; volumes: string[] }[] = []
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    if (url === '/api/v1/model-cache/prune') {
      const body = JSON.parse(
        typeof init?.body === 'string' ? init.body : '{}',
      ) as {
        dry_run: boolean
        volumes: string[]
      }
      prunes.push(body)
      const result: CachePruneResult = {
        dry_run: body.dry_run,
        candidates: [entry({ volume: 'lr-model-old-cache', prunable: true })],
        removed: body.dry_run ? [] : ['lr-model-old-cache'],
        skipped: [],
        reclaimed_bytes: body.dry_run ? 0 : GIB,
      }
      return Promise.resolve(new Response(JSON.stringify(result)))
    }
    return Promise.resolve(new Response(JSON.stringify(cache)))
  })
  vi.stubGlobal('fetch', fetchMock)
  return prunes
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ModelCacheCard />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ModelCacheCard', () => {
  it('lists volumes with their keep or prune state', async () => {
    stubApi(
      report([
        entry({ volume: 'lr-model-live-cache', in_use: true, model: 'live' }),
        entry({ volume: 'lr-model-old-cache', prunable: true }),
      ]),
    )
    renderCard()
    expect(await screen.findByText('lr-model-live-cache')).toBeInTheDocument()
    expect(screen.getByText('In use')).toBeInTheDocument()
    expect(screen.getByText('Prunable')).toBeInTheDocument()
    expect(screen.getByText(/3.0 GiB cached/)).toBeInTheDocument()
  })

  it('previews with a dry run, then removes only the previewed volumes', async () => {
    const prunes = stubApi(
      report([entry({ volume: 'lr-model-old-cache', prunable: true })]),
    )
    renderCard()
    await userEvent.click(
      await screen.findByRole('button', { name: /Prune unused/ }),
    )

    const confirm = await screen.findByRole('button', {
      name: 'Remove 1 volume',
    })
    expect(prunes).toEqual([{ dry_run: true, volumes: [] }])
    await userEvent.click(confirm)

    await waitFor(() => {
      expect(prunes).toContainEqual({
        dry_run: false,
        volumes: ['lr-model-old-cache'],
      })
    })
  })

  it('disables pruning when nothing is reclaimable', async () => {
    const empty = report([
      entry({ volume: 'lr-model-live-cache', in_use: true }),
    ])
    const [node] = empty.nodes
    if (node) node.reclaimable_bytes = 0
    stubApi(empty)
    renderCard()
    expect(
      await screen.findByRole('button', { name: /Prune unused/ }),
    ).toBeDisabled()
  })

  it('shows an error with a retry', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ error: 'docker is down' }), {
            status: 500,
          }),
        ),
      ),
    )
    renderCard()
    expect(await screen.findByRole('alert')).toHaveTextContent('docker is down')
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })

  it('shows an empty state when the host caches nothing', async () => {
    stubApi(report([]))
    renderCard()
    expect(
      await screen.findByText('No cached model weights'),
    ).toBeInTheDocument()
  })
})
