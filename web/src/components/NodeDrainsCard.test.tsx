import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../locales/en/nodes.json'
import { NodeDrainsCard } from './NodeDrainsCard'
import type { NodeDrain } from '../types/nodeMoves'
import { createTestQueryClient } from '@/test/testQueryClient'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

function respond(body: unknown, status = 200) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: status < 300,
        status,
        json: () => Promise.resolve(body),
      } as unknown as Response),
    ),
  )
}

function renderCard() {
  const queryClient = createTestQueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <NodeDrainsCard nodeId="node-1" />
    </QueryClientProvider>,
  )
}

describe('NodeDrainsCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows an empty state with a next step', async () => {
    respond([])
    renderCard()
    expect(
      await screen.findByText('This node has not been drained.'),
    ).toBeVisible()
  })

  it('lists a failed drain with its error and moves', async () => {
    const drain: NodeDrain = {
      id: 'drn_1',
      node_id: 'node-1',
      status: 'failed',
      error: 'move of database main failed',
      blocked: [{ kind: 'app', name: 'llm', reason: 'needs a GPU node' }],
      moves: [
        {
          id: 'mv_1',
          kind: 'database',
          resource: 'main',
          from_node_id: 'node-1',
          to_node_id: 'node-2',
          stateful: true,
          volumes: ['data'],
          phase: 'rollback',
          status: 'rolled_back',
          error: 'volume copy timed out',
          steps: [],
          started_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ],
      started_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    }
    respond([drain])
    renderCard()
    expect(await screen.findByText('Drain failed')).toBeVisible()
    expect(screen.getByText('move of database main failed')).toBeVisible()
    expect(screen.getByText('volume copy timed out')).toBeVisible()
    expect(screen.getByText(/needs a GPU node/)).toBeVisible()
  })
})
