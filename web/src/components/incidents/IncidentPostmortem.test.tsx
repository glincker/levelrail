import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { QueryClientProvider } from '@tanstack/react-query'
import { IncidentPostmortem } from './IncidentPostmortem'
import type { IncidentPostmortem as Postmortem } from '../../types/incident'
import { createTestQueryClient } from '@/test/testQueryClient'

const published: Postmortem = {
  incident_id: 'si_1',
  state: 'published',
  summary: '<img src=x onerror=alert(1)>',
  impact: '',
  root_cause: 'pool size',
  resolution: '',
  action_items: ['raise pool'],
  impact_start: '2026-10-29T09:00:00Z',
  impact_end: '2026-10-29T10:00:00Z',
  alerts: [{ kind: 'monitor', ref: 'mon1' }],
  updated_at: '2026-10-29T11:00:00Z',
}

function renderSection() {
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <IncidentPostmortem id="si_1" />
    </QueryClientProvider>,
  )
}

describe('IncidentPostmortem', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('offers a draft when none exists and posts to the draft route', async () => {
    const calls: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push(`${init?.method ?? 'GET'} ${url}`)
        if (init?.method === 'POST') return Promise.resolve(Response.json({}))
        return Promise.resolve(new Response('{}', { status: 404 }))
      }),
    )
    renderSection()
    await userEvent.setup().click(
      await screen.findByRole('button', {
        name: 'incidents.postmortem.start',
      }),
    )
    expect(calls).toContain('POST /api/v1/incidents/si_1/postmortem/draft')
  })

  it('renders a published postmortem as read-only text', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(Response.json(published))),
    )
    renderSection()
    expect(
      await screen.findByText('<img src=x onerror=alert(1)>'),
    ).toBeInTheDocument()
    expect(document.querySelector('img')).toBeNull()
    expect(screen.queryByRole('textbox')).toBeNull()
    expect(screen.getByText('mon1')).toBeInTheDocument()
  })
})
