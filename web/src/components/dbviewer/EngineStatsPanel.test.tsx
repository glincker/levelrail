import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { EngineStatsPanel } from './EngineStatsPanel'
import { DatabasesI18n } from '../../test/databasesI18n'
import { createTestQueryClient } from '@/test/testQueryClient'

function stubStats(status: number, body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: status === 200,
        status,
        json: () => Promise.resolve(body),
        text: () => Promise.resolve(JSON.stringify(body)),
        headers: new Headers({ 'content-type': 'application/json' }),
      } as unknown as Response),
    ),
  )
}

function renderPanel(engine: string) {
  const client = createTestQueryClient()
  render(
    <QueryClientProvider client={client}>
      <DatabasesI18n>
        <EngineStatsPanel databaseName="search" engine={engine} />
      </DatabasesI18n>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('EngineStatsPanel', () => {
  it('says there is no browser and lists live counts', async () => {
    stubStats(200, {
      engine: 'qdrant',
      objects: [
        { name: 'docs', count: 10 },
        { name: 'faq', count: 3 },
      ],
      total: 13,
    })
    renderPanel('qdrant')
    expect(screen.getByText('No data browser for qdrant')).toBeInTheDocument()
    expect(await screen.findByText('docs')).toBeInTheDocument()
    expect(screen.getByText('Points')).toBeInTheDocument()
    expect(screen.getByText('13 items in total')).toBeInTheDocument()
  })

  it('shows Memcached server stats', async () => {
    stubStats(200, {
      engine: 'memcached',
      objects: [],
      total: 4,
      metrics: {
        curr_items: 4,
        get_hits: 3,
        get_misses: 1,
        bytes: 1024,
        limit_maxbytes: 2048,
      },
    })
    renderPanel('memcached')
    expect(await screen.findByText('75%')).toBeInTheDocument()
    expect(screen.getByText('Cache stats')).toBeInTheDocument()
  })

  it('shows real error copy when the engine does not answer', async () => {
    stubStats(502, { error: 'the engine did not answer its stats API' })
    renderPanel('meilisearch')
    expect(
      await screen.findByText('Could not read live stats'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Try again' }),
    ).toBeInTheDocument()
  })
})
