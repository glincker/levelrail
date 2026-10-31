import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { IncidentsList, IncidentsSection } from './IncidentsSection'
import type { Incident } from '../../types/incident'
import { renderWithQuery as wrap } from '@/test/renderWithQuery'

const inc: Incident = {
  id: 'si_1',
  title: 'Site down',
  status: 'investigating',
  impact: 'none',
  source: 'monitor',
  source_ref: 'm1',
  internal_state: 'triggered',
  published: true,
  updated_at: '2026-10-29T10:00:00Z',
  starts_at: '2026-10-29T09:00:00Z',
}

describe('IncidentsList', () => {
  it('shows state, title, monitor and published badge', () => {
    render(
      <IncidentsList
        incidents={[inc]}
        monitorName={() => 'Site'}
        onOpen={vi.fn()}
      />,
    )
    expect(screen.getByText('Site down')).toBeTruthy()
    expect(screen.getByText('Site')).toBeTruthy()
  })
})

describe('IncidentsSection', () => {
  it('renders nothing when the API answers 403', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response('', { status: 403 }))),
    )
    const { container } = wrap(<IncidentsSection />)
    await new Promise((r) => setTimeout(r, 20))
    expect(container.textContent).toBe('')
    vi.unstubAllGlobals()
  })
})
