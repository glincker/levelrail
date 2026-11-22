import { cleanup, render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../../locales/en/nodes.json'
import { NodeHealthTimelineCard } from './NodeHealthTimelineCard'
import { createTestQueryClient } from '@/test/testQueryClient'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

describe('NodeHealthTimelineCard', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('shows uptime and the strip once the timeline loads', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({
              from: '2026-11-21T00:00:00Z',
              to: '2026-11-21T04:00:00Z',
              transitions: 2,
              uptime_percent: 50,
              segments: [
                {
                  state: 'online',
                  from: '2026-11-21T00:00:00Z',
                  to: '2026-11-21T02:00:00Z',
                  seconds: 7200,
                },
                {
                  state: 'offline',
                  from: '2026-11-21T02:00:00Z',
                  to: '2026-11-21T04:00:00Z',
                  seconds: 7200,
                },
              ],
            }),
        } as unknown as Response),
      ),
    )
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <NodeHealthTimelineCard nodeId="n1" />
      </QueryClientProvider>,
    )
    expect(await screen.findByText('50% uptime, 2 changes')).toBeInTheDocument()
    expect(
      screen.getByRole('img', { name: 'Online and offline periods' }),
    ).toBeInTheDocument()
  })
})
