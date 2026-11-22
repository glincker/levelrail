import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../../locales/en/nodes.json'
import { NodeDrainProgress } from './NodeDrainProgress'
import type { NodeDrain, NodeDrainProgressInfo } from '../../types/nodeMoves'
import { createTestQueryClient } from '@/test/testQueryClient'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

function drain(progress: NodeDrainProgressInfo): NodeDrain {
  return {
    id: 'd1',
    node_id: 'n1',
    status: 'running',
    blocked: [],
    moves: [],
    progress,
    started_at: '2026-11-21T10:00:00Z',
    updated_at: '2026-11-21T10:01:00Z',
  }
}

function renderDrain(d: NodeDrain) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve([]),
      } as unknown as Response),
    ),
  )
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <NodeDrainProgress drain={d} />
    </QueryClientProvider>,
  )
}

describe('NodeDrainProgress bar', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows moved count and the remaining time estimate', () => {
    renderDrain(
      drain({
        state: 'running',
        total: 3,
        succeeded: 1,
        failed: 0,
        rolled_back: 0,
        running: 2,
        blocked: 0,
        percent: 33,
        eta_seconds: 120,
      }),
    )
    expect(screen.getByText(/1 of 3 moved, 0 failed/)).toBeTruthy()
    expect(screen.getByText(/about 120 s left/)).toBeTruthy()
    expect(screen.queryByText(/No progress for a while/)).toBeNull()
  })

  it('warns when the drain is stalled', () => {
    renderDrain(
      drain({
        state: 'stalled',
        total: 2,
        succeeded: 0,
        failed: 1,
        rolled_back: 0,
        running: 1,
        blocked: 0,
        percent: 50,
      }),
    )
    expect(screen.getByText(/No progress for a while/)).toBeTruthy()
  })
})
